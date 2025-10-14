package metrics

import (
	"context"
	"runtime"
	"sync"
)

// SystemInfo holds static system information captured once at startup
type SystemInfo struct {
	OS              string `json:"os"`                // OS type (linux, darwin, windows)
	OSVersion       string `json:"os_version"`        // OS version/release
	Arch            string `json:"arch"`              // Architecture (amd64, arm64, etc.)
	Hostname        string `json:"hostname"`          // Machine hostname
	CPUCores        int    `json:"cpu_cores"`         // Physical CPU cores
	CPULogical      int    `json:"cpu_logical"`       // Logical CPUs (with hyperthreading)
	TotalMemoryMB   uint64 `json:"total_memory_mb"`   // Total system RAM in MB
	GoVersion       string `json:"go_version"`        // Go runtime version
	InContainer     bool   `json:"in_container"`      // Running in container (Docker/K8s)
	ContainerRuntime string `json:"container_runtime,omitempty"` // docker, containerd, etc.
}

var (
	systemInfo     *SystemInfo
	systemInfoOnce sync.Once
)

// GetSystemInfo returns cached system information (captured once)
func GetSystemInfo() *SystemInfo {
	systemInfoOnce.Do(func() {
		systemInfo = captureSystemInfo()
	})
	return systemInfo
}

// RuntimeMetrics captures memory and goroutine metrics for worker execution
type RuntimeMetrics struct {
	MemoryStartMB  float64
	MemoryPeakMB   float64
	MemoryEndMB    float64
	GoroutineStart int
	GoroutineEnd   int
}

// CaptureStart captures runtime metrics at the beginning of execution
// Context is provided for future extensions (tracing, cancellation, etc.)
func CaptureStart(ctx context.Context) *RuntimeMetrics {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return &RuntimeMetrics{
		MemoryStartMB:  float64(m.Alloc) / 1024 / 1024,
		GoroutineStart: runtime.NumGoroutine(),
	}
}

// Finalize completes the metrics capture at the end of execution
// Context is provided for future extensions (tracing, cancellation, etc.)
func (rm *RuntimeMetrics) Finalize(ctx context.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	rm.MemoryEndMB = float64(m.Alloc) / 1024 / 1024
	rm.GoroutineEnd = runtime.NumGoroutine()

	// Peak is the higher of start or end (for short operations)
	// For longer operations, this could be enhanced with periodic sampling
	if rm.MemoryEndMB > rm.MemoryStartMB {
		rm.MemoryPeakMB = rm.MemoryEndMB
	} else {
		rm.MemoryPeakMB = rm.MemoryStartMB
	}
}

// ToMap converts RuntimeMetrics to a map for storage/serialization
func (rm *RuntimeMetrics) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"memory_start_mb": rm.MemoryStartMB,
		"memory_peak_mb":  rm.MemoryPeakMB,
		"memory_end_mb":   rm.MemoryEndMB,
		"goroutine_start": rm.GoroutineStart,
		"goroutine_end":   rm.GoroutineEnd,
		"thread_count":    rm.GoroutineEnd, // Use goroutine count as thread count
	}
}

// ToMap converts SystemInfo to a map for storage/serialization
func (si *SystemInfo) ToMap() map[string]interface{} {
	m := map[string]interface{}{
		"os":              si.OS,
		"os_version":      si.OSVersion,
		"arch":            si.Arch,
		"hostname":        si.Hostname,
		"cpu_cores":       si.CPUCores,
		"cpu_logical":     si.CPULogical,
		"total_memory_mb": si.TotalMemoryMB,
		"go_version":      si.GoVersion,
		"in_container":    si.InContainer,
	}
	if si.ContainerRuntime != "" {
		m["container_runtime"] = si.ContainerRuntime
	}
	return m
}
