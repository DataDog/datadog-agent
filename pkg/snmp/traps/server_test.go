package traps

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getAvailableUDPPort requests a random port number and makes sure it is available
func getAvailableUDPPort() (int, error) {
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return -1, fmt.Errorf("can't find an available udp port: %s", err)
	}
	defer conn.Close()

	_, portString, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return -1, fmt.Errorf("can't find an available udp port: %s", err)
	}
	portInt, err := strconv.Atoi(portString)
	if err != nil {
		return -1, fmt.Errorf("can't convert udp port: %s", err)
	}

	return portInt, nil
}

func configure(t *testing.T, yaml string) {
	config.Datadog.SetConfigType("yaml")
	err := config.Datadog.ReadConfig(strings.NewReader(yaml))
	require.NoError(t, err)
}

func TestNewServer(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		s, err := NewTrapServer()
		assert.NotNil(t, s)
		defer s.Stop()
		require.NoError(t, err)
		assert.True(t, s.Started)
	})

	t.Run("v2c-single", func(t *testing.T) {
		port, err := getAvailableUDPPort()
		require.NoError(t, err)

		configure(t, fmt.Sprintf(`
snmp_traps_listeners:
  - port: %d
    community: public
`, port))

		s, err := NewTrapServer()
		assert.NotNil(t, s)
		defer s.Stop()
		require.NoError(t, err)
		assert.True(t, s.Started)
		assert.Equal(t, s.NumListeners(), 1)
	})

	t.Run("v2c-multiple", func(t *testing.T) {
		port0, err := getAvailableUDPPort()
		require.NoError(t, err)
		port1, err := getAvailableUDPPort()
		require.NoError(t, err)
		port2, err := getAvailableUDPPort()
		require.NoError(t, err)

		configure(t, fmt.Sprintf(`
snmp_traps_listeners:
  - port: %d
    community: public0
  - port: %d
    community: public1
  - port: %d
    community: public2
`, port0, port1, port2))

		s, err := NewTrapServer()
		assert.NotNil(t, s)
		defer s.Stop()
		require.NoError(t, err)
		assert.True(t, s.Started)
		assert.Equal(t, s.NumListeners(), 3)
	})

	t.Run("handle-listener-error", func(t *testing.T) {
		port, err := getAvailableUDPPort()
		require.NoError(t, err)

		// Use the same port to trigger an "address already in use" error for one of the listeners.
		configure(t, fmt.Sprintf(`
snmp_traps_listeners:
  - port: %d
    community: public0
  - port: %d
    community: public1
`, port, port))

		s, err := NewTrapServer()
		assert.Nil(t, s)
		require.Error(t, err)
	})
}
