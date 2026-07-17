package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ShizukaJiku/gameops/internal/gamecontrol"
)

func TestHostClientListInstances(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Errorf("expected Authorization header with token, got %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/instances" {
			t.Errorf("expected path /instances, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode([]InstanceInfo{{Name: "minecraft", Game: "minecraft"}})
	}))
	defer ts.Close()

	c := NewHostClient("shizu-server", ts.Listener.Addr().String(), "secret-token")
	infos, err := c.ListInstances(context.Background())
	if err != nil {
		t.Fatalf("ListInstances error: %v", err)
	}
	if len(infos) != 1 || infos[0].Name != "minecraft" {
		t.Fatalf("unexpected infos: %+v", infos)
	}
}

func TestHostClientStatus(t *testing.T) {
	want := gamecontrol.Status{Online: true, PlayerCount: 1, MaxPlayers: 20, UptimeSec: 42}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(want)
	}))
	defer ts.Close()

	c := NewHostClient("shizu-server", ts.Listener.Addr().String(), "secret-token")
	got, err := c.Status(context.Background(), "minecraft")
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if got != want {
		t.Fatalf("expected %+v, got %+v", want, got)
	}
}

func TestHostClientStartStopRestartPostToRightPath(t *testing.T) {
	var gotPaths []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.Method+" "+r.URL.Path)
	}))
	defer ts.Close()

	c := NewHostClient("shizu-server", ts.Listener.Addr().String(), "secret-token")
	if err := c.Start(context.Background(), "minecraft"); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	if err := c.Stop(context.Background(), "minecraft"); err != nil {
		t.Fatalf("Stop error: %v", err)
	}
	if err := c.Restart(context.Background(), "minecraft"); err != nil {
		t.Fatalf("Restart error: %v", err)
	}

	want := []string{"POST /instances/minecraft/start", "POST /instances/minecraft/stop", "POST /instances/minecraft/restart"}
	if len(gotPaths) != len(want) {
		t.Fatalf("expected %d requests, got %d: %v", len(want), len(gotPaths), gotPaths)
	}
	for i := range want {
		if gotPaths[i] != want[i] {
			t.Errorf("request %d: expected %q, got %q", i, want[i], gotPaths[i])
		}
	}
}

func TestHostClientReturnsErrorOnNon200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "instance broken", http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := NewHostClient("shizu-server", ts.Listener.Addr().String(), "secret-token")
	if _, err := c.Status(context.Background(), "minecraft"); err == nil {
		t.Fatal("expected error for non-200 response")
	}
}

func TestHostClientReturnsErrorWhenUnreachable(t *testing.T) {
	c := NewHostClient("shizu-server", "127.0.0.1:1", "secret-token") // port 1: nothing listens there
	if _, err := c.Status(context.Background(), "minecraft"); err == nil {
		t.Fatal("expected error when host is unreachable")
	}
}

// TestHostClientActionSurvivesSlowHost proves the per-call timeout fix for
// Finding 1: Start/Stop/Restart get actionTimeout (90s), not readTimeout
// (5s), so a genuinely slow-but-successful host action (maintenance.Stop's
// clean-stop poll can legitimately take up to stopPollTimeout = 60s, see
// maintenance/maintenance.go) doesn't get reported to the admin as a failed
// request.
//
// The fake server here sleeps 6s before responding: longer than readTimeout
// (5s) so it would have failed under the old single-client-wide-5s-timeout
// behavior, but far under actionTimeout (90s) so the test doesn't have to
// wait anywhere near the real worst case. 6s is slow for a unit test, but is
// the minimum delay that actually distinguishes the two timeouts, so it's
// accepted here as the one place a slower test is justified for a real
// regression proof.
func TestHostClientActionSurvivesSlowHost(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow-host regression test in -short mode")
	}

	const delay = 6 * time.Second // > readTimeout (5s), << actionTimeout (90s)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c := NewHostClient("shizu-server", ts.Listener.Addr().String(), "secret-token")
	if err := c.Start(context.Background(), "minecraft"); err != nil {
		t.Fatalf("Start with a %s-delayed host should succeed under the 90s action timeout, got error: %v", delay, err)
	}
}
