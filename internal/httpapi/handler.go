// Package httpapi contains the HTTP inbound adapter.
package httpapi

import (
	"net/http"

	"github.com/luisrpp/pc-control/internal/wake"
)

// Handler is the HTTP adapter for the wake use case.
type Handler struct {
	waker *wake.UseCase
}

// NewHandler creates an HTTP adapter for waker.
func NewHandler(waker *wake.UseCase) http.Handler {
	return &Handler{waker: waker}
}

// ServeHTTP handles an HTTP request.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "pc-control HTTP adapter: not implemented", http.StatusNotImplemented)
}
