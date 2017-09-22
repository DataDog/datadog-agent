package py

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitialize(t *testing.T) {
	require.NotEmpty(t, PythonVersion)
}
