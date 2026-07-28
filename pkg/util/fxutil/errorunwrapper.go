// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxutil

import (
	"errors"
	"reflect"
)

// UnwrapIfErrArgumentsFailed unwrap the error if the error was returned by an FX invoke method otherwise return the error.
func UnwrapIfErrArgumentsFailed(err error) error {
	// This is a workaround until https://github.com/uber-go/fx/issues/988 will be done.
	if reflect.TypeOf(err).Name() == "errArgumentsFailed" {
		for {
			cause := errors.Unwrap(err)
			if cause == nil {
				return err
			}
			err = cause
		}
	}
	return err
}
