package telemetry

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/lyzr/orchestrator/common/logger"
)

// Telemetry holds observability components
type Telemetry struct {
	log        *logger.Logger
	pprofAddr  string
	metricsAddr string
}

// New creates telemetry components
func New(pprofPort, metricsPort int, log *logger.Logger) *Telemetry {
	return &Telemetry{
		log:         log,
		pprofAddr:   fmt.Sprintf("localhost:%d", pprofPort),
		metricsAddr: fmt.Sprintf("localhost:%d", metricsPort),
	}
}

// Start starts telemetry endpoints
func (t *Telemetry) Start(ctx context.Context) error {
	// Start pprof server
	go func() {
		t.log.Info("pprof server starting", "addr", t.pprofAddr)
		if err := http.ListenAndServe(t.pprofAddr, nil); err != nil {
			t.log.Error("pprof server error", "error", err)
		}
	}()

	// TODO: Add Prometheus metrics endpoint on metricsAddr

	return nil
}

// RecordDuration records operation duration
func (t *Telemetry) RecordDuration(operation string, start time.Time) {
	duration := time.Since(start)
	t.log.Debug("operation completed",
		"operation", operation,
		"duration_ms", duration.Milliseconds(),
	)
}

// RecordEvent records a telemetry event
func (t *Telemetry) RecordEvent(event string, attrs map[string]any) {
	t.log.Info("telemetry_event",
		"event", event,
		"attrs", attrs,
	)
}