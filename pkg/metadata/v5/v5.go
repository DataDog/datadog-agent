// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows || darwin

package v5

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/common"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/metadata/internal/gohai"
	"github.com/DataDog/datadog-agent/pkg/metadata/internal/resources"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

// GetPayload returns the complete metadata payload as seen in Agent v5
func GetPayload(ctx context.Context, hostnameData hostname.Data) *Payload {
	cp := common.GetPayload(hostnameData.Hostname)
	hp := host.GetPayload(ctx, hostnameData)
	rp := resources.GetPayload(hostnameData.Hostname)

	p := &Payload{
		CommonPayload: CommonPayload{*cp},
		HostPayload:   HostPayload{*hp},
	}

	if rp != nil {
		p.ResourcesPayload = ResourcesPayload{*rp}
	}

	if config.Datadog.GetBool("enable_gohai") {
		p.GohaiPayload = GohaiPayload{MarshalledGohaiPayload{*gohai.GetPayload()}}
	}

	return p
}
