package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	SessionCookie = "hd_session"
	CSRFCookie    = "hd_csrf"
	CSRFHeader    = "X-CSRF-Token"
)

type Manager struct {
	hash   string
	secret []byte
	ttl    time.Duration
	secure bool

	mu      sync.Mutex
	revoked map[string]time.Time
	limiter *rateLimiter
}

func NewManager(password, hash, secret, dataDir string, ttl time.Duration, secure bool) (*Manager, error) {
	if hash == "" {
		h, err := HashPassword(password)
		if err != nil {
			return nil, err
		}
		hash = h
	} else if _, err := VerifyPassword(hash, "probe"); err != nil {
		return nil, err
	}
	key, err := loadOrCreateSecret(secret, dataDir)
	if err != nil {
		return nil, err
	}
	return &Manager{
		hash:    hash,
		secret:  key,
		ttl:     ttl,
		secure:  secure,
		revoked: map[string]time.Time{},
		limiter: newRateLimiter(5, time.Minute),
	}, nil
}

func loadOrCreateSecret(configured, dataDir string) ([]byte, error) {
	if configured != "" {
		if len(configured) < 16 {
			return nil, errors.New("SESSION_SECRET must be at least 16 characters")
		}
		return []byte(configured), nil
	}
	path := filepath.Join(dataDir, "session.secret")
	if data, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(data))) >= 32 {
		return []byte(strings.TrimSpace(string(data))), nil
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	encoded := hex.EncodeToString(raw)
	if err := os.WriteFile(path, []byte(encoded+"\n"), 0o600); err != nil {
		return nil, fmt.Errorf("write session secret: %w", err)
	}
	return []byte(encoded), nil
}

func (m *Manager) CheckPassword(password string) bool {
	ok, err := VerifyPassword(m.hash, password)
	return err == nil && ok
}

func (m *Manager) Allow(remote string) bool {
	return m.limiter.allow(clientIP(remote))
}

func (m *Manager) Issue(w http.ResponseWriter) error {
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return err
	}
	id := hex.EncodeToString(idBytes)
	expires := time.Now().Add(m.ttl)
	payload := id + "." + strconv.FormatInt(expires.Unix(), 10)
	token := payload + "." + m.sign(payload)

	csrfBytes := make([]byte, 16)
	if _, err := rand.Read(csrfBytes); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: token, Path: "/", Expires: expires,
		HttpOnly: true, Secure: m.secure, SameSite: http.SameSiteStrictMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name: CSRFCookie, Value: hex.EncodeToString(csrfBytes), Path: "/", Expires: expires,
		HttpOnly: false, Secure: m.secure, SameSite: http.SameSiteStrictMode,
	})
	return nil
}

func (m *Manager) Clear(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookie); err == nil {
		if id, exp, ok := m.parse(c.Value); ok {
			m.mu.Lock()
			m.revoked[id] = exp
			m.mu.Unlock()
		}
	}
	for _, name := range []string{SessionCookie, CSRFCookie} {
		http.SetCookie(w, &http.Cookie{
			Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: name == SessionCookie,
			Secure: m.secure, SameSite: http.SameSiteStrictMode,
		})
	}
}

func (m *Manager) Authenticated(r *http.Request) bool {
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return false
	}
	id, _, ok := m.parse(c.Value)
	if !ok {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.revoked) > 0 {
		now := time.Now()
		for rid, exp := range m.revoked {
			if exp.Before(now) {
				delete(m.revoked, rid)
			}
		}
		if _, gone := m.revoked[id]; gone {
			return false
		}
	}
	return true
}

func (m *Manager) parse(token string) (string, time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", time.Time{}, false
	}
	payload := parts[0] + "." + parts[1]
	if subtle.ConstantTimeCompare([]byte(m.sign(payload)), []byte(parts[2])) != 1 {
		return "", time.Time{}, false
	}
	expUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", time.Time{}, false
	}
	exp := time.Unix(expUnix, 0)
	if time.Now().After(exp) {
		return "", time.Time{}, false
	}
	return parts[0], exp, true
}

func (m *Manager) sign(payload string) string {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

type ctxKey struct{}

func (m *Manager) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.Authenticated(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"authentication required"}`))
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, true)))
	})
}

func (m *Manager) CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie(CSRFCookie)
		header := r.Header.Get(CSRFHeader)
		if err != nil || header == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) != 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":"csrf token mismatch"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

type rateLimiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	attempts map[string][]time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{limit: limit, window: window, attempts: map[string][]time.Time{}}
}

func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-l.window)
	kept := l.attempts[key][:0]
	for _, t := range l.attempts[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.limit {
		l.attempts[key] = kept
		return false
	}
	l.attempts[key] = append(kept, now)
	if len(l.attempts) > 10000 {
		for k, ts := range l.attempts {
			if len(ts) == 0 || ts[len(ts)-1].Before(cutoff) {
				delete(l.attempts, k)
			}
		}
	}
	return true
}

func clientIP(remote string) string {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return remote
	}
	return host
}

func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		return strings.TrimSpace(first)
	}
	if rip := r.Header.Get("X-Real-IP"); rip != "" {
		return rip
	}
	return clientIP(r.RemoteAddr)
}
