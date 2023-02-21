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
