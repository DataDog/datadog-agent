// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package common provides functions used by several mutating webhooks
package common

import (
	"fmt"
	"strconv"

	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
)

// ValidationFunc is a function that validates an admission request.
type ValidationFunc func(request *admission.Request, ns string, cl dynamic.Interface) (bool, error)

// Validate handles validating, encoding and decoding admission requests and responses for the public validate functions.
func Validate(request *admission.Request, webhookName string, v ValidationFunc, dc dynamic.Interface) (bool, error) {
	validated, err := v(request, request.Namespace, dc)
	if err != nil {
		metrics.ValidationAttempts.Inc(webhookName, metrics.StatusError, strconv.FormatBool(false), err.Error())
		return false, fmt.Errorf("failed to validate admission request: %w", err)
	}

	metrics.ValidationAttempts.Inc(webhookName, metrics.StatusSuccess, strconv.FormatBool(validated), "")
	return validated, nil
}
