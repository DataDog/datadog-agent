// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateLevelRules(t *testing.T) {
	for spec, valid := range map[string]bool{
		"debug":                     true,
		"error,some/pkg=debug":      true,
		"info,./relative/...=trace": true,
		"error,.=debug":             true,
		"":                          false,
		"verbose":                   false,
		"error,some/pkg=noisy":      false,
		"error,=debug":              false,
		"info,debug":                false,
	} {
		err := ValidateLevelRules(spec)
		if valid {
			assert.NoError(t, err, "expected %q to be valid", spec)
		} else {
			assert.Error(t, err, "expected %q to be invalid", spec)
		}
	}
}
