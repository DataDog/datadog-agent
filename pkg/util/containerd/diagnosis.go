// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd
// +build containerd

package containerd

import (
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
)

func init() {
	diagnosis.RegisterMetadataAvail("Containerd availability", diagnose)
}

// diagnose the Containerd socket connectivity
func diagnose() error {
	client, err := NewContainerdUtil()
	if err != nil {
		return err
	}

	return client.Close()
}
