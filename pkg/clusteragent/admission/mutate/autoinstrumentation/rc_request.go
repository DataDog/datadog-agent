// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// TargetObjKind represents the supported k8s object kinds
type TargetObjKind string

const (
	// KindCluster refers to k8s clusters
	KindCluster TargetObjKind = "cluster"
)

// Action is the action requested by the user
type Action string

const (
	// EnableConfig instructs the patcher to apply the patch request
	EnableConfig Action = "enable"
)

// Request holds the required data to target a k8s object and apply library configuration
type Request struct {
	ID            string `json:"id"`
	Revision      int64  `json:"revision"`
	RcVersion     uint64 `json:"rc_version"`
	SchemaVersion string `json:"schema_version"`
	Action        Action `json:"action"`

	// Library parameters
	LibConfig common.LibConfig `json:"lib_config"`

	K8sTargetV2 *K8sTargetV2 `json:"k8s_target_v2,omitempty"`
}

// K8sClusterTarget represents k8s target within a cluster
type K8sClusterTarget struct {
	ClusterName       string    `json:"cluster_name"`
	Enabled           *bool     `json:"enabled,omitempty"`
	EnabledNamespaces *[]string `json:"enabled_namespaces,omitempty"`
}

// K8sTargetV2 represent the targetet k8s scope
type K8sTargetV2 struct {
	ClusterTargets []K8sClusterTarget `json:"cluster_targets"`
}

// Response represents the result of applying RC config
type Response struct {
	ID        string            `json:"id"`
	Revision  int64             `json:"revision"`
	RcVersion uint64            `json:"rc_version"`
	Status    state.ApplyStatus `json:"status"`
}
