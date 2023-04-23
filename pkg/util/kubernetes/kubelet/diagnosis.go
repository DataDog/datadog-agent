// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && kubelet
// +build docker,kubelet

package kubelet

import (
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
)

func init() {
	diagnosis.RegisterMetadataAvail("Kubelet availability", diagnose)
}

// diagnose the API server availability
func diagnose() error {
	_, err := GetKubeUtil()
	return err
}
