// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import "fmt"

type VolumeAlreadyAttached struct {
	volume string
}

type PathAlreadyMounted struct {
	path string
}

func (e VolumeAlreadyAttached) Error() string {
	return fmt.Sprintf("%s is already attached", e.volume)
}

func (e PathAlreadyMounted) Error() string {
	return fmt.Sprintf("there is already a volume mounted at %s", e.path)
}
