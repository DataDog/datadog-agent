// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build freebsd || netbsd || openbsd || solaris || dragonfly

package v5

import (
	"github.com/DataDog/datadog-agent/pkg/metadata/common"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
)

// GetPayload returns the complete metadata payload as seen in Agent v5.
// Note: gohai can't be used on the platforms this module builds for
func GetPayload(hostname string) *Payload {
	cp := common.GetPayload(hostname)
	hp := host.GetPayload(hostname)

	return &Payload{
		CommonPayload: CommonPayload{*cp},
		HostPayload:   HostPayload{*hp},
	}
}
