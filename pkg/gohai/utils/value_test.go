// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package utils

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValueDefault(t *testing.T) {
	value := Value[int]{}
	_, err := value.Value()
	require.Error(t, err)
}

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
}

func TestNewValueFrom(t *testing.T) {
	myerr := fmt.Errorf("yet another error")
	value := NewValueFrom(42, myerr)
	_, err := value.Value()
	require.ErrorIs(t, err, myerr)

	value = NewValueFrom(42, nil)
	val, err := value.Value()
	require.NoError(t, err)
	require.Equal(t, 42, val)
}
