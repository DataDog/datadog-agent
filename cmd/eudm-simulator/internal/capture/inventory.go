// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package capture

import (
	"errors"
	"math"
	"time"

	tc "github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

// Inventory copies a credential-free producer projection and makes observation
// and startup timestamps relative to the same origin as the other streams.
func (*Normalizer) Inventory(in *tc.Inventory, origin time.Time) (*tc.Inventory, error) {
	if in == nil {
		return nil, errors.New("missing inventory projection")
	}
	kinds := 0
	if in.Agent != nil {
		kinds++
	}
	if in.Host != nil {
		kinds++
	}
	if in.SystemInfo != nil {
		kinds++
	}
	if in.Hostname == "" || in.UUID == "" || in.Timestamp < origin.UnixNano() || kinds != 1 {
		return nil, errors.New("inventory lacks a current observation or a unique metadata type")
	}
	out := tc.CloneInventory(in)
	out.Timestamp -= origin.UnixNano()
	if a := out.Agent; a != nil {
		if a.AgentVersion == "" || a.Flavor != "agent" || a.InfrastructureMode != "end_user_device" || a.AgentStartupTimeMS <= 0 || a.AgentStartupTimeMS > in.Timestamp/int64(time.Millisecond) {
			return nil, errors.New("Agent inventory requires an observed EUDM Agent identity and startup time")
		}
		a.AgentStartupTimeMS -= origin.UnixMilli()
	}
	if h := out.Host; h != nil {
		if h.AgentVersion == "" || h.OS == "" || h.MemoryTotalKb == 0 || h.CPUFrequency < 0 || math.IsNaN(h.CPUFrequency) || math.IsInf(h.CPUFrequency, 0) {
			return nil, errors.New("host inventory lacks operating system or resource information")
		}
	}
	return out, nil
}
