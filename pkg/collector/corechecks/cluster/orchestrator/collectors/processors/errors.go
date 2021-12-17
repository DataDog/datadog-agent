// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package processors

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/pkg/errors"
)

// newMarshallingError creates an error that wraps the cause of a marshalling
// error.
func newMarshallingError(cause error) error {
	return errors.WithMessage(cause, "unable to marshal resource to JSON")
}

// recoverOnPanic is used to recover panics triggered by processors.
func recoverOnPanic() {
	if r := recover(); r != nil {
		log.Errorf("unable to process resources (panic!): %s", r)
	}
}
