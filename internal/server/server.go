// Package server composes and runs pc-control.
package server

import (
	"context"
	"errors"
)

var errNotImplemented = errors.New("pc-control server: not implemented")

// Server is a composed pc-control service.
type Server struct{}

// NewFromEnv loads configuration and composes a service.
func NewFromEnv() (*Server, error) {
	return nil, errNotImplemented
}

// Run serves the composed service until ctx is cancelled or serving fails.
func (s *Server) Run(ctx context.Context) error {
	return errNotImplemented
}
