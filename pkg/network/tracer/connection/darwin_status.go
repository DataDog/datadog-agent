// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package connection

import (
	"strings"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/nstat"
)

const (
	darwinStatusErrorLimit = 256

	darwinSidecarDisabled = "disabled"
	darwinSidecarHealthy  = "healthy"
	darwinSidecarDegraded = "degraded"
	darwinSidecarStopped  = "stopped"

	// darwinPacketDegradedMinAttempts is the minimum inspected TCP/decode
	// attempts before unmatched/drop rate can mark enrichment degraded.
	darwinPacketDegradedMinAttempts = 20
	// darwinPacketDegradedUnmatchedRatio is the unmatched+ambiguous+decode
	// fraction that marks a still-running sidecar as degraded.
	darwinPacketDegradedUnmatchedRatio = 0.5
)

// DarwinTracerStatus is the bounded product-facing health summary for the
// active macOS connection backend.
type DarwinTracerStatus struct {
	// ActiveBackend is "nstat", "ebpfless", "unavailable", or "unknown".
	ActiveBackend string `json:"active_backend"`
	// ABIRevision is the private NStat ABI revision in use or attempted before fallback.
	ABIRevision int `json:"nstat_abi_revision"`
	// SourceHealthy reports whether the active connection source can serve data.
	SourceHealthy bool `json:"source_healthy"`
	// RuntimeFallback is true when requested NStat collection failed and the
	// eBPF-less backend took over. Inspect LastError for the fallback cause.
	RuntimeFallback bool `json:"runtime_fallback"`
	// PacketEnrichment is "healthy", "disabled", "degraded", "stopped", or "unavailable".
	PacketEnrichment string `json:"packet_enrichment"`
	// LibprocReconciler is "healthy", "disabled", or "unavailable".
	LibprocReconciler string `json:"libproc_reconciler"`
	// PacketMatchRate is unique-match / inspected packets when the sidecar is up.
	PacketMatchRate float64 `json:"packet_match_rate,omitempty"`
	// LastError is a bounded, single-line primary, fallback, or sidecar diagnostic.
	LastError string `json:"last_error,omitempty"`
}

type darwinStatusProvider interface {
	darwinStatus() DarwinTracerStatus
}

// GetDarwinTracerStatus returns a stable status even for the legacy backend.
func GetDarwinTracerStatus(tracer Tracer) DarwinTracerStatus {
	if provider, ok := tracer.(darwinStatusProvider); ok {
		return provider.darwinStatus()
	}
	if tracer.Type() == TracerTypeNStat {
		return nstatStatus()
	}
	return DarwinTracerStatus{
		ActiveBackend:     darwinBackendName(tracer.Type()),
		SourceHealthy:     true,
		PacketEnrichment:  darwinSidecarDisabled,
		LibprocReconciler: darwinSidecarDisabled,
	}
}

func darwinBackendName(tracerType TracerType) string {
	switch tracerType {
	case TracerTypeNStat:
		return "nstat"
	case TracerTypeDarwin, TracerTypeEbpfless:
		return "ebpfless"
	default:
		return "unknown"
	}
}

func boundedDarwinStatusError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ReplaceAll(err.Error(), "\n", " ")
	message = strings.ToValidUTF8(message, "�")
	if len(message) > darwinStatusErrorLimit {
		end := darwinStatusErrorLimit
		for !utf8.ValidString(message[:end]) {
			end--
		}
		return message[:end]
	}
	return message
}

func nstatStatus() DarwinTracerStatus {
	return DarwinTracerStatus{
		ActiveBackend:     "nstat",
		ABIRevision:       nstat.ABIRevision,
		SourceHealthy:     true,
		PacketEnrichment:  darwinSidecarDisabled,
		LibprocReconciler: darwinSidecarDisabled,
	}
}

func darwinSidecarStatus(requested, available bool, err error) string {
	if !requested {
		return darwinSidecarDisabled
	}
	if err != nil {
		return darwinSidecarStopped
	}
	if !available {
		return darwinSidecarDisabled
	}
	return darwinSidecarHealthy
}

func darwinPacketEnrichmentStatus(requested, available bool, err error, stats darwinPacketSidecarStats) string {
	if !requested {
		return darwinSidecarDisabled
	}
	if err != nil {
		if available {
			return darwinSidecarStopped
		}
		return darwinSidecarDisabled
	}
	if !available {
		return darwinSidecarDisabled
	}
	if stats.stopped {
		return darwinSidecarStopped
	}
	if packetEnrichmentDegraded(stats) {
		return darwinSidecarDegraded
	}
	return darwinSidecarHealthy
}

func packetEnrichmentDegraded(stats darwinPacketSidecarStats) bool {
	if stats.attempts < darwinPacketDegradedMinAttempts {
		return false
	}
	return float64(packetUnresolved(stats)) >= darwinPacketDegradedUnmatchedRatio*float64(stats.attempts)
}

func packetMatchRate(stats darwinPacketSidecarStats) float64 {
	if stats.attempts == 0 {
		return 0
	}
	matched := stats.attempts - packetUnresolved(stats)
	if matched < 0 {
		return 0
	}
	return float64(matched) / float64(stats.attempts)
}

func packetUnresolved(stats darwinPacketSidecarStats) int64 {
	return stats.unmatched + stats.ambiguous + stats.decodeErrors
}
