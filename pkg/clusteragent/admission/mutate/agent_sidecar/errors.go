// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

// VolumeAlreadyAttached indicates that a give volume has
// already been attached to a Pod's spec
type VolumeAlreadyAttached struct {
	volume string
}

// PathAlreadyMounted indicates that there is already
// a volume mount mounted on a container at the specified path
type PathAlreadyMounted struct {
	path string
}

func (e VolumeAlreadyAttached) Error() string {
	return e.volume + " is already attached"
}

func (e PathAlreadyMounted) Error() string {
	return "there is already a volume mounted at " + e.path
}
