package app

import (
	"banner-fingerprint/internal/config"
	"banner-fingerprint/internal/fingerprint"
	"banner-fingerprint/internal/httpapi"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

func Serve(ctx context.Context, c config.Server, logger *slog.Logger) error {
	engine, err := fingerprint.Load(c.Rules)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", c.Addr)
	if err != nil {
		return err
	}
	var ready atomic.Bool
	mux := httpapi.Router(engine, c, ready.Load)
	s := &http.Server{
		Handler:           httpapi.Middleware(logger, c.MaxBodyBytes, c.MaxConcurrent, mux),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10,
	}
	done := make(chan error, 1)
	ready.Store(true)
	logger.Info("server_ready", "addr", listener.Addr().String(), "rules", c.Rules)
	go func() { done <- s.Serve(listener) }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		ready.Store(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.Shutdown(shutdownCtx); err != nil {
			_ = s.Close()
			return err
		}
		logger.Info("server_stopped")
		return nil
	}
}
