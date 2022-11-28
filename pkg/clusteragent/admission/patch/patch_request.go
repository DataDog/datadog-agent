// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

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
	// ApplyConfig instructs the patcher to apply the patch request
	ApplyConfig Action = "apply"
	// DisableInjection instructs the patcher to disable library injection
	DisableInjection Action = "disable"
)

// PatchRequest holds the required data to target a k8s object and apply library configuration
type PatchRequest struct {
	Action    Action           `yaml:"action"`
	K8sTarget K8sTarget        `yaml:"k8s_target"`
	LibID     LibID            `yaml:"lib_id"`
	LibConfig common.LibConfig `yaml:"lib_config"`
}

// Validate returns whether a patch request is applicable
func (pr PatchRequest) Validate(clusterName string) error {
	if err := pr.K8sTarget.validate(clusterName); err != nil {
		return err
	}
	return pr.LibID.validate()
}

// K8sTarget represent the targetet k8s object
type K8sTarget struct {
	ClusterName string        `yaml:"cluster_name"`
	Kind        TargetObjKind `yaml:"kind"`
	Name        string        `yaml:"name"`
	Namespace   string        `yaml:"namespace"`
}

// String returns a string representation of the targeted k8s object
func (k K8sTarget) String() string {
	return fmt.Sprintf("Obj %s/%s of kind %s", k.Namespace, k.Name, k.Kind)
}

func (k K8sTarget) validate(clusterName string) error {
	if k.ClusterName != clusterName {
		return fmt.Errorf("target cluster name %q is different from the local one %q", k.ClusterName, clusterName)
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

// LibID hold the minimal information to inject a library
type LibID struct {
	Language string `yaml:"language"`
	Version  string `yaml:"version"`
}

func (li LibID) validate() error {
	if li.Language == "" {
		return errors.New("library language is empty")
	}
	if li.Version == "" {
		return errors.New("library version is empty")
	}
	return nil
}
