// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"errors"
	"os"
	"testing"
)

func TestNewMissingConfigFileError(t *testing.T) {
	err := NewMissingConfigFileError(os.ErrNotExist)

	for _, target := range []error{ErrMissingConfigFile, ErrConfigFileNotFound, os.ErrNotExist} {
		if !errors.Is(err, target) {
			t.Errorf("error %q does not wrap %q", err, target)
		}
	}
}
