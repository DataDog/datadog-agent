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
  foobar: error1
error2`

			expectedResult := strings.Replace(expectedTextOutput, "\r\n", "\n", -1)
			output := strings.Replace(b.String(), "\r\n", "\n", -1)
			assert.Equal(t, expectedResult, output)

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
	autodiscoveryExpVar.Set("127.0.0.1/24", expvar.Func(func() interface{} {
		return "scanning"
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
  Subnet 127.0.0.1/24 scanning...`

			expectedResult := strings.Replace(expectedTextOutput, "\r\n", "\n", -1)
			output := strings.Replace(b.String(), "\r\n", "\n", -1)
			assert.Equal(t, expectedResult, output)

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
    Subnet 127.0.0.1/24 scanning...</br>
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
