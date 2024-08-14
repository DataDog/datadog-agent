// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package common provides functions used by several mutating webhooks
package common

import (
	"encoding/json"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
)

// ValidationFunc is a function that validates a pod
type ValidationFunc func(pod *corev1.Pod, ns string, cl dynamic.Interface) (bool, error)

// Validate handles validating pods and encoding and decoding admission
// requests and responses for the public validate functions
func Validate(rawPod []byte, ns string, webhookName string, v ValidationFunc, dc dynamic.Interface) (bool, error) {
	var pod corev1.Pod
	if err := json.Unmarshal(rawPod, &pod); err != nil {
		return false, fmt.Errorf("failed to decode raw object: %v", err)
	}

	validated, err := v(&pod, ns, dc)
	if err != nil {
		metrics.ValidationAttempts.Inc(webhookName, metrics.StatusError, strconv.FormatBool(false), err.Error())
		return false, fmt.Errorf("failed to validate pod: %v", err)
	}

	metrics.ValidationAttempts.Inc(webhookName, metrics.StatusSuccess, strconv.FormatBool(validated), "")
	return validated, nil
}
