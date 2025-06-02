// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"strconv"
	"time"

	admiv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// ClusterAgentStartTime records the Cluster Agent start time
	ClusterAgentStartTime = strconv.FormatInt(time.Now().Unix(), 10)
)

// ValidationResponse returns the result of the validation
func ValidationResponse(validation bool, err error) *admiv1.AdmissionResponse {
	if err != nil {
		log.Warnf("Failed to validate: %v", err)

		return &admiv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
			Allowed: false,
		}
	}

	return &admiv1.AdmissionResponse{
		Allowed: validation,
	}
}

// MutationResponse returns the result of the mutation
func MutationResponse(jsonPatch []byte, err error) *admiv1.AdmissionResponse {
	if err != nil {
		log.Warnf("Failed to mutate: %v", err)

		return &admiv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
			Allowed: true,
		}
	}

	patchType := admiv1.PatchTypeJSONPatch

	return &admiv1.AdmissionResponse{
		Patch:     jsonPatch,
		PatchType: &patchType,
		Allowed:   true,
	}
}
