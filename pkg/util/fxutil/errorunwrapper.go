// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxutil

import (
	"errors"
	"reflect"
	"regexp"
)

// UnwrapIfErrArgumentsFailed unwrap the error if the error was returned by an FX invoke method otherwise return the error.
func UnwrapIfErrArgumentsFailed(err error) error {
	// This is a workaround until https://github.com/uber-go/fx/issues/988 will be done.
	if reflect.TypeOf(err).Name() == "errArgumentsFailed" {
		re := regexp.MustCompile(`.*received non-nil error from function.*\(.*\): (.*)`)
		matches := re.FindStringSubmatch(err.Error())
		if len(matches) == 2 {
			return errors.New(matches[1])
		}
	}
	return err
}
