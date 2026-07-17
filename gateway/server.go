package gateway

import (
	"net"
	"net/http"
	"time"

	"github.com/ShizukaJiku/gameops/internal/gwconfig"
	"github.com/ShizukaJiku/gameops/internal/webauth"
)

const (
	sessionTTL       = 24 * time.Hour
	loginMaxAttempts = 5
	loginWindow      = 5 * time.Minute
)

// Server is the gameops gateway HTTP server: one process serves the admin
// frontend and proxies actions to each configured host.
type Server struct {
	hosts        []*HostClient
	passwordHash string
	sessions     *webauth.SessionManager
	limiter      *webauth.RateLimiter
	mux          *http.ServeMux
}

func NewServer(cfg *gwconfig.GatewayConfig) *Server {
	s := &Server{
		passwordHash: cfg.AdminPasswordHash,
		sessions:     webauth.NewSessionManager([]byte(cfg.SessionSecret), sessionTTL),
		limiter:      webauth.NewRateLimiter(loginMaxAttempts, loginWindow),
	}
	for _, h := range cfg.Hosts {
		s.hosts = append(s.hosts, NewHostClient(h.Name, h.Addr, h.Token))
	}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /login", s.handleLoginPage)
	s.mux.HandleFunc("POST /login", s.handleLoginSubmit)
	s.mux.HandleFunc("GET /", s.requireAuth(s.handleDashboard))
	s.mux.HandleFunc("GET /hosts/{host}/instances/{name}/fragment", s.requireAuth(s.handleFragment))
	s.mux.HandleFunc("POST /hosts/{host}/instances/{name}/{action}", s.requireAuth(s.handleAction))
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// requireAuth wraps next so it only runs for requests carrying a valid
// session cookie — anything else is redirected to /login.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.sessions.Validate(r) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	renderLogin(w, http.StatusOK, "")
}

func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.limiter.Allow(clientIP(r)) {
		renderLogin(w, http.StatusTooManyRequests, "Demasiados intentos, esperá unos minutos.")
		return
	}

	if !webauth.CheckPassword(s.passwordHash, r.FormValue("password")) {
		renderLogin(w, http.StatusUnauthorized, "Password incorrecta.")
		return
	}

	s.sessions.IssueCookie(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
