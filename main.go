package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

// Configuration constants for better maintainability
const (
	// ServerPort is the port number the HTTP server listens on
	ServerPort = ":1122"
	// MetricsUpdateInterval is how often system metrics are collected and sent
	MetricsUpdateInterval = time.Second
)

// main initializes the HTTP server with structured logging and starts serving
// Server-Sent Events for real-time system monitoring
func main() {
	// Initialize structured logger with JSON format for better parsing
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("Starting SSE Event Server",
		"port", ServerPort,
		"update_interval", MetricsUpdateInterval)

	// Register the SSE handler endpoint
	http.HandleFunc("/events", sseHandler)

	slog.Info("Server ready to accept connections", "endpoint", "/events")

	// Start the HTTP server
	if err := http.ListenAndServe(ServerPort, nil); err != nil {
		slog.Error("Failed to start server", "error", err, "port", ServerPort)
		os.Exit(1)
	}
}

// sseHandler handles Server-Sent Events connections for real-time system monitoring.
// It streams CPU and memory statistics to connected clients every second.
func sseHandler(w http.ResponseWriter, r *http.Request) {
	// Set up SSE headers and CORS
	setSSEHeaders(w)

	clientIP := getClientIP(r)
	slog.Info("New SSE client connected", "client_ip", clientIP, "user_agent", r.UserAgent())

	// Create tickers for periodic metric collection
	memoryTicker := time.NewTicker(MetricsUpdateInterval)
	defer memoryTicker.Stop()

	cpuTicker := time.NewTicker(MetricsUpdateInterval)
	defer cpuTicker.Stop()

	// Monitor client connection status
	clientDisconnected := r.Context().Done()

	// Get response controller for flushing data
	responseController := http.NewResponseController(w)

	// Main event loop
	for {
		select {
		case <-clientDisconnected:
			slog.Info("SSE client disconnected", "client_ip", clientIP)
			return

		case <-memoryTicker.C:
			if err := sendMemoryMetrics(w, responseController); err != nil {
				slog.Error("Failed to send memory metrics", "error", err, "client_ip", clientIP)
				return
			}

		case <-cpuTicker.C:
			if err := sendCPUMetrics(w, responseController); err != nil {
				slog.Error("Failed to send CPU metrics", "error", err, "client_ip", clientIP)
				return
			}
		}
	}
}

// setSSEHeaders configures the HTTP response headers required for Server-Sent Events
// and enables CORS for cross-origin requests
func setSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
}

// getClientIP extracts the client IP address from the HTTP request,
// checking for common proxy headers first
func getClientIP(r *http.Request) string {
	// Check for forwarded IP from proxy
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	// Fall back to remote address
	return r.RemoteAddr
}

// sendMemoryMetrics collects current memory statistics and sends them as SSE data
func sendMemoryMetrics(w http.ResponseWriter, rc *http.ResponseController) error {
	memStats, err := mem.VirtualMemory()
	if err != nil {
		return fmt.Errorf("failed to get memory statistics: %w", err)
	}

	// Convert bytes to MB for better readability
	totalMB := float64(memStats.Total) / 1024 / 1024
	usedMB := float64(memStats.Used) / 1024 / 1024

	// Send formatted memory data as SSE event
	_, err = fmt.Fprintf(w, "event:mem\ndata:Total: %.2f MB, Used: %.2f MB, Perc: %.2f%%\n\n",
		totalMB, usedMB, memStats.UsedPercent)
	if err != nil {
		return fmt.Errorf("failed to write memory data: %w", err)
	}

	// Flush data to client immediately
	if err := rc.Flush(); err != nil {
		return fmt.Errorf("failed to flush memory data: %w", err)
	}

	slog.Debug("Memory metrics sent",
		"total_mb", totalMB,
		"used_mb", usedMB,
		"used_percent", memStats.UsedPercent)

	return nil
}

// sendCPUMetrics collects current CPU statistics and sends them as SSE data
func sendCPUMetrics(w http.ResponseWriter, rc *http.ResponseController) error {
	cpuStats, err := cpu.Times(false)
	if err != nil {
		return fmt.Errorf("failed to get CPU statistics: %w", err)
	}

	if len(cpuStats) == 0 {
		return fmt.Errorf("no CPU statistics available")
	}

	// Use first CPU core statistics (represents overall system)
	firstCore := cpuStats[0]

	// Send formatted CPU data as SSE event
	_, err = fmt.Fprintf(w, "event:cpu\ndata:User: %.2f, Sys: %.2f, Idle: %.2f\n\n",
		firstCore.User, firstCore.System, firstCore.Idle)
	if err != nil {
		return fmt.Errorf("failed to write CPU data: %w", err)
	}

	// Flush data to client immediately
	if err := rc.Flush(); err != nil {
		return fmt.Errorf("failed to flush CPU data: %w", err)
	}

	slog.Debug("CPU metrics sent",
		"user", firstCore.User,
		"system", firstCore.System,
		"idle", firstCore.Idle)

	return nil
}
