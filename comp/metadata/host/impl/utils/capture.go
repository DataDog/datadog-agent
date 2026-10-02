// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package utils

import (
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

// CaptureMetadataSize conservatively charges the fields CopyCaptureMetadata
// allocates. APIKey and unrelated cloud/container/configuration data are never
// read by either helper, including when computing the reservation size.
func CaptureMetadataSize(common *CommonPayload, p *Payload) int64 {
	n := int64(2048 + len(common.AgentVersion) + len(common.UUID) + len(common.InternalHostname) + len(p.Os) + len(p.AgentFlavor))
	if stats := p.SystemStats; stats != nil {
		n += int64(len(stats.Machine) + len(stats.Platform))
		for _, value := range stats.Macver {
			if v, ok := any(value).(string); ok {
				n += int64(len(v))
			}
		}
		for _, value := range stats.Winver {
			if v, ok := any(value).(string); ok {
				n += int64(len(v))
			}
		}
	}
	if network := p.NetworkMeta; network != nil {
		n += int64(len(network.ID))
	}
	if tags := p.HostTags; tags != nil {
		n += int64(len(tags.System)) * int64(unsafe.Sizeof(""))
		for _, tag := range tags.System {
			n += int64(len(tag))
		}
	}
	return n
}

// CopyCaptureMetadata constructs a distinct projection from safe fields. It
// never marshals the original, which contains a legacy API key in its body.
func CopyCaptureMetadata(common *CommonPayload, p *Payload) *telemetrycapture.HostMetadata {
	result := &telemetrycapture.HostMetadata{
		AgentVersion: strings.Clone(common.AgentVersion), UUID: strings.Clone(common.UUID),
		Hostname: strings.Clone(common.InternalHostname), OS: strings.Clone(p.Os), AgentFlavor: strings.Clone(p.AgentFlavor),
	}
	if stats := p.SystemStats; stats != nil {
		result.CPUCores, result.Machine, result.Platform = int(stats.CPUCores), strings.Clone(stats.Machine), strings.Clone(stats.Platform)
		// The legacy tuple differs between platforms; consume only the fields
		// used by the sanitizer, leaving the old Python tuple/config data behind.
		for i, value := range stats.Macver {
			if v, ok := any(value).(string); ok {
				switch i {
				case 0:
					result.MacVersion = strings.Clone(v)
				case 2:
					result.MacMachine = strings.Clone(v)
				}
			}
		}
		if p.Os == "win32" || p.Os == "windows" {
			result.Windows = make([]string, 2)
			for i, value := range stats.Winver {
				if i >= len(result.Windows) {
					break
				}
				if v, ok := any(value).(string); ok {
					result.Windows[i] = strings.Clone(v)
				}
			}
		}
	}
	if network := p.NetworkMeta; network != nil {
		result.NetworkID = strings.Clone(network.ID)
	}
	if tags := p.HostTags; tags != nil {
		owned := make([]string, len(tags.System))
		for i, tag := range tags.System {
			owned[i] = strings.Clone(tag)
		}
		result.HostTags = map[string][]string{"system": owned}
	}
	return result
}
