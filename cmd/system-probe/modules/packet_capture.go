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
	"time"

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

func handleCapture(w http.ResponseWriter, req *http.Request) {
	var reqBody captureRequest
	if err := json.NewDecoder(req.Body).Decode(&reqBody); err != nil {
		log.Errorf("packet_capture: invalid request body: %s", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if reqBody.DurationSecs <= 0 {
		http.Error(w, "durationSecs must be positive", http.StatusBadRequest)
		return
	}

	duration := time.Duration(reqBody.DurationSecs) * time.Second

	cfg := capture.CaptureConfig{
		Filter:     reqBody.BPFFilter,
		Interface:  reqBody.Interface,
		Output:     w,
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

	if err := capturer.Start(ctx); err != nil {
		log.Errorf("packet_capture: starting capture: %s", err)
		w.Header().Del("Trailer")
		http.Error(w, fmt.Sprintf("starting capture: %s", err), http.StatusInternalServerError)
		return
	}

	// Send the status and the PCAP global header now. A quiet interface may not
	// write again for the whole capture, and the caller gives up if it sees no
	// response headers within its setup timeout.
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// The capture ends itself on Duration, MaxPackets or MaxBytes; ctx only
	// covers the caller disconnecting or the drain loop failing to exit.
	select {
	case <-capturer.Done():
	case <-ctx.Done():
	}

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
