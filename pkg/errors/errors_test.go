// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package errors

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNotFound(t *testing.T) {
	// New
	err := NewNotFound("foo")
	require.Error(t, err)
	require.Equal(t, `"foo" not found`, err.Error())

	// Is
	require.True(t, IsNotFound(err))
	require.False(t, IsNotFound(fmt.Errorf("fake")))
	require.False(t, IsNotFound(fmt.Errorf(`"foo" not found`)))
}
