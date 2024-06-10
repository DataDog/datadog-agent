// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patch

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/telemetry"
)

// TargetObjKind represents the supported k8s object kinds
type TargetObjKind string

const (
	// KindCluster refers to k8s cluster
	KindCluster TargetObjKind = "cluster"
)

// Action is the action requested in the patch
type Action string

const (
	// EnableConfig instructs the patcher to apply the patch request
	EnableConfig Action = "enable"
	// DisableConfig instructs the patcher to apply the patch request
	DisableConfig Action = "disable"
	// DeleteConfig instruct the patcher to delete configuration
	DeleteConfig Action = "delete"
)

// K8sClusterTarget represents k8s target within a cluster
type K8sClusterTarget struct {
	ClusterName       string    `json:"cluster_name"`
	Enabled           *bool     `json:"enabled,omitempty"`
	EnabledNamespaces *[]string `json:"enabled_namespaces,omitempty"`
}

func (k *K8sClusterTarget) isEnabled() bool {
	return k.Enabled != nil && *k.Enabled
}

func (k *K8sClusterTarget) matchesCluster(name string) bool {
	return k.ClusterName == name
}


// K8sTarget represent the targetet k8s scope
type K8sTarget struct {
	ClusterTargets []K8sClusterTarget `json:"cluster_targets"`
}

// Request holds the required data to target a k8s object and apply library configuration
type Request struct {
	ID            string `json:"id"`
	Revision      int64  `json:"revision"`
	RcVersion     uint64 `json:"rc_version"`
	SchemaVersion string `json:"schema_version"`
	Action        Action `json:"action"`

	// Library parameters
	LibConfig common.LibConfig `json:"lib_config"`

	// Target k8s object
	K8sTarget *K8sTarget `json:"k8s_target_v2,omitempty"`
}

// Validate returns whether a patch request is applicable
func (pr Request) Validate(clusterName string) error {
	return pr.K8sTarget.validate(clusterName)
}

func (pr Request) getApmRemoteConfigEvent(err error, errorCode int) telemetry.ApmRemoteConfigEvent {
	env := ""
	if pr.LibConfig.Env != nil {
		env = *pr.LibConfig.Env
	}
	errorMessage := ""
	if err != nil {
		errorMessage = err.Error()
	}
	var clusterTargets []telemetry.K8sClusterTarget
	if pr.K8sTarget != nil {
		for _, t := range pr.K8sTarget.ClusterTargets {
			target := telemetry.K8sClusterTarget{
				ClusterName:       t.ClusterName,
				Enabled:           *t.Enabled,
				EnabledNamespaces: *t.EnabledNamespaces,
			}
			clusterTargets = append(clusterTargets, target)
		}
	}
	return telemetry.ApmRemoteConfigEvent{
		RequestType: "apm-remote-config-event",
		ApiVersion:  "v2",
		Payload: telemetry.ApmRemoteConfigEventPayload{
			Tags: telemetry.ApmRemoteConfigEventTags{
				Env:            env,
				RcId:           pr.ID,
				RcRevision:     pr.Revision,
				RcVersion:      pr.RcVersion,
				ClusterTargets: clusterTargets,
			},
			Error: telemetry.ApmRemoteConfigEventError{
				Code:    errorCode,
				Message: errorMessage,
			},
		},
	}
}

func (k K8sTarget) validate(clusterName string) error {
	if len(k.ClusterTargets) != 1 {
		return errors.New("does not target exactly one k8s cluster")
	}

	target := k.ClusterTargets[0]

	if !target.matchesCluster(clusterName) {
		return fmt.Errorf("target cluster name %q is different from the local one %q", target.ClusterName, clusterName)
	}

	if !target.isEnabled() {
		return errors.New("instrumentation is unset or disabled on the scope")
	}

	return nil
}
