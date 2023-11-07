// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"fmt"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"reflect"
	"testing"
)

// connectionInitializer defines a method which is used to initialize an connection between
// testing infrastructure and clients
type connectionInitializer interface {
	// InitFromConnection initializes the instance from ssh connection data.
	// This method is called by [CallConnectionInitializers] using reflection.
	initFromConnection(t *testing.T, conns map[string]*utils.Connection) error
}

// CallConnectionInitializers validates an environment struct and initializes a connection to the testing infrastructure.
func CallConnectionInitializers[Env any](t *testing.T, env *Env, conns map[string]*utils.Connection) error {
	fields, err := getFields(env)

	for _, field := range fields {
		initializer := field.connInitializer
		if reflect.TypeOf(initializer).Kind() == reflect.Ptr && reflect.ValueOf(initializer).IsNil() {
			return fmt.Errorf("the field %v of %v is nil", field.name, reflect.TypeOf(env))
		}

		if err = initializer.initFromConnection(t, conns); err != nil {
			return err
		}
	}

	return err
}
