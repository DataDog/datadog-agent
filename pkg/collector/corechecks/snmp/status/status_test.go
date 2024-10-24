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

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
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

			assert.Empty(t, stats)
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
	profileExpVar := expvar.NewMap("snmpProfileErrors")
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

func TestStatusAutodiscovery(t *testing.T) {
	autodiscoveryExpVar := expvar.NewMap("snmpAutodiscovery")
	devicesScannedInSubnetVar := expvar.NewMap("devicesScannedInSubnet")
	autodiscoveryExpVar.Set("devicesScannedInSubnet", devicesScannedInSubnetVar)
	devicesScannedInSubnetVar.Set("127.0.0.1/24|hashconfig", expvar.Func(func() interface{} {
		return 0
	}))

	devicesFoundInSubnetVar := expvar.NewMap("devicesFoundInSubnet")
	autodiscoveryExpVar.Set("devicesFoundInSubnet", devicesFoundInSubnetVar)
	devicesFoundInSubnetVar.Set("127.0.0.1/24|hashconfig", expvar.Func(func() interface{} {
		return ""
	}))

	deviceScanningInSubnetVar := expvar.NewMap("deviceScanningInSubnet")
	autodiscoveryExpVar.Set("deviceScanningInSubnet", deviceScanningInSubnetVar)
	deviceScanningInSubnetVar.Set("127.0.0.1/24|hashconfig", expvar.Func(func() interface{} {
		return ""
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
  Autodiscovery
  =============
  Scanning subnet 127.0.0.1/24 with config hashconfig... Currently scanning IP , 0 IPs out of 256 scanned.`

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
    Scanning subnet 127.0.0.1/24 with config hashconfig... Currently scanning IP , 0 IPs out of 256 scanned.</br>
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
