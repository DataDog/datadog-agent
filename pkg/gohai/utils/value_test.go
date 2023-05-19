// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package utils

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewValue(t *testing.T) {
	value := NewValue(42)
	v, err := value.Value()
	require.NoError(t, err)
	require.Equal(t, 42, v)
}

func TestNewErrorValue(t *testing.T) {
	myErr := errors.New("this is an error")
	value := NewErrorValue[int](myErr)
	result, err := value.Value()
	require.ErrorIs(t, err, myErr)
	require.Equal(t, 0, result)

	myOtherErr := errors.New("this is another error")
	value = value.NewErrorValue(myOtherErr)
	result, err = value.Value()
	require.ErrorIs(t, err, myOtherErr)
	require.Equal(t, 0, result)
}
