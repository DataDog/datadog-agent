// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && test

package http2

import (
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetHTTP2KernelTelemetry returns the HTTP2 kernel telemetry
func (p *Protocol) GetHTTP2KernelTelemetry() (*HTTP2Telemetry, error) {
	http2Telemetry := &HTTP2Telemetry{}
	var zero uint32

	mp, _, err := p.mgr.GetMap(telemetryMap)
	if err != nil {
		log.Errorf("unable to get http2 telemetry map: %s", err)
		return nil, err
	}

	if err := mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(http2Telemetry)); err != nil {
		log.Errorf("unable to lookup http2 telemetry map: %s", err)
		return nil, err
	}
	return http2Telemetry, nil
}
