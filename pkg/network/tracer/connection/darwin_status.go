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

const darwinStatusErrorLimit = 256

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
	// PacketEnrichment is "healthy", "disabled", or "unavailable".
	PacketEnrichment string `json:"packet_enrichment"`
	// LibprocReconciler is "healthy", "disabled", or "unavailable".
	LibprocReconciler string `json:"libproc_reconciler"`
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
		PacketEnrichment:  "disabled",
		LibprocReconciler: "disabled",
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
		PacketEnrichment:  "disabled",
		LibprocReconciler: "disabled",
	}
}
