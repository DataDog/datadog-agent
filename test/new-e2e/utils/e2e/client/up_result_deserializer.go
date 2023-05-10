// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

type clientService[T any] interface {
	stackInitializer
	clientServiceInitializer[T]
}

type clientServiceInitializer[T any] interface {
	initService(*T) error
}

// UpResultDeserializer is an helper to build a new type that can be used in an environment.
// It is designed to be used as an embeded field.
// See VM type in this package for an example of usage.
type UpResultDeserializer[T any] struct {
	initializer  clientServiceInitializer[T]
	deserializer utils.RemoteServiceDeserializer[T]
}

// NewUpResultDeserializer creates a new instance of UpResultDeserializer.
// deserializer is a function that deserializes the output of a stack
// init is a function that initializes the parent struct.
func NewUpResultDeserializer[T any](
	deserializer utils.RemoteServiceDeserializer[T],
	initializer clientServiceInitializer[T]) *UpResultDeserializer[T] {
	return &UpResultDeserializer[T]{
		initializer:  initializer,
		deserializer: deserializer,
	}
}

func (d *UpResultDeserializer[T]) setStack(stackResult auto.UpResult) error {
	value, err := d.deserializer.Deserialize(stackResult)
	if err != nil {
		return err
	}
	return d.initializer.initService(value)
}
