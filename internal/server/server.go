// Package server composes and runs pc-control.
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/luisrpp/pc-control/internal/config"
	"github.com/luisrpp/pc-control/internal/httpapi"
	"github.com/luisrpp/pc-control/internal/shutdown"
	"github.com/luisrpp/pc-control/internal/sshshutdown"
	"github.com/luisrpp/pc-control/internal/wake"
	"github.com/luisrpp/pc-control/internal/wol"
)

// Server is a composed pc-control service.
type Server struct {
	httpServer *http.Server
}

// NewFromEnv loads configuration and composes a service.
func NewFromEnv() (*Server, error) {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return nil, fmt.Errorf("pc-control server configuration: %w", err)
	}
	shutdownCfg, err := config.LoadShutdownFromEnv()
	if err != nil {
		return nil, fmt.Errorf("pc-control server configuration: %w", err)
	}

	sender := wol.NewSender(cfg.WOLDestination, cfg.WOLPort, cfg.WOLMAC)
	shutdownAdapter := sshshutdown.New(sshshutdown.Config{
		Host:           shutdownCfg.SSHHost,
		Port:           shutdownCfg.SSHPort,
		User:           shutdownCfg.SSHUser,
		PrivateKeyPath: shutdownCfg.SSHPrivateKeyPath,
		KnownHostsPath: shutdownCfg.SSHKnownHostsPath,
		Timeout:        shutdownCfg.SSHTimeout,
	})
	if err := shutdownAdapter.Validate(); err != nil {
		return nil, errors.New("pc-control server configuration: invalid shutdown SSH material")
	}
	handler := httpapi.NewHandler(wake.New(sender), shutdown.New(shutdownAdapter))
	return &Server{httpServer: &http.Server{
		Addr:    cfg.HTTPListenAddr,
		Handler: handler,
	}}, nil
}

// Run serves the composed service until ctx is cancelled or serving fails.
func (s *Server) Run(ctx context.Context) error {
	if s == nil || s.httpServer == nil {
		return errors.New("pc-control server: not initialized")
	}

	serveResult := make(chan error, 1)
	go func() {
		serveResult <- s.httpServer.ListenAndServe()
	}()

	select {
	case err := <-serveResult:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			return err
		}
		err := <-serveResult
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
