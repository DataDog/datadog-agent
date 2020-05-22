package traps

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/soniah/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getAvailableUDPPort requests a random port number and makes sure it is available
func getAvailableUDPPort() (uint16, error) {
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return 0, fmt.Errorf("can't find an available udp port: %s", err)
	}
	defer conn.Close()

	_, portString, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return 0, fmt.Errorf("can't find an available udp port: %s", err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		return 0, fmt.Errorf("can't convert udp port: %s", err)
	}

	return uint16(port), nil
}

func configure(t *testing.T, yaml string) {
	config.Datadog.SetConfigType("yaml")
	err := config.Datadog.ReadConfig(strings.NewReader(yaml))
	require.NoError(t, err)
}

func sendTrapV2c(port uint16, community string, variables []gosnmp.SnmpPDU) error {
	params := &gosnmp.GoSNMP{
		Target:    "127.0.0.1",
		Port:      port,
		Version:   gosnmp.Version2c,
		Community: community,
		Retries:   3,
		Timeout:   time.Duration(5) * time.Second,
	}

	err := params.Connect()
	if err != nil {
		return err
	}

	defer params.Conn.Close()

	_, err = params.SendTrap(gosnmp.SnmpTrap{Variables: variables})

	return err
}

func TestNewServer(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		s, err := NewTrapServer()
		require.NoError(t, err)
		assert.NotNil(t, s)
		defer s.Stop()
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

		// Start the server
		s, err := NewTrapServer()
		require.NoError(t, err)
		assert.NotNil(t, s)
		defer s.Stop()
		assert.True(t, s.Started)
		assert.Equal(t, s.NumListeners(), 1)

		// Prepare to receive test traps
		packets := make(chan gosnmp.SnmpPacket)
		s.SetTrapHandler(func(p *gosnmp.SnmpPacket, u *net.UDPAddr) {
			packets <- *p
		})

		// Send an netSnmpExampleHeartbeatNotification trap
		// http://www.circitor.fr/Mibs/Html/N/NET-SNMP-EXAMPLES-MIB.php#netSnmpExampleHeartbeatNotification
		err = sendTrapV2c(port, "public", []gosnmp.SnmpPDU{
			// snmpTrapOID (points to netSnmpExampleHeartbeatNotification)
			{Name: "1.3.6.1.6.3.1.1.4.1", Type: gosnmp.OctetString, Value: "1.3.6.1.4.1.8072.2.3.0.1"},
			// heartBeatRate
			{Name: "1.3.6.1.4.1.8072.2.3.2.1", Type: gosnmp.Integer, Value: 1024},
			// heartBeatName
			{Name: "1.3.6.1.4.1.8072.2.3.2.2", Type: gosnmp.OctetString, Value: "test"},
		})
		require.NoError(t, err)

		var p gosnmp.SnmpPacket

		select {
		case p = <-packets:
			close(packets)
			break
		case <-time.After(3 * time.Second):
			t.Errorf("Trap not received")
			return
		}

		assert.Equal(t, gosnmp.Version2c, p.Version)
		assert.Equal(t, "public", p.Community)
		assert.Equal(t, 4, len(p.Variables))

		uptime := p.Variables[0]
		assert.Equal(t, ".1.3.6.1.2.1.1.3.0", uptime.Name)
		assert.Equal(t, gosnmp.TimeTicks, uptime.Type)

		snmptrapOID := p.Variables[1]
		assert.Equal(t, ".1.3.6.1.6.3.1.1.4.1", snmptrapOID.Name)
		assert.Equal(t, gosnmp.OctetString, snmptrapOID.Type)
		snmptrapOIDValue, ok := snmptrapOID.Value.([]byte)
		assert.True(t, ok)
		assert.Equal(t, "1.3.6.1.4.1.8072.2.3.0.1", string(snmptrapOIDValue))

		heartBeatRate := p.Variables[2]
		assert.Equal(t, ".1.3.6.1.4.1.8072.2.3.2.1", heartBeatRate.Name)
		assert.Equal(t, gosnmp.Integer, heartBeatRate.Type)
		heartBeatRateValue, ok := heartBeatRate.Value.(int)
		assert.True(t, ok)
		assert.Equal(t, 1024, heartBeatRateValue)

		heartBeatName := p.Variables[3]
		assert.Equal(t, ".1.3.6.1.4.1.8072.2.3.2.2", heartBeatName.Name)
		assert.Equal(t, gosnmp.OctetString, heartBeatName.Type)
		heartBeatNameValue, ok := heartBeatName.Value.([]byte)
		assert.True(t, ok)
		assert.Equal(t, "test", string(heartBeatNameValue))
	})

	t.Run("v3-single", func(t *testing.T) {
		port, err := getAvailableUDPPort()
		require.NoError(t, err)

		configure(t, fmt.Sprintf(`
snmp_traps_listeners:
  - port: %d
    user: doggo
    auth_protocol: MD5
    auth_key: doggopass
    priv_protocol: DES
    priv_key: doggokey
`, port))

		s, err := NewTrapServer()
		assert.NotNil(t, s)
		defer s.Stop()
		require.NoError(t, err)

		// TODO send and receive a test trap.
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
		require.NoError(t, err)
		assert.NotNil(t, s)
		defer s.Stop()
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
		require.Error(t, err)
		assert.Nil(t, s)
	})
}
