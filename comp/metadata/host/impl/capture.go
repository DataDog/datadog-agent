// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin || aix

package hostimpl

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/metadata/host/impl/utils"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

var _ serializer.HostMetadataCapture = (*Payload)(nil)

// CaptureMetadataSchedule is set only by a real provider collection. Cached
// endpoint and flare payloads have no collection boundary and cannot arm a copy.
func (p *Payload) CaptureMetadataSchedule() (time.Time, time.Duration) {
	return p.captureCollectedAt, p.captureCadence
}

// CaptureMetadataSize measures only the credential-free projection, without
// encoding or allocating a copy of the original payload.
func (p *Payload) CaptureMetadataSize() int64 {
	return utils.CaptureMetadataSize(&p.CommonPayload, &p.Payload) + utils.CaptureGohaiSize(p.captureGohaiFields())
}

// CopyCaptureMetadata is called only after reserving its entire owned size.
func (p *Payload) CopyCaptureMetadata() *telemetrycapture.HostMetadata {
	projection := utils.CopyCaptureMetadata(&p.CommonPayload, &p.Payload)
	projection.Gohai = utils.CopyCaptureGohai(p.captureGohaiFields())
	return projection
}

func (p *Payload) captureGohaiFields() utils.GohaiFields {
	if p.nativeGohai == nil || p.nativeGohai.Gohai == nil {
		return utils.GohaiFields{}
	}
	return utils.GohaiFields{
		CPU: p.nativeGohai.Gohai.CPU, Memory: p.nativeGohai.Gohai.Memory,
		Platform: p.nativeGohai.Gohai.Platform, Network: p.nativeGohai.Gohai.Network,
	}
}
