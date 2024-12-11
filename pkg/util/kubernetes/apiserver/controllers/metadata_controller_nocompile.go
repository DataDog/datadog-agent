// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubeapiserver

// Package controllers is responsible for running the Kubernetes controllers
// needed by the Datadog Cluster Agent
package controllers

import (
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetPodMetadataNames is used when the API endpoint of the DCA to get the services of a pod is hit.
func GetPodMetadataNames(_, _, _ string) ([]string, error) {
	log.Errorf("GetPodMetadataNames not implemented %s", apiserver.ErrNotCompiled.Error())
	return nil, nil
}
