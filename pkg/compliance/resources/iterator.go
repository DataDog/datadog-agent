// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
)

// ErrInvalidIteration is returned when an invalid (out of bounds) iteration is performed
var ErrInvalidIteration = errors.New("out of bounds iteration")

type instanceIterator struct {
	instances []eval.Instance
	index     int
}

func (it *instanceIterator) Next() (eval.Instance, error) {
	if it.Done() {
		return nil, ErrInvalidIteration
	}
	instance := it.instances[it.index]
	it.index++
	return instance, nil
}

func (it *instanceIterator) Done() bool {
	return it.index >= len(it.instances)
}

func newInstanceIterator(instances []eval.Instance) *instanceIterator {
	return &instanceIterator{instances: instances}
}
