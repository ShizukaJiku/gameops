// Package host implements `gameops host`'s HTTP API: one Server instance
// serves every game instance configured on this machine, reachable only by
// gameops gateway over the existing frp tunnel (see design spec §5) — never
// exposed directly to the internet, which is why main.go binds this to
// 127.0.0.1 only (see cmd/gameops/main.go's runHost).
package host

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/ShizukaJiku/gameops/internal/config"
	"github.com/ShizukaJiku/gameops/internal/gamecontrol"
)

type Server struct {
	token         string
	controllers   map[string]gamecontrol.GameController
	instanceGames map[string]string
	mux           *http.ServeMux
}

// InstanceInfo is what GET /instances returns for each configured
// instance — enough for the gateway to know what to ask status for without
// keeping its own copy of this host's game list.
type InstanceInfo struct {
	Name string `json:"name"`
	Game string `json:"game"`
}

func NewServer(cfg config.Config, token string) (*Server, error) {
	controllers := make(map[string]gamecontrol.GameController, len(cfg.Instances))
	instanceGames := make(map[string]string, len(cfg.Instances))
	for _, inst := range cfg.Instances {
		gc, err := gamecontrol.BuildController(inst)
		if err != nil {
			return nil, err
		}
		controllers[inst.Name] = gc
		instanceGames[inst.Name] = inst.Game
	}
	return newServerWithControllers(controllers, instanceGames, token), nil
}

// newServerWithControllers is the seam tests use to inject fakeController
// values instead of real ones built from config (which would need a live
// RCON/process on the machine running the tests).
func newServerWithControllers(controllers map[string]gamecontrol.GameController, instanceGames map[string]string, token string) *Server {
	s := &Server{token: token, controllers: controllers, instanceGames: instanceGames}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /instances", s.handleList)
	mux.HandleFunc("GET /instances/{name}/status", s.handleStatus)
	mux.HandleFunc("POST /instances/{name}/start", s.handleStart)
	mux.HandleFunc("POST /instances/{name}/stop", s.handleStop)
	mux.HandleFunc("POST /instances/{name}/restart", s.handleRestart)
	s.mux = mux
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.authenticate(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) authenticate(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return false
	}
	token := strings.TrimPrefix(auth, prefix)
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) == 1
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	infos := make([]InstanceInfo, 0, len(s.instanceGames))
	for name, game := range s.instanceGames {
		infos = append(infos, InstanceInfo{Name: name, Game: game})
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(infos)
}

func (s *Server) controllerFor(w http.ResponseWriter, r *http.Request) (gamecontrol.GameController, bool) {
	gc, ok := s.controllers[r.PathValue("name")]
	if !ok {
		http.NotFound(w, r)
		return nil, false
	}
	return gc, true
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	gc, ok := s.controllerFor(w, r)
	if !ok {
		return
	}
	status, err := gc.Status(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	s.runAction(w, r, gamecontrol.GameController.Start)
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	s.runAction(w, r, gamecontrol.GameController.Stop)
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	s.runAction(w, r, gamecontrol.GameController.Restart)
}

func (s *Server) runAction(w http.ResponseWriter, r *http.Request, action func(gamecontrol.GameController, context.Context) error) {
	gc, ok := s.controllerFor(w, r)
	if !ok {
		return
	}
	if err := action(gc, r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
