// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type ValidEnv struct {
	VM *VM
}

func TestValidEnv(t *testing.T) {
	require.NoError(t, CheckEnvStructValid[ValidEnv]())
}

type UnexportedFieldEnv struct {
	vm *VM
}

func TestUnexportedFieldEnv(t *testing.T) {
	require.Error(t, CheckEnvStructValid[UnexportedFieldEnv]())
}

type DoesNotImplementInterfaceEnv struct {
	VM string
}

func TestDoesNotImplementInterfaceEnv(t *testing.T) {
	require.Error(t, CheckEnvStructValid[DoesNotImplementInterfaceEnv]())
}
