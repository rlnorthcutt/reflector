package httpserver

import (
	"net/http"
	"strconv"
	"time"

	"github.com/rlnorthcutt/reflector/internal/envelope"
	"github.com/rlnorthcutt/reflector/internal/negotiate"
)

// handleStatus responds instantly with the requested status code.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	code, err := strconv.Atoi(r.PathValue("code"))
	if err != nil || code < 100 || code > 599 {
		http.Error(w, "invalid status code", http.StatusBadRequest)
		return
	}
	w.WriteHeader(code)
}

// handleDelay waits the requested duration before responding with the
// envelope, releasing resources immediately if the client disconnects.
func (s *Server) handleDelay(w http.ResponseWriter, r *http.Request) {
	dur, err := time.ParseDuration(r.PathValue("duration"))
	if err != nil || dur < 0 {
		http.Error(w, "invalid duration", http.StatusBadRequest)
		return
	}

	timer := time.NewTimer(dur)
	defer timer.Stop()

	select {
	case <-timer.C:
		env := envelope.Build(r, s.ID)
		negotiate.Write(w, r, http.StatusOK, env)
	case <-r.Context().Done():
		// Client disconnected; nothing left to do.
	}
}
