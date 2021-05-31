// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"errors"
	"testing"

	"github.com/hashicorp/go-multierror"
	"gotest.tools/assert"
)

func TestBasic(t *testing.T) {
	var multi *multierror.Error

	multi = multierror.Append(multi, errors.New("A"))
	multi = multierror.Append(multi, errors.New("B"))

	filter := []error{errors.New("B"), errors.New("C")}

	res := FilterMultiError(multi, filter)

	assert.Assert(t, res != nil, "should return a non-nil error")
	assert.Equal(t, len(res.Errors), 1, "should return the correct count of errors")
}

func TestEmptyFilter(t *testing.T) {
	var multi *multierror.Error

	multi = multierror.Append(multi, errors.New("A"))
	multi = multierror.Append(multi, errors.New("B"))

	filter := []error{}

	res := FilterMultiError(multi, filter)

	assert.Assert(t, res != nil, "should return a non-nil error")
	assert.Equal(t, len(res.Errors), 2, "should return the correct count of errors")
}

func TestNilMulti(t *testing.T) {
	var multi *multierror.Error

	filter := []error{errors.New("B"), errors.New("C")}

	res := FilterMultiError(multi, filter)

	assert.Assert(t, res == nil, "should return a nil multierror")
}
