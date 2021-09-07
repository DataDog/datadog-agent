package discovery

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/session"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestDiscovery(t *testing.T) {
	sess := session.CreateMockSession()
	session.NewSession = func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}

	packet := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: "1.3.6.1.4.1.3375.2.1.3.4.1",
			},
		},
	}
	sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).Return(&packet, nil)

	checkConfig := &checkconfig.CheckConfig{
		Network:            "192.168.0.0/29",
		CommunityString:    "public",
		DiscoveryInterval:  3600,
		DiscoveryWorkers:   1,
		IgnoredIPAddresses: map[string]bool{"192.168.0.5": true},
	}
	discovery := NewDiscovery(checkConfig)
	discovery.Start()
	time.Sleep(100 * time.Millisecond)
	discovery.Stop()

	deviceConfigs := discovery.GetDiscoveredDeviceConfigs()

	var actualDiscoveredIps []string
	for _, deviceCk := range deviceConfigs {
		actualDiscoveredIps = append(actualDiscoveredIps, deviceCk.GetIPAddress())
	}
	expectedDiscoveredIps := []string{
		"192.168.0.0",
		"192.168.0.1",
		"192.168.0.2",
		"192.168.0.3",
		"192.168.0.4",
		// 192.168.0.5 is ignored
		"192.168.0.6",
		"192.168.0.7",
	}
	assert.ElementsMatch(t, expectedDiscoveredIps, actualDiscoveredIps)
}
