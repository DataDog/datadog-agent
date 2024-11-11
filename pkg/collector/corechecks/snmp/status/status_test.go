// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package status

import (
	"bytes"
	"expvar"
	"fmt"
	"strings"
	"testing"

	// Importing the discovery package to register the expvar
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/discovery"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/snmp"
	"github.com/stretchr/testify/assert"
)

func TestStatus(t *testing.T) {
	provider := Provider{}
	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			provider.JSON(false, stats)

			assert.Equal(t, map[string]interface{}{
				"autodiscoverySubnets": []subnetStatus{},
				"discoverySubnets":     []subnetStatus{},
				"snmpProfiles":         map[string]string{},
			}, stats)

			fmt.Printf("%v", stats)
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := provider.Text(false, b)

			assert.NoError(t, err)

			assert.Empty(t, b.String())
			fmt.Printf("%s", b.String())
		}},
		{"HTML", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := provider.HTML(false, b)

			assert.NoError(t, err)

			assert.Empty(t, b.String())
			fmt.Printf("%s", b.String())

		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}

func TestStatusWithProfileError(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("snmp_profile_errors", "error")
	profileExpVar := expvar.Get("snmpProfileErrors").(*expvar.Map)
	errors := []string{"error1", "error2"}
	profileExpVar.Set("foobar", expvar.Func(func() interface{} {
		return strings.Join(errors, "\n")
	}))

	provider := Provider{}
	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			provider.JSON(false, stats)

			assert.NotEmpty(t, stats)
			fmt.Printf("%v", stats)
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := provider.Text(false, b)

			assert.NoError(t, err)

			expectedTextOutput := `
  Profiles
  ========
  foobar: error1
error2`

			expectedResult := strings.Replace(expectedTextOutput, "\r\n", "\n", -1)
			output := strings.Replace(b.String(), "\r\n", "\n", -1)
			assert.Contains(t, output, expectedResult)

			fmt.Printf("%s", b.String())
		}},
		{"HTML", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := provider.HTML(false, b)

			assert.NoError(t, err)
			expectedTextOutput := `<div class="stat">
  <span class="stat_title">SNMP Profiles</span>
  <span class="stat_data">
      foobar: error1
error2
  </span>
</div>`

			expectedResult := strings.Replace(expectedTextOutput, "\r\n", "\n", -1)
			output := strings.Replace(b.String(), "\r\n", "\n", -1)
			assert.Contains(t, output, expectedResult)

			fmt.Printf("%s", b.String())

		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}

func TestStatusAutodiscoveryMultipleSubnets(t *testing.T) {
	mockSnmpConfig1 := snmp.Config{
		Network:   "127.0.0.1/24",
		Community: "public",
	}
	mockSnmpConfig2 := snmp.Config{
		Network:   "127.0.10.1/30",
		Community: "public",
	}
	mockSnmpConfig3 := snmp.Config{
		Network:   "127.0.10.1/30",
		Community: "cisco",
	}

	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("network_devices.autodiscovery", snmp.ListenerConfig{
		Configs: []snmp.Config{mockSnmpConfig1, mockSnmpConfig2, mockSnmpConfig3},
		Workers: 1,
	})

	listenerConfig, _ := snmp.NewListenerConfig()

	snmpConfig1 := listenerConfig.Configs[0]
	snmpConfig2 := listenerConfig.Configs[1]
	snmpConfig3 := listenerConfig.Configs[2]

	autodiscoveryExpVar := expvar.Get("snmpAutodiscovery").(*expvar.Map)

	autodiscoveryStatus1 := listeners.AutodiscoveryStatus{DevicesFoundList: []string{}, CurrentDevice: "", DevicesScannedCount: 0}
	autodiscoveryExpVar.Set(listeners.GetSubnetVarKey(snmpConfig1.Network, snmpConfig1.Digest(snmpConfig1.Network)), &autodiscoveryStatus1)

	autodiscoveryStatus2 := listeners.AutodiscoveryStatus{DevicesFoundList: []string{"127.0.10.1", "127.0.10.2"}, CurrentDevice: "127.0.10.2", DevicesScannedCount: 3}
	autodiscoveryExpVar.Set(listeners.GetSubnetVarKey(snmpConfig2.Network, snmpConfig2.Digest(snmpConfig2.Network)), &autodiscoveryStatus2)

	autodiscoveryStatus3 := listeners.AutodiscoveryStatus{DevicesFoundList: []string{}, CurrentDevice: "127.0.10.3", DevicesScannedCount: 4}
	autodiscoveryExpVar.Set(listeners.GetSubnetVarKey(snmpConfig3.Network, snmpConfig3.Digest(snmpConfig3.Network)), &autodiscoveryStatus3)

	provider := Provider{}
	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			provider.JSON(false, stats)

			assert.NotEmpty(t, stats)
			fmt.Printf("%v", stats)
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := provider.Text(false, b)

			assert.NoError(t, err)

			expectedTextOutput := `
  Autodiscovery
  =============
  Subnet 127.0.0.1/24 is queued for scanning.
  No IPs found in the subnet.

  Scanning subnet 127.0.10.1/30... Currently scanning IP 127.0.10.2, 3 IPs out of 4 scanned.
  Found the following IP(s) in the subnet:
    - 127.0.10.1
    - 127.0.10.2

  Subnet 127.0.10.1/30 scanned.
  No IPs found in the subnet.
`

			expectedResult := strings.Replace(expectedTextOutput, "\r\n", "\n", -1)
			output := strings.Replace(b.String(), "\r\n", "\n", -1)
			assert.Contains(t, output, expectedResult)

			fmt.Printf("%s", b.String())
		}},
		{"HTML", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := provider.HTML(false, b)

			assert.NoError(t, err)
			expectedTextOutput := `
<div class="stat">
  <span class="stat_title">SNMP Autodiscovery</span>
  <span class="stat_data">
    Subnet 127.0.0.1/24 is queued for scanning.</br>
    Found no IPs in the subnet.</br>

    Scanning subnet 127.0.10.1/30... Currently scanning IP 127.0.10.2, 3 IPs out of 4 scanned.</br>
    Found the following IP(s) :</br>
      - 127.0.10.1</br>
      - 127.0.10.2</br>

    Subnet 127.0.10.1/30 scanned.</br>
    Found no IPs in the subnet.</br>
</span>
</div>`

			expectedResult := strings.Replace(expectedTextOutput, "\r\n", "\n", -1)
			output := strings.Replace(b.String(), "\r\n", "\n", -1)
			assert.Contains(t, output, expectedResult)

			fmt.Printf("%s", b.String())

		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}

