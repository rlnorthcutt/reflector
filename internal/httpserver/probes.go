package httpserver

import "net/http"

// handleHealthz is the liveness probe, toggleable via the admin API.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeProbeResult(w, s.Health.Healthy())
}

// handleReadyz is the readiness probe, toggleable via the admin API.
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	writeProbeResult(w, s.Health.Healthy())
}

func writeProbeResult(w http.ResponseWriter, healthy bool) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if !healthy {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("unhealthy\n"))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}
