package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lyzr/orchestrator/common/logger"
)

// Server wraps HTTP server with graceful shutdown
type Server struct {
	httpServer *http.Server
	log        *logger.Logger
	name       string
}

// New creates a new server
func New(name string, port int, handler http.Handler, log *logger.Logger) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:         fmt.Sprintf(":%d", port),
			Handler:      handler,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		log:  log,
		name: name,
	}
}

// Start starts the server with graceful shutdown
func (s *Server) Start() error {
	// Channel to listen for errors
	serverErrors := make(chan error, 1)

	// Start HTTP server
	go func() {
		s.log.Info(fmt.Sprintf("%s starting", s.name), "addr", s.httpServer.Addr)
		serverErrors <- s.httpServer.ListenAndServe()
	}()

	// Channel to listen for interrupt signal
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Block until error or shutdown signal
	select {
	case err := <-serverErrors:
		return fmt.Errorf("server error: %w", err)

	case sig := <-shutdown:
		s.log.Info("shutdown signal received", "signal", sig.String())

		// Give outstanding requests time to complete
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := s.httpServer.Shutdown(ctx); err != nil {
			s.log.Error("graceful shutdown failed", "error", err)
			if err := s.httpServer.Close(); err != nil {
				return fmt.Errorf("could not stop server: %w", err)
			}
		}

		s.log.Info("shutdown complete")
	}

	return nil
}

// HealthHandler returns a simple health check handler
func HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	}
}