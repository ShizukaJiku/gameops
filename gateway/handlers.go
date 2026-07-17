package gateway

import "net/http"

func (s *Server) hostByName(name string) (*HostClient, bool) {
	for _, h := range s.hosts {
		if h.Name == name {
			return h, true
		}
	}
	return nil, false
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	var refs []instanceRef
	for _, h := range s.hosts {
		infos, err := h.ListInstances(r.Context())
		if err != nil {
			// Host unreachable — its cards just don't appear this refresh;
			// the operator sees it missing rather than the whole dashboard
			// failing to load.
			continue
		}
		for _, info := range infos {
			refs = append(refs, instanceRef{Host: h.Name, Name: info.Name})
		}
	}
	renderDashboard(w, refs)
}

func (s *Server) handleFragment(w http.ResponseWriter, r *http.Request) {
	h, ok := s.hostByName(r.PathValue("host"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	name := r.PathValue("name")
	status, err := h.Status(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	renderFragment(w, h.Name, name, status)
}

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	h, ok := s.hostByName(r.PathValue("host"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	name := r.PathValue("name")

	var err error
	switch r.PathValue("action") {
	case "start":
		err = h.Start(r.Context(), name)
	case "stop":
		err = h.Stop(r.Context(), name)
	case "restart":
		err = h.Restart(r.Context(), name)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	status, err := h.Status(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	renderFragment(w, h.Name, name, status)
}
