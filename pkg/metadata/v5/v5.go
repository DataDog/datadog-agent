// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build linux windows darwin

package v5

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/common"
	"github.com/DataDog/datadog-agent/pkg/metadata/gohai"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/metadata/resources"
)

// GetPayload returns the complete metadata payload as seen in Agent v5
func GetPayload(hostname string) *Payload {
	cp := common.GetPayload(hostname)
	hp := host.GetPayload(hostname)
	rp := resources.GetPayload(hostname)

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
