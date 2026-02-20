// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package kubernetes

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type CreateFields struct {
	DryRun          string `json:"dryRun,omitempty"`
	FieldManager    string `json:"fieldManager,omitempty"`
	FieldValidation string `json:"fieldValidation,omitempty"`
}

func MetaCreate(inputs *CreateFields) metav1.CreateOptions {
	if inputs == nil {
		return metav1.CreateOptions{}
	}
	return metav1.CreateOptions{
		DryRun:          dryRunFlag(inputs.DryRun),
		FieldManager:    inputs.FieldManager,
		FieldValidation: inputs.FieldValidation,
	}
}

type DeleteFields struct {
	Name               string                      `json:"name,omitempty"`
	GracePeriodSeconds *int64                      `json:"gracePeriodSeconds,omitempty"`
	PropagationPolicy  *metav1.DeletionPropagation `json:"propagationPolicy,omitempty"`
	DryRun             string                      `json:"dryRun,omitempty"`
}

func MetaDelete(inputs *DeleteFields) metav1.DeleteOptions {
	if inputs == nil {
		return metav1.DeleteOptions{}
	}
	return metav1.DeleteOptions{
		DryRun:             dryRunFlag(inputs.DryRun),
		GracePeriodSeconds: inputs.GracePeriodSeconds,
		PropagationPolicy:  inputs.PropagationPolicy,
	}
}

type GetFields struct {
	Name string `json:"name,omitempty"`
}

func MetaGet(_ *GetFields) metav1.GetOptions {
	return metav1.GetOptions{}
}

type ListFields struct {
	FieldSelector string `json:"fieldSelector,omitempty"`
	LabelSelector string `json:"labelSelector,omitempty"`
	Limit         int64  `json:"limit,omitempty"`
}

func MetaList(inputs *ListFields) metav1.ListOptions {
	if inputs == nil {
		return metav1.ListOptions{}
	}
	return metav1.ListOptions{
		FieldSelector: inputs.FieldSelector,
		LabelSelector: inputs.LabelSelector,
		Limit:         inputs.Limit,
	}
}

type PatchFields struct {
	Name            string                   `json:"name,omitempty"`
	DryRun          string                   `json:"dryRun,omitempty"`
	FieldManager    string                   `json:"fieldManager,omitempty"`
	FieldValidation string                   `json:"fieldValidation,omitempty"`
	Body            []map[string]interface{} `json:"body,omitempty"`
	Force           *bool                    `json:"force,omitempty"`
}

func MetaPatch(inputs *PatchFields) metav1.PatchOptions {
	if inputs == nil {
		return metav1.PatchOptions{}
	}
	return metav1.PatchOptions{
		DryRun:          dryRunFlag(inputs.DryRun),
		FieldManager:    inputs.FieldManager,
		FieldValidation: inputs.FieldValidation,
		Force:           inputs.Force,
	}
}

type UpdateFields struct {
	DryRun          string `json:"dryRun,omitempty"`
	FieldManager    string `json:"fieldManager,omitempty"`
	FieldValidation string `json:"fieldValidation,omitempty"`
}

func MetaUpdate(inputs *UpdateFields) metav1.UpdateOptions {
	if inputs == nil {
		return metav1.UpdateOptions{}
	}
	return metav1.UpdateOptions{
		DryRun:          dryRunFlag(inputs.DryRun),
		FieldManager:    inputs.FieldManager,
		FieldValidation: inputs.FieldValidation,
	}
}

func dryRunFlag(dryRun string) []string {
	if dryRun == "" {
		return nil
	}
	return []string{dryRun}
}
