// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build linux windows darwin

package v5

import (
	"path"

	"github.com/DataDog/datadog-agent/pkg/metadata/common"
	"github.com/DataDog/datadog-agent/pkg/metadata/gohai"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/metadata/resources"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// GetPayload returns the complete metadata payload as seen in Agent v5
func GetPayload(hostname string) *Payload {
	cp := common.GetPayload()
	hp := host.GetPayload(hostname)
	rp := resources.GetPayload(hostname)
	gp := gohai.GetPayload()
	payload := Payload{
		CommonPayload:    CommonPayload{*cp},
		HostPayload:      HostPayload{*hp},
		ResourcesPayload: ResourcesPayload{*rp},
		GohaiPayload:     GohaiPayload{MarshalledGohaiPayload{*gp}},
	}

	// Cache the metadata for use in other payload
	key := path.Join(util.AgentCachePrefix, "metav5")
	util.Cache.Set(key, payload, util.NoExpiration)

	return &payload
}
