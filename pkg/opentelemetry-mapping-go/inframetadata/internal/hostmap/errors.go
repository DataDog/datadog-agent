// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package hostmap

import (
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

var _ error = mismatchedTypeErr{}

type mismatchedTypeErr struct {
	name         string
	actualType   pcommon.ValueType
	expectedType pcommon.ValueType
}

func mismatchErr(name string, actualType, expectedType pcommon.ValueType) error {
	return mismatchedTypeErr{
		name:         name,
		actualType:   actualType,
		expectedType: expectedType,
	}
}

func (e mismatchedTypeErr) Error() string {
	return fmt.Sprintf("%q has type %q, expected type %q instead", e.name, e.actualType, e.expectedType)
}
