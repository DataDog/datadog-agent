package traps

import (
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var serverPort = getFreePort()

func TestListenV1GenericTrap(t *testing.T) {
	config := Config{Port: serverPort, CommunityStrings: []string{"public"}}
	Configure(t, config)

	packetOutChan := make(PacketsChannel)
	trapListener, err := startSNMPTrapListener(config, packetOutChan)
	require.NoError(t, err)
	defer trapListener.Stop()

	sendTestV1GenericTrap(t, config, "public")
	packet := receivePacket(t, trapListener)
	require.NotNil(t, packet)
	packet.Content.SnmpTrap.Variables = packet.Content.Variables
	assert.Equal(t, LinkDownv1GenericTrap, packet.Content.SnmpTrap)
}

func TestServerV1SpecificTrap(t *testing.T) {
	config := Config{Port: serverPort, CommunityStrings: []string{"public"}}
	Configure(t, config)

	packetOutChan := make(PacketsChannel)
	trapListener, err := startSNMPTrapListener(config, packetOutChan)
	require.NoError(t, err)
	defer trapListener.Stop()

	sendTestV1SpecificTrap(t, config, "public")
	packet := receivePacket(t, trapListener)
	require.NotNil(t, packet)
	packet.Content.SnmpTrap.Variables = packet.Content.Variables
	assert.Equal(t, AlarmActiveStatev1SpecificTrap, packet.Content.SnmpTrap)
}

func TestServerV2(t *testing.T) {
	config := Config{Port: serverPort, CommunityStrings: []string{"public"}}
	Configure(t, config)

	packetOutChan := make(PacketsChannel)
	trapListener, err := startSNMPTrapListener(config, packetOutChan)
	require.NoError(t, err)
	defer trapListener.Stop()

	sendTestV2Trap(t, config, "public")
	packet := receivePacket(t, trapListener)
	require.NotNil(t, packet)
	assertIsValidV2Packet(t, packet, config)
	assertVariables(t, packet)
}

func TestServerV2BadCredentials(t *testing.T) {
	config := Config{Port: serverPort, CommunityStrings: []string{"public"}}
	Configure(t, config)

	packetOutChan := make(PacketsChannel)
	trapListener, err := startSNMPTrapListener(config, packetOutChan)
	require.NoError(t, err)
	defer trapListener.Stop()

	sendTestV2Trap(t, config, "wrong-community")
	assertNoPacketReceived(t, trapListener)
}

func TestServerV3(t *testing.T) {
	userV3 := UserV3{Username: "user", AuthKey: "password", AuthProtocol: "sha", PrivKey: "password", PrivProtocol: "aes"}
	config := Config{Port: serverPort, Users: []UserV3{userV3}}
	Configure(t, config)

	packetOutChan := make(PacketsChannel)
	trapListener, err := startSNMPTrapListener(config, packetOutChan)
	require.NoError(t, err)
	defer trapListener.Stop()

	sendTestV3Trap(t, config, &gosnmp.UsmSecurityParameters{
		UserName:                 "user",
		AuthoritativeEngineID:    "foobarbaz",
		AuthenticationPassphrase: "password",
		AuthenticationProtocol:   gosnmp.SHA,
		PrivacyPassphrase:        "password",
		PrivacyProtocol:          gosnmp.AES,
	})
	packet := receivePacket(t, trapListener)
	require.NotNil(t, packet)
	assertVariables(t, packet)
}

func TestServerV3BadCredentials(t *testing.T) {
	userV3 := UserV3{Username: "user", AuthKey: "password", AuthProtocol: "sha", PrivKey: "password", PrivProtocol: "aes"}
	config := Config{Port: serverPort, Users: []UserV3{userV3}}
	Configure(t, config)

	packetOutChan := make(PacketsChannel)
	trapListener, err := startSNMPTrapListener(config, packetOutChan)
	require.NoError(t, err)
	defer trapListener.Stop()

	sendTestV3Trap(t, config, &gosnmp.UsmSecurityParameters{
		UserName:                 "user",
		AuthoritativeEngineID:    "foobarbaz",
		AuthenticationPassphrase: "password",
		AuthenticationProtocol:   gosnmp.SHA,
		PrivacyPassphrase:        "wrong_password",
		PrivacyProtocol:          gosnmp.AES,
	})
	assertNoPacketReceived(t, trapListener)
}

// receivePacket waits for a received trap packet and returns it.
func receivePacket(t *testing.T, listener *TrapListener) *SnmpPacket {
	select {
	case packet := <-listener.packets:
		return packet
	case <-time.After(3 * time.Second):
		t.Error("Trap not received")
		return nil
	}
}

func assertNoPacketReceived(t *testing.T, listener *TrapListener) {
	select {
	case <-listener.packets:
		t.Error("Unexpectedly received an unauthorized packet")
	case <-time.After(100 * time.Millisecond):
		break
	}
}