func TestStatusLegacyDiscoveryMultipleSubnets(t *testing.T) {
	snmpConfig1 := checkconfig.CheckConfig{
		Network:         "127.0.0.1/24",
		CommunityString: "public",
	}
	snmpConfig2 := checkconfig.CheckConfig{
		Network:         "127.0.10.1/30",
		CommunityString: "public",
	}
	snmpConfig3 := checkconfig.CheckConfig{
		Network:         "127.0.10.1/30",
		CommunityString: "cisco",
	}

	autodiscoveryExpVar := expvar.Get("snmpDiscovery").(*expvar.Map)

	autodiscoveryStatus1 := listeners.AutodiscoveryStatus{DevicesFoundList: []string{}, CurrentDevice: "", DevicesScannedCount: 0}
	autodiscoveryExpVar.Set(listeners.GetSubnetVarKey(snmpConfig1.Network, string(snmpConfig1.DeviceDigest(snmpConfig1.Network))), &autodiscoveryStatus1)

	autodiscoveryStatus2 := listeners.AutodiscoveryStatus{DevicesFoundList: []string{"127.0.10.1", "127.0.10.2"}, CurrentDevice: "127.0.10.2", DevicesScannedCount: 3}
	autodiscoveryExpVar.Set(listeners.GetSubnetVarKey(snmpConfig2.Network, string(snmpConfig2.DeviceDigest(snmpConfig2.Network))), &autodiscoveryStatus2)

	autodiscoveryStatus3 := listeners.AutodiscoveryStatus{DevicesFoundList: []string{}, CurrentDevice: "127.0.10.3", DevicesScannedCount: 4}
	autodiscoveryExpVar.Set(listeners.GetSubnetVarKey(snmpConfig3.Network, string(snmpConfig3.DeviceDigest(snmpConfig3.Network))), &autodiscoveryStatus3)

	provider := Provider{}
	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			provider.JSON(false, stats)

			assert.NotEmpty(t, stats)
			fmt.Printf("%v", stats)
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := provider.Text(false, b)

			assert.NoError(t, err)

			expectedTextOutputs := []string{
				`Subnet 127.0.0.1/24 is queued for scanning.
  No IPs found in the subnet.`,
				`Scanning subnet 127.0.10.1/30... Currently scanning IP 127.0.10.2, 3 IPs out of 4 scanned.
  Found the following IP(s) in the subnet:
    - 127.0.10.1
    - 127.0.10.2`,
				`Subnet 127.0.10.1/30 scanned.
  No IPs found in the subnet.`,
			}

			output := strings.Replace(b.String(), "\r\n", "\n", -1)
			for _, expectedTextOutput := range expectedTextOutputs {
				expectedResult := strings.Replace(expectedTextOutput, "\r\n", "\n", -1)
				assert.Contains(t, output, expectedResult)
			}

			fmt.Printf("%s", b.String())
		}},
		{"HTML", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := provider.HTML(false, b)

			assert.NoError(t, err)
			expectedTextOutputs := []string{
				`Subnet 127.0.0.1/24 is queued for scanning.</br>
    Found no IPs in the subnet.</br>`,
				`Scanning subnet 127.0.10.1/30... Currently scanning IP 127.0.10.2, 3 IPs out of 4 scanned.</br>
    Found the following IP(s) :</br>
      - 127.0.10.1</br>
      - 127.0.10.2</br>`,
				`Subnet 127.0.10.1/30 scanned.</br>
    Found no IPs in the subnet.</br>`,
			}
			output := strings.Replace(b.String(), "\r\n", "\n", -1)
			for _, expectedTextOutput := range expectedTextOutputs {
				expectedResult := strings.Replace(expectedTextOutput, "\r\n", "\n", -1)
				assert.Contains(t, output, expectedResult)
			}
			fmt.Printf("%s", b.String())

		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}
