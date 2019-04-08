// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !kubelet !kubeapiserver

package hostinfo

// GetTags gets the tags from the kubernetes apiserver
func GetTags() ([]string, error) {
	return nil, nil
}
