package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/iModyHK/homelab-dashboard/internal/config"
	"github.com/iModyHK/homelab-dashboard/internal/docker"
)

const manifestAccept = "application/vnd.docker.distribution.manifest.list.v2+json, " +
	"application/vnd.oci.image.index.v1+json, " +
	"application/vnd.docker.distribution.manifest.v2+json, " +
	"application/vnd.oci.image.manifest.v1+json"

type Result struct {
	Image           string
	LocalDigest     string
	RemoteDigest    string
	UpdateAvailable bool
	Error           string
}

type Checker struct {
	creds  map[string]config.RegistryCredential
	http   *http.Client
	log    *slog.Logger
	mu     sync.Mutex
	tokens map[string]cachedToken
}

type cachedToken struct {
	value   string
	expires time.Time
}

func NewChecker(creds map[string]config.RegistryCredential, logger *slog.Logger) *Checker {
	return &Checker{
		creds:  creds,
		http:   &http.Client{Timeout: 20 * time.Second},
		log:    logger,
		tokens: map[string]cachedToken{},
	}
}

func (c *Checker) Check(ctx context.Context, images []string, local []docker.ImageSummary) []Result {
	localDigests := indexLocalDigests(local)
	results := make([]Result, len(images))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, image := range images {
		wg.Add(1)
		go func(i int, image string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i] = Result{Image: image, Error: "cancelled"}
				return
			}
			defer func() { <-sem }()
			results[i] = c.checkOne(ctx, image, localDigests)
		}(i, image)
	}
	wg.Wait()
	return results
}

func indexLocalDigests(local []docker.ImageSummary) map[string][]string {
	out := map[string][]string{}
	for _, img := range local {
		for _, rd := range img.RepoDigests {
			repo, digest := NormalizeRepoDigest(rd)
			if repo != "" {
				out[repo] = append(out[repo], digest)
			}
		}
		for _, tag := range img.RepoTags {
			ref, err := ParseReference(tag)
			if err != nil {
				continue
			}
			key := ref.RepoKey() + ":" + ref.Tag
			for _, rd := range img.RepoDigests {
				_, digest := NormalizeRepoDigest(rd)
				if digest != "" {
					out[key] = append(out[key], digest)
				}
			}
		}
	}
	return out
}

func (c *Checker) checkOne(ctx context.Context, image string, localDigests map[string][]string) Result {
	res := Result{Image: image}
	ref, err := ParseReference(image)
	if err != nil {
		res.Error = "local build"
		return res
	}
	candidates := localDigests[ref.RepoKey()+":"+ref.Tag]
	if len(candidates) == 0 {
		candidates = localDigests[ref.RepoKey()]
	}
	if len(candidates) == 0 {
		res.Error = "no local digest"
		return res
	}
	remote, err := c.remoteDigest(ctx, ref)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.RemoteDigest = remote
	res.LocalDigest = candidates[0]
	res.UpdateAvailable = true
	for _, d := range candidates {
		if d == remote {
			res.UpdateAvailable = false
			res.LocalDigest = d
			break
		}
	}
	return res
}

func (c *Checker) remoteDigest(ctx context.Context, ref Reference) (string, error) {
	token, _ := c.cachedToken(ref)
	digest, status, challenge, err := c.headManifest(ctx, ref, token)
	if err != nil {
		return "", err
	}
	if status == http.StatusUnauthorized && challenge != "" {
		token, err = c.fetchToken(ctx, ref, challenge)
		if err != nil {
			return "", err
		}
		digest, status, _, err = c.headManifest(ctx, ref, token)
		if err != nil {
			return "", err
		}
	}
	switch status {
	case http.StatusOK:
		if digest != "" {
			return digest, nil
		}
		return c.getManifestDigest(ctx, ref, token)
	case http.StatusNotFound:
		return "", errors.New("tag not found upstream")
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", errors.New("registry denied access")
	case http.StatusTooManyRequests:
		return "", errors.New("registry rate limited")
	default:
		return "", fmt.Errorf("registry returned %d", status)
	}
}

func (c *Checker) headManifest(ctx context.Context, ref Reference, token string) (digest string, status int, challenge string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, ref.ManifestURL(), nil)
	if err != nil {
		return "", 0, "", err
	}
	req.Header.Set("Accept", manifestAccept)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", 0, "", fmt.Errorf("registry unreachable: %w", redactErr(err))
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.Header.Get("Docker-Content-Digest"), resp.StatusCode, resp.Header.Get("WWW-Authenticate"), nil
}

func (c *Checker) getManifestDigest(ctx context.Context, ref Reference, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref.ManifestURL(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", manifestAccept)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("registry unreachable: %w", redactErr(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry returned %d", resp.StatusCode)
	}
	if d := resp.Header.Get("Docker-Content-Digest"); d != "" {
		return d, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func (c *Checker) cachedToken(ref Reference) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.tokens[ref.Registry+"|"+ref.Scope()]
	if !ok || time.Now().After(t.expires) {
		return "", false
	}
	return t.value, true
}

func (c *Checker) fetchToken(ctx context.Context, ref Reference, challenge string) (string, error) {
	params := parseChallenge(challenge)
	realm := params["realm"]
	if realm == "" {
		return "", errors.New("registry challenge has no realm")
	}
	q := url.Values{}
	if s := params["service"]; s != "" {
		q.Set("service", s)
	}
	scope := params["scope"]
	if scope == "" {
		scope = ref.Scope()
	}
	q.Set("scope", scope)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realm+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	if cred, ok := c.credentialFor(ref.Registry); ok {
		req.SetBasicAuth(cred.Username, cred.Password)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("token endpoint unreachable: %w", redactErr(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned %d", resp.StatusCode)
	}
	var payload struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", fmt.Errorf("token decode: %w", err)
	}
	token := payload.Token
	if token == "" {
		token = payload.AccessToken
	}
	if token == "" {
		return "", errors.New("token endpoint returned no token")
	}
	ttl := time.Duration(payload.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 4 * time.Minute
	}
	c.mu.Lock()
	c.tokens[ref.Registry+"|"+ref.Scope()] = cachedToken{value: token, expires: time.Now().Add(ttl - 30*time.Second)}
	c.mu.Unlock()
	return token, nil
}

func (c *Checker) credentialFor(registry string) (config.RegistryCredential, bool) {
	if cred, ok := c.creds[registry]; ok {
		return cred, true
	}
	if registry == dockerHubRegistry {
		for _, alias := range []string{"docker.io", "index.docker.io", "hub.docker.com"} {
			if cred, ok := c.creds[alias]; ok {
				return cred, true
			}
		}
	}
	return config.RegistryCredential{}, false
}

func parseChallenge(header string) map[string]string {
	out := map[string]string{}
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return out
	}
	header = header[len("bearer "):]
	for _, part := range splitChallenge(header) {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	return out
}

func splitChallenge(s string) []string {
	var parts []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case r == ',' && !inQuote:
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

func redactErr(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}
