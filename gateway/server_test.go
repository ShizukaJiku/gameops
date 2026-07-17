package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ShizukaJiku/gameops/internal/gamecontrol"
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

func TestLoginRateLimitedEvenWithCorrectPassword(t *testing.T) {
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

	resp, err := http.PostForm(ts.URL+"/login", url.Values{"password": {"correct-horse"}})
	if err != nil {
		t.Fatalf("PostForm error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 even with the correct password once rate-limited, got %d", resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == "gameops_session" {
			t.Fatal("expected no session cookie to be issued when rate-limited, even with the correct password")
		}
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

func TestDashboardRequiresAuth(t *testing.T) {
	s := NewServer(testConfig())
	ts := httptest.NewServer(s)
	defer ts.Close()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected redirect to login, got %d", resp.StatusCode)
	}
}

func TestDashboardListsInstancesFromAllHosts(t *testing.T) {
	hostSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/instances" {
			json.NewEncoder(w).Encode([]InstanceInfo{{Name: "minecraft", Game: "minecraft"}})
			return
		}
		http.NotFound(w, r)
	}))
	defer hostSrv.Close()

	cfg := testConfig()
	cfg.Hosts = []gwconfig.HostEntry{{Name: "shizu-server", Addr: hostSrv.Listener.Addr().String(), Token: "tok"}}
	s := NewServer(cfg)
	cookie := loggedInCookie(t, s)

	ts := httptest.NewServer(s)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `id="instance-shizu-server-minecraft"`) {
		t.Fatalf("expected dashboard to contain instance card, got: %s", body)
	}
}

func TestFragmentReturnsInstanceStatus(t *testing.T) {
	hostSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(gamecontrol.Status{Online: true, PlayerCount: 2, MaxPlayers: 20, UptimeSec: 10})
	}))
	defer hostSrv.Close()

	cfg := testConfig()
	cfg.Hosts = []gwconfig.HostEntry{{Name: "shizu-server", Addr: hostSrv.Listener.Addr().String(), Token: "tok"}}
	s := NewServer(cfg)
	cookie := loggedInCookie(t, s)

	ts := httptest.NewServer(s)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/hosts/shizu-server/instances/minecraft/fragment", nil)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "2/20") {
		t.Fatalf("expected fragment to show 2/20 players, got: %s", body)
	}
}

func TestActionCallsHostAndReturnsUpdatedFragment(t *testing.T) {
	var startCalled bool
	hostSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/instances/minecraft/start" {
			startCalled = true
			w.Write([]byte(`{"ok":true}`))
			return
		}
		json.NewEncoder(w).Encode(gamecontrol.Status{Online: true, PlayerCount: 0, MaxPlayers: 20})
	}))
	defer hostSrv.Close()

	cfg := testConfig()
	cfg.Hosts = []gwconfig.HostEntry{{Name: "shizu-server", Addr: hostSrv.Listener.Addr().String(), Token: "tok"}}
	s := NewServer(cfg)
	cookie := loggedInCookie(t, s)

	ts := httptest.NewServer(s)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/hosts/shizu-server/instances/minecraft/start", nil)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	defer resp.Body.Close()
	if !startCalled {
		t.Fatal("expected the host's start endpoint to be called")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func loggedInCookie(t *testing.T, s *Server) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	s.sessions.IssueCookie(rec)
	for _, c := range rec.Result().Cookies() {
		if c.Name == "gameops_session" {
			return c
		}
	}
	t.Fatal("no session cookie issued")
	return nil
}
