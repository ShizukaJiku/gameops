// Package gateway implements `gameops gateway`: the HTTPS admin frontend
// that proxies to each configured gameops host over the existing frp
// tunnel (see design spec §3, §5).
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ShizukaJiku/gameops/internal/gamecontrol"
)

// InstanceInfo mirrors host.InstanceInfo's JSON shape without importing
// that package — HostClient talks to a host purely over HTTP, potentially a
// separately deployed binary on a separate machine.
type InstanceInfo struct {
	Name string `json:"name"`
	Game string `json:"game"`
}

// HostClient talks to one gameops host's HTTP API.
type HostClient struct {
	Name  string
	addr  string
	token string
	http  *http.Client
}

func NewHostClient(name, addr, token string) *HostClient {
	return &HostClient{Name: name, addr: addr, token: token, http: &http.Client{Timeout: 5 * time.Second}}
}

func (h *HostClient) ListInstances(ctx context.Context) ([]InstanceInfo, error) {
	var infos []InstanceInfo
	if err := h.doJSON(ctx, http.MethodGet, "/instances", &infos); err != nil {
		return nil, err
	}
	return infos, nil
}

func (h *HostClient) Status(ctx context.Context, instance string) (gamecontrol.Status, error) {
	var status gamecontrol.Status
	if err := h.doJSON(ctx, http.MethodGet, "/instances/"+instance+"/status", &status); err != nil {
		return gamecontrol.Status{}, err
	}
	return status, nil
}

func (h *HostClient) Start(ctx context.Context, instance string) error {
	return h.doJSON(ctx, http.MethodPost, "/instances/"+instance+"/start", nil)
}

func (h *HostClient) Stop(ctx context.Context, instance string) error {
	return h.doJSON(ctx, http.MethodPost, "/instances/"+instance+"/stop", nil)
}

func (h *HostClient) Restart(ctx context.Context, instance string) error {
	return h.doJSON(ctx, http.MethodPost, "/instances/"+instance+"/restart", nil)
}

func (h *HostClient) doJSON(ctx context.Context, method, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, "http://"+h.addr+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	resp, err := h.http.Do(req)
	if err != nil {
		return fmt.Errorf("gateway: host %s unreachable: %w", h.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gateway: host %s returned %d: %s", h.Name, resp.StatusCode, body)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
