package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHashAndVerify(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyPassword(h, "correct horse battery")
	if err != nil || !ok {
		t.Fatalf("expected match, ok=%v err=%v", ok, err)
	}
	ok, _ = VerifyPassword(h, "wrong")
	if ok {
		t.Fatal("wrong password matched")
	}
	if _, err := VerifyPassword("$bcrypt$nope", "x"); err == nil {
		t.Fatal("expected error for non argon2id hash")
	}
}

func TestSessionRoundTripAndRevoke(t *testing.T) {
	m, err := NewManager("password123", "", "", t.TempDir(), time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	if err := m.Issue(rec); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/overview", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	if !m.Authenticated(req) {
		t.Fatal("fresh session must authenticate")
	}
	rec2 := httptest.NewRecorder()
	m.Clear(rec2, req)
	if m.Authenticated(req) {
		t.Fatal("revoked session must not authenticate")
	}
}

func TestTamperedTokenRejected(t *testing.T) {
	m, err := NewManager("password123", "", "", t.TempDir(), time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: "abc.9999999999.forged"})
	if m.Authenticated(req) {
		t.Fatal("forged token accepted")
	}
}

func TestRateLimiter(t *testing.T) {
	l := newRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.allow("1.2.3.4") {
			t.Fatalf("attempt %d should pass", i)
		}
	}
	if l.allow("1.2.3.4") {
		t.Fatal("fourth attempt should be blocked")
	}
	if !l.allow("5.6.7.8") {
		t.Fatal("other client should pass")
	}
}

func TestCSRFMiddleware(t *testing.T) {
	m, _ := NewManager("password123", "", "", t.TempDir(), time.Hour, false)
	handler := m.CSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/1/ack", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("missing token should be 403, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/alerts/1/ack", nil)
	req.AddCookie(&http.Cookie{Name: CSRFCookie, Value: "tok"})
	req.Header.Set(CSRFHeader, "tok")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("matching token should pass, got %d", rec.Code)
	}
}
