package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
