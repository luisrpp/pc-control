// Package httpapi contains the HTTP inbound adapter.
package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/luisrpp/pc-control/internal/shutdown"
	"github.com/luisrpp/pc-control/internal/wake"
)

// Handler is the HTTP adapter for wake and shutdown use cases.
type Handler struct {
	waker      *wake.UseCase
	shutdowner *shutdown.UseCase
}

// NewHandler creates an HTTP adapter for waker and, when provided, shutdowner.
func NewHandler(waker *wake.UseCase, shutdowners ...*shutdown.UseCase) http.Handler {
	handler := &Handler{waker: waker}
	if len(shutdowners) > 0 {
		handler.shutdowner = shutdowners[0]
	}
	return handler
}

// ServeHTTP handles an HTTP request.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case exactPath(r, "/v1/wake"):
		h.serveWake(w, r)
	case exactPath(r, "/v1/shutdown"):
		h.serveShutdown(w, r)
	default:
		writeError(w, r, http.StatusNotFound, "not_found", "requested endpoint was not found")
	}
}

func exactPath(r *http.Request, path string) bool {
	return r.URL.Path == path && r.URL.EscapedPath() == path
}

func (h *Handler) serveWake(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method is not allowed for this endpoint")
		return
	}
	if r.URL.RawQuery != "" || !emptyBody(r.Body) {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "wake commands do not accept input")
		return
	}
	if err := h.waker.Wake(); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "wake_failed", "wake request could not be sent")
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]string{"result": "sent"})
}

func (h *Handler) serveShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method is not allowed for this endpoint")
		return
	}
	if r.URL.RawQuery != "" || !emptyBody(r.Body) {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "shutdown commands do not accept input")
		return
	}
	if h.shutdowner == nil || h.shutdowner.Shutdown() != nil {
		writeError(w, r, http.StatusServiceUnavailable, "shutdown_failed", "shutdown request could not be initiated")
		return
	}
	writeJSON(w, r, http.StatusAccepted, map[string]string{"result": "initiated"})
}

func emptyBody(body io.Reader) bool {
	if body == nil {
		return true
	}
	var buffer [1]byte
	for {
		n, err := body.Read(buffer[:])
		if n > 0 {
			return false
		}
		if err == io.EOF {
			return true
		}
		if err != nil {
			return false
		}
	}
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, r, status, map[string]map[string]string{
		"error": {"code": code, "message": message},
	})
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, payload any) {
	if r.Method == http.MethodHead {
		w.WriteHeader(status)
		return
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(encoded)
}
