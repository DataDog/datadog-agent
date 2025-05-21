// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package runner

import (
	"fmt"
	"math"
	"os"

	"github.com/vishvananda/netns"

	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network"
	netnsutil "github.com/DataDog/datadog-agent/pkg/util/kernel/netns"
)

func createGatewayLookup(telemetryComp telemetryComponent.Component) (network.GatewayLookup, uint32, error) {
	rootNs, err := rootNsLookup()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to look up root network namespace: %w", err)
	}
	defer rootNs.Close()

	nsIno, err := netnsutil.GetInoForNs(rootNs)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get inode number: %w", err)
	}

	gatewayLookup := network.NewGatewayLookup(rootNsLookup, math.MaxUint32, telemetryComp)
	return gatewayLookup, nsIno, nil
}

func rootNsLookup() (netns.NsHandle, error) {
	return netns.GetFromPid(os.Getpid())
}
