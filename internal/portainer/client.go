package portainer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

var ErrUnauthorized = errors.New("portainer rejected the API key")

type Client struct {
	baseURL    string
	keySource  func() string
	httpClient *http.Client

	mu  sync.Mutex
	key string
}

type Status struct {
	Version    string `json:"Version"`
	InstanceID string `json:"InstanceID"`
}

type Endpoint struct {
	ID     int    `json:"Id"`
	Name   string `json:"Name"`
	Type   int    `json:"Type"`
	URL    string `json:"URL"`
	Status int    `json:"Status"`
}

type Stack struct {
	ID         int    `json:"Id"`
	Name       string `json:"Name"`
	Type       int    `json:"Type"`
	EndpointID int    `json:"EndpointId"`
	Status     int    `json:"Status"`
	CreatedBy  string `json:"CreatedBy"`
	Created    int64  `json:"CreationDate"`
	Updated    int64  `json:"UpdateDate"`
}

func (e Endpoint) Online() bool {
	return e.Status == 1
}

func (s Stack) Active() bool {
	return s.Status == 1
}

func (s Stack) TypeName() string {
	switch s.Type {
	case 1:
		return "swarm"
	case 2:
		return "compose"
	case 3:
		return "kubernetes"
	default:
		return "unknown"
	}
}

func New(baseURL string, keySource func() string) *Client {
	return &Client{
		baseURL:   baseURL,
		keySource: keySource,
		key:       keySource(),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *Client) Status(ctx context.Context) (Status, error) {
	var out Status
	err := c.getJSON(ctx, "/api/system/status", nil, &out)
	return out, err
}

func (c *Client) Endpoints(ctx context.Context) ([]Endpoint, error) {
	var out []Endpoint
	err := c.getJSON(ctx, "/api/endpoints", nil, &out)
	return out, err
}

func (c *Client) Stacks(ctx context.Context) ([]Stack, error) {
	var out []Stack
	err := c.getJSON(ctx, "/api/stacks", nil, &out)
	return out, err
}

func (c *Client) StacksForEndpoint(ctx context.Context, endpointID int) ([]Stack, error) {
	filters, _ := json.Marshal(map[string]int{"EndpointID": endpointID})
	q := url.Values{"filters": {string(filters)}}
	var out []Stack
	err := c.getJSON(ctx, "/api/stacks", q, &out)
	return out, err
}

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	body, status, err := c.do(ctx, path, query)
	if err != nil {
		return err
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		c.refreshKey()
		body, status, err = c.do(ctx, path, query)
		if err != nil {
			return err
		}
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return ErrUnauthorized
		}
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("portainer %s returned %d", path, status)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("portainer %s: decode: %w", path, err)
	}
	return nil
}

func (c *Client) do(ctx context.Context, path string, query url.Values) ([]byte, int, error) {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-API-Key", c.currentKey())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("portainer %s: %w", path, sanitize(err))
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("portainer %s: read: %w", path, err)
	}
	return body, resp.StatusCode, nil
}

func (c *Client) currentKey() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.key
}

func (c *Client) refreshKey() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if next := c.keySource(); next != "" {
		c.key = next
	}
}

func sanitize(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Errorf("%s %s: %w", urlErr.Op, redactURL(urlErr.URL), urlErr.Err)
	}
	return err
}

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<url>"
	}
	u.RawQuery = ""
	u.User = nil
	return u.String()
}

func EndpointLabel(e Endpoint) string {
	if e.Name != "" {
		return e.Name
	}
	return "endpoint-" + strconv.Itoa(e.ID)
}
