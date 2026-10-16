// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && pcap && cgo

package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/network/capture"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() { registerModule(PacketCapture) }

// captureRequest is the JSON body accepted by POST /capture.
type captureRequest struct {
	Interface    string `json:"interface,omitempty"`
	BPFFilter    string `json:"bpfFilter,omitempty"`
	DurationSecs int    `json:"durationSecs"`
	MaxPackets   uint64 `json:"maxPackets,omitempty"`
	MaxBytes     uint64 `json:"maxBytes,omitempty"`
	SnapLen      uint32 `json:"snapLen,omitempty"`
}

// stopGracePeriod bounds how long the handler waits, beyond the requested
// duration, for the capturer's drain loop to exit before forcing Stop().
const stopGracePeriod = 5 * time.Second

// Limits enforced here, where the capture runs, whatever the caller sends.
// They match the Private Action Runner's, which validates first.
const (
	maxDurationSecs   = 120
	defaultMaxPackets = 2_000_000
	defaultMaxBytes   = 160 << 20
)

// drainOverrun records how long a capture ran past its requested duration,
// so stopGracePeriod can be tuned from observed values. A capture that ends
// early on a packet or byte cap records 0.
var drainOverrun = telemetryimpl.GetCompatComponent().NewHistogram(
	"packet_capture", "drain_overrun_seconds", []string{},
	"Time a capture ran past its requested duration before its drain loop exited",
	[]float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5},
)

// boundRequest rejects a duration outside 1..maxDurationSecs and lowers
// unset or larger caps to the defaults, so no request can disable them.
func boundRequest(r *captureRequest) error {
	if r.DurationSecs < 1 || r.DurationSecs > maxDurationSecs {
		return fmt.Errorf("durationSecs must be between 1 and %d, got %d", maxDurationSecs, r.DurationSecs)
	}
	if r.MaxPackets == 0 || r.MaxPackets > defaultMaxPackets {
		r.MaxPackets = defaultMaxPackets
	}
	if r.MaxBytes == 0 || r.MaxBytes > defaultMaxBytes {
		r.MaxBytes = defaultMaxBytes
	}
	return nil
}

type packetCapture struct{}

// PacketCapture is a factory for the packet capture module, which lets
// trusted local callers (e.g. the Private Action Runner) trigger a
// short-lived, header-only libpcap packet capture over the system-probe unix
// socket.
var PacketCapture = &module.Factory{
	Name: config.PacketCaptureModule,
	Fn: func(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
		return &packetCapture{}, nil
	},
	NeedsEBPF: func() bool {
		return false
	},
}

var _ module.Module = &packetCapture{}

func (p *packetCapture) GetStats() map[string]interface{} {
	return nil
}

func (p *packetCapture) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("POST /capture", handleCapture)
	return nil
}

func (p *packetCapture) Close() {}

// flushWriter serializes the capture's packet writes, made from its drain
// goroutine, with flushes made from the handler goroutine:
// http.ResponseWriter is not safe for concurrent use.
type flushWriter struct {
	mu sync.Mutex
	w  http.ResponseWriter
}

func (f *flushWriter) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.w.Write(p)
}

func (f *flushWriter) Flush() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if fl, ok := f.w.(http.Flusher); ok {
		fl.Flush()
	}
}

func handleCapture(w http.ResponseWriter, req *http.Request) {
	var reqBody captureRequest
	if err := json.NewDecoder(req.Body).Decode(&reqBody); err != nil {
		log.Errorf("packet_capture: invalid request body: %s", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := boundRequest(&reqBody); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	duration := time.Duration(reqBody.DurationSecs) * time.Second
	out := &flushWriter{w: w}

	cfg := capture.CaptureConfig{
		Filter:     reqBody.BPFFilter,
		Interface:  reqBody.Interface,
		Output:     out,
		Duration:   duration,
		MaxPackets: reqBody.MaxPackets,
		MaxBytes:   reqBody.MaxBytes,
		SnapLen:    reqBody.SnapLen,
	}

	capturer, err := capture.NewCapturer(cfg)
	if err != nil {
		log.Errorf("packet_capture: creating capturer: %s", err)
		http.Error(w, fmt.Sprintf("creating capturer: %s", err), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(req.Context(), duration+stopGracePeriod)
	defer cancel()

	// Response headers must be set before Start, whose first write (the PCAP
	// global header) sends them. If Start fails before writing, nothing has been
	// sent yet and the caller gets a proper error status instead of an empty 200.
	w.Header().Set("Content-Type", "application/vnd.tcpdump.pcap")
	w.Header().Set("Trailer", "X-Packet-Count, X-Bytes-Captured, X-Packets-Dropped, X-Headers-Truncated, X-Capture-Errors")

	startedAt := time.Now()
	if err := capturer.Start(ctx); err != nil {
		log.Errorf("packet_capture: starting capture: %s", err)
		w.Header().Del("Trailer")
		http.Error(w, fmt.Sprintf("starting capture: %s", err), http.StatusInternalServerError)
		return
	}

	// Send the status and the PCAP global header now. A quiet interface may not
	// write again for the whole capture, and the caller gives up if it sees no
	// response headers within its setup timeout. The drain loop is already
	// writing packets, so the flush must go through out.
	out.Flush()

	// The capture ends itself on Duration, MaxPackets or MaxBytes; ctx only
	// covers the caller disconnecting or the drain loop failing to exit.
	select {
	case <-capturer.Done():
	case <-ctx.Done():
	}
	drainOverrun.Observe(max(0, time.Since(startedAt)-duration).Seconds())

	if err := capturer.Stop(); err != nil {
		log.Errorf("packet_capture: stopping capture: %s", err)
	}

	stats := capturer.Stats()
	w.Header().Set("X-Packet-Count", strconv.FormatUint(stats.PacketsCaptured, 10))
	w.Header().Set("X-Bytes-Captured", strconv.FormatUint(stats.BytesCaptured, 10))
	w.Header().Set("X-Packets-Dropped", strconv.FormatUint(stats.PacketsDropped, 10))
	w.Header().Set("X-Headers-Truncated", strconv.FormatUint(stats.HeadersTruncated, 10))
	w.Header().Set("X-Capture-Errors", strconv.FormatUint(stats.Errors, 10))
}
