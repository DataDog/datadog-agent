package deviceconfig

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeviceConfig(t *testing.T) {
	check := DeviceConfigCheck{}
	err := check.Run()
	require.NoError(t, err)
}
