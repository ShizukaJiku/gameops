package host

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ShizukaJiku/gameops/internal/gamecontrol"
)

type fakeController struct {
	status                       gamecontrol.Status
	statusErr                    error
	started, stopped, restarted bool
	actionErr                    error
}

func (f *fakeController) Status(ctx context.Context) (gamecontrol.Status, error) {
	return f.status, f.statusErr
}
func (f *fakeController) Start(ctx context.Context) error   { f.started = true; return f.actionErr }
func (f *fakeController) Stop(ctx context.Context) error    { f.stopped = true; return f.actionErr }
func (f *fakeController) Restart(ctx context.Context) error { f.restarted = true; return f.actionErr }

func TestServerRejectsMissingToken(t *testing.T) {
	s := newServerWithControllers(nil, nil, "secret-token")
	ts := httptest.NewServer(s)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/instances")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestServerRejectsWrongToken(t *testing.T) {
	s := newServerWithControllers(nil, nil, "secret-token")
	ts := httptest.NewServer(s)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/instances", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestServerListInstances(t *testing.T) {
	instanceGames := map[string]string{"minecraft": "minecraft"}
	s := newServerWithControllers(map[string]gamecontrol.GameController{"minecraft": &fakeController{}}, instanceGames, "secret-token")
	ts := httptest.NewServer(s)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/instances", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var infos []InstanceInfo
	if err := json.NewDecoder(resp.Body).Decode(&infos); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(infos) != 1 || infos[0].Name != "minecraft" || infos[0].Game != "minecraft" {
		t.Fatalf("unexpected instances: %+v", infos)
	}
}

func TestServerStatusReturnsControllerStatus(t *testing.T) {
	fake := &fakeController{status: gamecontrol.Status{Online: true, PlayerCount: 2, MaxPlayers: 20, UptimeSec: 120}}
	s := newServerWithControllers(map[string]gamecontrol.GameController{"minecraft": fake}, nil, "secret-token")
	ts := httptest.NewServer(s)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/instances/minecraft/status", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	defer resp.Body.Close()
	var status gamecontrol.Status
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if status != fake.status {
		t.Fatalf("expected %+v, got %+v", fake.status, status)
	}
}

func TestServerStatusUnknownInstanceReturns404(t *testing.T) {
	s := newServerWithControllers(nil, nil, "secret-token")
	ts := httptest.NewServer(s)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/instances/nope/status", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestServerStartStopRestartCallController(t *testing.T) {
	fake := &fakeController{}
	s := newServerWithControllers(map[string]gamecontrol.GameController{"minecraft": fake}, nil, "secret-token")
	ts := httptest.NewServer(s)
	defer ts.Close()

	for _, action := range []string{"start", "stop", "restart"} {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/instances/minecraft/"+action, nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Do(%s) error: %v", action, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d", action, resp.StatusCode)
		}
	}
	if !fake.started || !fake.stopped || !fake.restarted {
		t.Fatalf("expected all three actions to be called, got started=%v stopped=%v restarted=%v", fake.started, fake.stopped, fake.restarted)
	}
}

func TestServerActionReturnsErrorFromController(t *testing.T) {
	fake := &fakeController{actionErr: errors.New("boom")}
	s := newServerWithControllers(map[string]gamecontrol.GameController{"minecraft": fake}, nil, "secret-token")
	ts := httptest.NewServer(s)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/instances/minecraft/start", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}
