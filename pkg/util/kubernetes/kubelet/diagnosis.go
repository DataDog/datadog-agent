// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && kubelet

package kubelet

import (
	diagnoseComp "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
)

func init() {
	diagnoseComp.RegisterMetadataAvail("Kubelet availability", diagnose)
}

// diagnose the API server availability
func diagnose() error {
	_, err := GetKubeUtil()
	return err
}
