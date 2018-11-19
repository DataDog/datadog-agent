// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build containerd

package containerd

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() {
	diagnosis.Register("Containerd availability", diagnose)
}

// diagnose the Containerd socket connectivity
func diagnose() error {
	ctx := context.Background()
	cu := InstanciateContainerdUtil()
	defer cu.Close()
	err := cu.EnsureServing(ctx)
	if err != nil {
		log.Error(err)
	}
	return err
}
