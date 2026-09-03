package docker

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	streaming  *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        16,
				MaxIdleConnsPerHost: 16,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		streaming: &http.Client{
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 4,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func (c *Client) Ping(ctx context.Context) error {
	resp, err := c.get(ctx, "/_ping", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("docker ping returned %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) Version(ctx context.Context) (Version, error) {
	var out Version
	err := c.getJSON(ctx, "/version", nil, &out)
	return out, err
}

func (c *Client) Info(ctx context.Context) (Info, error) {
	var out Info
	err := c.getJSON(ctx, "/info", nil, &out)
	return out, err
}

func (c *Client) ListContainers(ctx context.Context) ([]ContainerSummary, error) {
	var out []ContainerSummary
	err := c.getJSON(ctx, "/containers/json", url.Values{"all": {"true"}}, &out)
	return out, err
}

func (c *Client) Inspect(ctx context.Context, id string) (ContainerInspect, error) {
	var out ContainerInspect
	err := c.getJSON(ctx, "/containers/"+url.PathEscape(id)+"/json", nil, &out)
	return out, err
}

func (c *Client) Stats(ctx context.Context, id string) (Stats, error) {
	var out Stats
	q := url.Values{"stream": {"false"}, "one-shot": {"true"}}
	err := c.getJSON(ctx, "/containers/"+url.PathEscape(id)+"/stats", q, &out)
	return out, err
}

func (c *Client) ListImages(ctx context.Context) ([]ImageSummary, error) {
	var out []ImageSummary
	err := c.getJSON(ctx, "/images/json", nil, &out)
	return out, err
}

type LogLine struct {
	Time   time.Time
	Stream string
	Text   string
}

type LogOptions struct {
	Tail  int
	Since time.Time
	Until time.Time
}

func (c *Client) Logs(ctx context.Context, id string, tty bool, opts LogOptions) ([]LogLine, error) {
	q := url.Values{
		"stdout":     {"true"},
		"stderr":     {"true"},
		"timestamps": {"true"},
	}
	if opts.Tail > 0 {
		q.Set("tail", strconv.Itoa(opts.Tail))
	}
	if !opts.Since.IsZero() {
		q.Set("since", strconv.FormatInt(opts.Since.Unix(), 10))
	}
	if !opts.Until.IsZero() {
		q.Set("until", strconv.FormatInt(opts.Until.Unix(), 10))
	}
	resp, err := c.get(ctx, "/containers/"+url.PathEscape(id)+"/logs", q)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("logs", resp)
	}
	limited := io.LimitReader(resp.Body, 32<<20)
	if tty {
		return parseRawLogs(limited, "stdout")
	}
	return parseMultiplexedLogs(limited)
}

func (c *Client) Events(ctx context.Context, since time.Time, handle func(Event)) error {
	q := url.Values{}
	if !since.IsZero() {
		q.Set("since", strconv.FormatInt(since.Unix(), 10))
	}
	filters, _ := json.Marshal(map[string][]string{"type": {"container"}})
	q.Set("filters", string(filters))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/events?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := c.streaming.Do(req)
	if err != nil {
		return fmt.Errorf("docker events: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return statusError("events", resp)
	}
	dec := json.NewDecoder(resp.Body)
	for {
		var ev Event
		if err := dec.Decode(&ev); err != nil {
			if errors.Is(err, io.EOF) || ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("docker events: decode: %w", err)
		}
		handle(ev)
	}
}

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	resp, err := c.get(ctx, path, query)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return statusError(path, resp)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(out); err != nil {
		return fmt.Errorf("docker %s: decode: %w", path, err)
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values) (*http.Response, error) {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("docker %s: %w", path, err)
	}
	return resp, nil
}

type NotFoundError struct {
	Path string
}

func (e NotFoundError) Error() string {
	return "docker " + e.Path + ": not found"
}

func statusError(path string, resp *http.Response) error {
	if resp.StatusCode == http.StatusNotFound {
		return NotFoundError{Path: path}
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	return fmt.Errorf("docker %s returned %d: %s", path, resp.StatusCode, msg)
}

func IsNotFound(err error) bool {
	var nf NotFoundError
	return errors.As(err, &nf)
}

func parseMultiplexedLogs(r io.Reader) ([]LogLine, error) {
	br := bufio.NewReaderSize(r, 64<<10)
	var lines []LogLine
	header := make([]byte, 8)
	for {
		if _, err := io.ReadFull(br, header); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return lines, nil
			}
			return lines, err
		}
		stream := "stdout"
		if header[0] == 2 {
			stream = "stderr"
		}
		size := binary.BigEndian.Uint32(header[4:8])
		if size > 4<<20 {
			return lines, fmt.Errorf("log frame too large: %d", size)
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(br, payload); err != nil {
			return lines, err
		}
		for _, chunk := range strings.Split(strings.TrimRight(string(payload), "\n"), "\n") {
			if chunk == "" {
				continue
			}
			lines = append(lines, splitTimestamp(chunk, stream))
		}
	}
}

func parseRawLogs(r io.Reader, stream string) ([]LogLine, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	var lines []LogLine
	for sc.Scan() {
		text := strings.TrimRight(sc.Text(), "\r")
		if text == "" {
			continue
		}
		lines = append(lines, splitTimestamp(text, stream))
	}
	return lines, sc.Err()
}

func splitTimestamp(raw, stream string) LogLine {
	ts, rest, ok := strings.Cut(raw, " ")
	if ok {
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			return LogLine{Time: t, Stream: stream, Text: rest}
		}
	}
	return LogLine{Time: time.Time{}, Stream: stream, Text: raw}
}
