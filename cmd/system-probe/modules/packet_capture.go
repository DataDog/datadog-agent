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
	"net"
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
	HeaderOnly   bool   `json:"headerOnly,omitempty"`
}

// pollInterval is how often the handler polls Stats() while waiting for the
// capture's own Duration/MaxPackets/MaxBytes bound to be reached, so Stop() can be
// invoked promptly rather than only after the full grace period.
const pollInterval = 200 * time.Millisecond

// stopGracePeriod bounds how long the handler waits, beyond the requested
// duration, for the capturer's internal drain loop to self-terminate before
// forcing Stop().
const stopGracePeriod = 5 * time.Second

type packetCapture struct{}

// PacketCapture is a factory for the packet capture module, which lets
// trusted local callers (e.g. the Private Action Runner) trigger a
// short-lived eBPF TC packet capture over the system-probe unix socket.
var PacketCapture = &module.Factory{
	Name: config.PacketCaptureModule,
	Fn: func(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
		return &packetCapture{}, nil
	},
	NeedsEBPF: func() bool {
		return true
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

	iface, err := resolveInterface(reqBody.Interface)
	if err != nil {
		log.Errorf("packet_capture: %s", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	duration := time.Duration(reqBody.DurationSecs) * time.Second

	cfg := capture.CaptureConfig{
		Filter:     reqBody.BPFFilter,
		Iface:      iface,
		Output:     w,
		Duration:   duration,
		MaxPackets: reqBody.MaxPackets,
		MaxBytes:   reqBody.MaxBytes,
		SnapLen:    reqBody.SnapLen,
		HeaderOnly: reqBody.HeaderOnly,
	}

	capturer, err := capture.NewCapturer(cfg)
	if err != nil {
		log.Errorf("packet_capture: creating capturer: %s", err)
		http.Error(w, fmt.Sprintf("creating capturer: %s", err), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.tcpdump.pcap")
	w.Header().Set("Trailer", "X-Packet-Count, X-Bytes-Captured, X-Packets-Dropped, X-Capture-Errors")
	w.WriteHeader(http.StatusOK)

	ctx, cancel := context.WithTimeout(req.Context(), duration+stopGracePeriod)
	defer cancel()

	if err := capturer.Start(ctx); err != nil {
		log.Errorf("packet_capture: starting capture on %s: %s", iface.Name, err)
		w.Header().Set("X-Capture-Errors", "1")
		return
	}

	waitForCapture(ctx, capturer, reqBody.MaxPackets)

	if err := capturer.Stop(); err != nil {
		log.Errorf("packet_capture: stopping capture on %s: %s", iface.Name, err)
	}

	stats := capturer.Stats()
	w.Header().Set("X-Packet-Count", strconv.FormatUint(stats.PacketsCaptured, 10))
	w.Header().Set("X-Bytes-Captured", strconv.FormatUint(stats.BytesCaptured, 10))
	w.Header().Set("X-Packets-Dropped", strconv.FormatUint(stats.PacketsDropped, 10))
	w.Header().Set("X-Capture-Errors", strconv.FormatUint(stats.Errors, 10))
}

// waitForCapture blocks until the capture's own Duration bound elapses (plus
// stopGracePeriod for teardown), the request context is cancelled (e.g. the
// caller disconnected), or MaxPackets is reached, whichever comes first.
func waitForCapture(ctx context.Context, capturer capture.Capturer, maxPackets uint64) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if maxPackets > 0 && capturer.Stats().PacketsCaptured >= maxPackets {
				return
			}
		}
	}
}

// resolveInterface returns the named interface, or the first non-loopback
// interface that is up if name is empty.
func resolveInterface(name string) (*net.Interface, error) {
	if name != "" {
		iface, err := net.InterfaceByName(name)
		if err != nil {
			return nil, fmt.Errorf("interface %q not found: %w", name, err)
		}
		return iface, nil
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("listing network interfaces: %w", err)
	}

	for i := range ifaces {
		iface := ifaces[i]
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		return &iface, nil
	}

	return nil, fmt.Errorf("no suitable network interface found")
}
