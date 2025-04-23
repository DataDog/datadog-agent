// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package testutils

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// ResourceFromMap builds a resource with resource attributes set to the provided map.
func NewResourceFromMap(t *testing.T, m map[string]any) pcommon.Resource {
	res := pcommon.NewResource()
	err := res.Attributes().FromRaw(m)
	require.NoError(t, err)
	return res
}
