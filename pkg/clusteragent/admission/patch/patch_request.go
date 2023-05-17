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
)

// TargetObjKind represents the supported k8s object kinds
type TargetObjKind string

const (
	// KindDeployment refers to k8s deployment objects
	KindDeployment TargetObjKind = "deployment"
)

// Action is the action requested in the patch
type Action string

const (
	// StageConfig instructs the patcher to process the configuration without triggering a rolling update
	StageConfig Action = "stage"
	// EnableConfig instructs the patcher to apply the patch request
	EnableConfig Action = "enable"
	// DisableConfig instructs the patcher to disable library injection
	DisableConfig Action = "disable"
)

// PatchRequest holds the required data to target a k8s object and apply library configuration
type PatchRequest struct {
	ID            string `json:"id"`
	Revision      int64  `json:"revision"`
	SchemaVersion string `json:"schema_version"`
	Action        Action `json:"action"`

	// Library parameters
	LibConfig common.LibConfig `json:"lib_config"`

	// Target k8s object
	K8sTarget K8sTarget `json:"k8s_target"`
}

// Validate returns whether a patch request is applicable
func (pr PatchRequest) Validate(clusterName string) error {
	if pr.LibConfig.Language == "" {
		return errors.New("library language is empty")
	}
	if pr.LibConfig.Version == "" {
		return errors.New("library version is empty")
	}
	return pr.K8sTarget.validate(clusterName)
}

// K8sTarget represent the targetet k8s object
type K8sTarget struct {
	Cluster   string        `json:"cluster"`
	Kind      TargetObjKind `json:"kind"`
	Name      string        `json:"name"`
	Namespace string        `json:"namespace"`
}

// String returns a string representation of the targeted k8s object
func (k K8sTarget) String() string {
	return fmt.Sprintf("Obj %s/%s of kind %s", k.Namespace, k.Name, k.Kind)
}

func (k K8sTarget) validate(clusterName string) error {
	if k.Cluster != clusterName {
		return fmt.Errorf("target cluster name %q is different from the local one %q", k.Cluster, clusterName)
	}
	if k.Name == "" {
		return errors.New("target object name is empty")
	}
	if k.Namespace == "" {
		return errors.New("target object namespace is empty")
	}
	switch k.Kind {
	case KindDeployment:
	default:
		return fmt.Errorf("target kind %q is not supported", k.Kind)
	}
	return nil
}
