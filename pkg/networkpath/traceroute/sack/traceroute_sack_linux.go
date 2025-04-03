// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sack

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
)

// RunSackTraceroute fully executes a SACK traceroute using the given parameters
func RunSackTraceroute(ctx context.Context, p Params) (*common.Results, error) {
	sackResult, err := runSackTraceroute(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("sack traceroute failed: %w", err)
	}

	hops, err := common.ToHops(p.ParallelParams, sackResult.Hops)
	if err != nil {
		return nil, fmt.Errorf("sack traceroute ToHops failed: %w", err)
	}

	result := &common.Results{
		Source:     sackResult.LocalAddr.Addr().AsSlice(),
		SourcePort: sackResult.LocalAddr.Port(),
		Target:     p.Target.Addr().AsSlice(),
		DstPort:    p.Target.Port(),
		Hops:       hops,
		Tags:       []string{"tcp_method:sack"},
	}

	return result, nil
}
