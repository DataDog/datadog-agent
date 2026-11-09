// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package sender

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/network"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	netnsutil "github.com/DataDog/datadog-agent/pkg/util/kernel/netns"
)

func getNetworkID(ctx context.Context) (id string, err error) {
	err = netnsutil.WithRootNS(kernel.ProcFSRoot(), func() error {
		id, err = network.GetNetworkID(ctx)
		return err
	})
	return
}
