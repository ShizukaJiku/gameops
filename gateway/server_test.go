package gateway

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ShizukaJiku/gameops/internal/gwconfig"
	"github.com/ShizukaJiku/gameops/internal/webauth"
)

func testConfig() *gwconfig.GatewayConfig {
	hash, err := webauth.HashPassword("correct-horse")
	if err != nil {
		panic(err)
	}
	return &gwconfig.GatewayConfig{
		AdminPasswordHash: hash,
		SessionSecret:     "test-secret-at-least-this-long",
	}
}

func TestLoginPageRendersForm(t *testing.T) {
	s := NewServer(testConfig())
	ts := httptest.NewServer(s)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/login")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestLoginSubmitWrongPasswordRejected(t *testing.T) {
	s := NewServer(testConfig())
	ts := httptest.NewServer(s)
	defer ts.Close()

	resp, err := http.PostForm(ts.URL+"/login", url.Values{"password": {"wrong"}})
	if err != nil {
		t.Fatalf("PostForm error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestLoginSubmitCorrectPasswordIssuesCookieAndRedirects(t *testing.T) {
	s := NewServer(testConfig())
	ts := httptest.NewServer(s)
	defer ts.Close()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.PostForm(ts.URL+"/login", url.Values{"password": {"correct-horse"}})
	if err != nil {
		t.Fatalf("PostForm error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	found := false
	for _, c := range resp.Cookies() {
		if c.Name == "gameops_session" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected gameops_session cookie to be set on successful login")
	}
}

func TestLoginRateLimitedAfterTooManyAttempts(t *testing.T) {
	s := NewServer(testConfig())
	ts := httptest.NewServer(s)
	defer ts.Close()

	for i := 0; i < 5; i++ {
		resp, err := http.PostForm(ts.URL+"/login", url.Values{"password": {"wrong"}})
		if err != nil {
			t.Fatalf("PostForm error: %v", err)
		}
		resp.Body.Close()
	}
	resp, err := http.PostForm(ts.URL+"/login", url.Values{"password": {"wrong"}})
	if err != nil {
		t.Fatalf("PostForm error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after rate limit exceeded, got %d", resp.StatusCode)
	}
}

func TestRequireAuthRedirectsWithoutCookie(t *testing.T) {
	s := NewServer(testConfig())
	protected := s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("protected"))
	})
	ts := httptest.NewServer(protected)
	defer ts.Close()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/login" {
		t.Fatalf("expected redirect to /login, got %q", loc)
	}
}

func TestRequireAuthAllowsValidCookie(t *testing.T) {
	s := NewServer(testConfig())
	protected := s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("protected"))
	})
	ts := httptest.NewServer(protected)
	defer ts.Close()

	rec := httptest.NewRecorder()
	s.sessions.IssueCookie(rec)
	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "gameops_session" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("IssueCookie did not set gameops_session")
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with valid cookie, got %d", resp.StatusCode)
	}
}
