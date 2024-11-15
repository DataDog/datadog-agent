// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package structure

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/nodetreemodel"
	"github.com/stretchr/testify/assert"
)

// Struct that is used within the config
type userV3 struct {
	Username       string `yaml:"user"`
	UsernameLegacy string `yaml:"username"`
	AuthKey        string `yaml:"authKey"`
	AuthProtocol   string `yaml:"authProtocol"`
	PrivKey        string `yaml:"privKey"`
	PrivProtocol   string `yaml:"privProtocol"`
}

// Type that gets parsed out of config
type trapsConfig struct {
	Enabled          bool     `yaml:"enabled"`
	Port             uint16   `yaml:"port"`
	Users            []userV3 `yaml:"users"`
	CommunityStrings []string `yaml:"community_strings"`
	BindHost         string   `yaml:"bind_host"`
	StopTimeout      int      `yaml:"stop_timeout"`
	Namespace        string   `yaml:"namespace"`
}

func TestUnmarshalKeyTrapsConfig(t *testing.T) {
	confYaml := `
network_devices:
  snmp_traps:
    enabled: true
    port: 1234
    community_strings: ["a","b","c"]
    users:
    - user:         alice
      authKey:      hunter2
      authProtocol: MD5
      privKey:      pswd
      privProtocol: AE5
    - user:         bob
      authKey:      "123456"
      authProtocol: MD5
      privKey:      secret
      privProtocol: AE5
    bind_host: ok
    stop_timeout: 4
    namespace: abc
`
	mockConfig := mock.NewFromYAML(t, confYaml)

	var trapsCfg = trapsConfig{}
	err := unmarshalKeyReflection(mockConfig, "network_devices.snmp_traps", &trapsCfg)
	assert.NoError(t, err)

	assert.Equal(t, trapsCfg.Enabled, true)
	assert.Equal(t, trapsCfg.Port, uint16(1234))
	assert.Equal(t, trapsCfg.CommunityStrings, []string{"a", "b", "c"})

	assert.Equal(t, len(trapsCfg.Users), 2)
	assert.Equal(t, trapsCfg.Users[0].Username, "alice")
	assert.Equal(t, trapsCfg.Users[0].AuthKey, "hunter2")
	assert.Equal(t, trapsCfg.Users[0].AuthProtocol, "MD5")
	assert.Equal(t, trapsCfg.Users[0].PrivKey, "pswd")
	assert.Equal(t, trapsCfg.Users[0].PrivProtocol, "AE5")
	assert.Equal(t, trapsCfg.Users[1].Username, "bob")
	assert.Equal(t, trapsCfg.Users[1].AuthKey, "123456")
	assert.Equal(t, trapsCfg.Users[1].AuthProtocol, "MD5")
	assert.Equal(t, trapsCfg.Users[1].PrivKey, "secret")
	assert.Equal(t, trapsCfg.Users[1].PrivProtocol, "AE5")

	assert.Equal(t, trapsCfg.BindHost, "ok")
	assert.Equal(t, trapsCfg.StopTimeout, 4)
	assert.Equal(t, trapsCfg.Namespace, "abc")
}

type endpoint struct {
	Name   string `yaml:"name"`
	APIKey string `yaml:"apikey"`
}

func TestUnmarshalKeySliceOfStructures(t *testing.T) {
	testcases := []struct {
		name string
		conf string
		want []endpoint
	}{
		{
			name: "simple wellformed",
			conf: `
endpoints:
- name: intake
  apikey: abc1
- name: config
  apikey: abc2
- name: health
  apikey: abc3
`,
			want: []endpoint{
				{Name: "intake", APIKey: "abc1"},
				{Name: "config", APIKey: "abc2"},
				{Name: "health", APIKey: "abc3"},
			},
		},
		{
			name: "missing a field is zero value",
			conf: `
endpoints:
- name: intake
- name: config
  apikey: abc2
`,
			want: []endpoint{
				{Name: "intake", APIKey: ""},
				{Name: "config", APIKey: "abc2"},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			mockConfig := mock.NewFromYAML(t, tc.conf)
			mockConfig.SetKnown("endpoints")

			var endpoints = []endpoint{}
			err := unmarshalKeyReflection(mockConfig, "endpoints", &endpoints)
			assert.NoError(t, err, "%s failed to marshal: %s", tc.name, err)

			assert.Equal(t, len(endpoints), len(tc.want), "%s marshalled unexepected length of slices, wanted: %s got: %s", tc.name, len(tc.want), len(endpoints))
			for i := range endpoints {
				assert.Equal(t, endpoints[i].Name, tc.want[i].Name)
				assert.Equal(t, endpoints[i].APIKey, tc.want[i].APIKey)
			}
		})
	}
}

type featureConfig struct {
	Enabled bool `yaml:"enabled"`
}

func TestUnmarshalKeyAsBool(t *testing.T) {
	testcases := []struct {
		name string
		conf string
		want bool
		skip bool
	}{
		{
			name: "string value to true",
			conf: `
feature:
  enabled: "true"
`,
			want: true,
			skip: false,
		},
		{
			name: "yaml boolean value true",
			conf: `
feature:
  enabled: true
`,
			want: true,
			skip: false,
		},
		{
			name: "string value to false",
			conf: `
feature:
  enabled: "false"
`,
			want: false,
			skip: false,
		},
		{
			name: "yaml boolean value false",
			conf: `
feature:
  enabled: false
`,
			want: false,
			skip: false,
		},
		{
			name: "missing value is false",
			conf: `
feature:
  not_enabled: "the missing key should be false"
`,
			want: false,
			skip: false,
		},
		{
			name: "string 'y' value is true",
			conf: `
feature:
  enabled: y
`,
			want: true,
			skip: false,
		},
		{
			name: "string 'yes' value is true",
			conf: `
feature:
  enabled: yes
`,
			want: true,
			skip: false,
		},
		{
			name: "string 'on' value is true",
			conf: `
feature:
  enabled: on
`,
			want: true,
			skip: false,
		},
		{
			name: "string '1' value is true",
			conf: `
feature:
  enabled: "1"
`,
			want: true,
			skip: false,
		},
		{
			name: "int 1 value is true",
			conf: `
feature:
  enabled: 1
`,
			want: true,
			skip: false,
		},
		{
			name: "string 'n' value is false",
			conf: `
feature:
  enabled: n
`,
			want: false,
			skip: false,
		},
		{
			name: "string 'no' value is false",
			conf: `
feature:
  enabled: no
`,
			want: false,
			skip: false,
		},
		{
			name: "string 'off' value is false",
			conf: `
feature:
  enabled: off
`,
			want: false,
			skip: false,
		},
		{
			name: "string '0' value is false",
			conf: `
feature:
  enabled: "0"
`,
			want: false,
			skip: false,
		},
		{
			name: "int 0 value is false",
			conf: `
feature:
  enabled: 0
`,
			want: false,
			skip: false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip {
				t.Skip("Skipping test case")
			}

			mockConfig := mock.NewFromYAML(t, tc.conf)
			mockConfig.SetKnown("feature")

			var feature = featureConfig{}
			err := unmarshalKeyReflection(mockConfig, "feature", &feature)
			assert.NoError(t, err, "%s failed to marshal: %s", tc.name, err)

			assert.Equal(t, feature.Enabled, tc.want, "%s unexpected marshal value, want: %s got: %s", tc.name, tc.want, feature.Enabled)
		})
	}
}

type uintConfig struct {
	Fielduint8  uint8  `yaml:"uint8"`
	Fielduint16 uint16 `yaml:"uint16"`
	Fielduint32 uint32 `yaml:"uint32"`
	Fielduint64 uint64 `yaml:"uint64"`
	Fieldint8   int8   `yaml:"int8"`
	Fieldint16  int16  `yaml:"int16"`
	Fieldint32  int32  `yaml:"int32"`
	Fieldint64  int64  `yaml:"int64"`
}

func TestUnmarshalKeyAsInt(t *testing.T) {
	testcases := []struct {
		name string
		conf string
		want uintConfig
		skip bool
	}{
		{
			name: "value int config map",
			conf: `
feature:
  uint8:  123
  uint16: 1234
  uint32: 1234
  uint64: 1234
  int8:  123
  int16: 1234
  int32: 1234
  int64: 1234
`,
			want: uintConfig{
				Fielduint8:  123,
				Fielduint16: 1234,
				Fielduint32: 1234,
				Fielduint64: 1234,
				Fieldint8:   123,
				Fieldint16:  1234,
				Fieldint32:  1234,
				Fieldint64:  1234,
			},
			skip: false,
		},
		{
			name: "float convert to int config map",
			conf: `
feature:
  uint8:  12.0
  uint16: 1234.0
  uint32: 1234
  uint64: 1234
  int8:  12.3
  int16: 12.9
  int32: 12.34
  int64: -12.34
`,
			want: uintConfig{
				Fielduint8:  12,
				Fielduint16: 1234,
				Fielduint32: 1234,
				Fielduint64: 1234,
				Fieldint8:   12,
				Fieldint16:  12, // expected truncation of the decimal, no rounding
				Fieldint32:  12,
				Fieldint64:  -12,
			},
			skip: false,
		},
		{
			name: "missing field is zero value config map",
			conf: `
feature:
  uint16: 1234
  uint32: 1234
  uint64: 1234
  int8:  123
  int16: 1234
  int32: 1234
  int64: 1234
`,
			want: uintConfig{
				Fielduint8:  0,
				Fielduint16: 1234,
				Fielduint32: 1234,
				Fielduint64: 1234,
				Fieldint8:   123,
				Fieldint16:  1234,
				Fieldint32:  1234,
				Fieldint64:  1234,
			},
			skip: false,
		},
		{
			name: "overflow int config map",
			conf: `
feature:
  uint8:  1234
  uint16: 1234
  uint32: 1234
  uint64: 1234
  int8:  123
  int16: 1234
  int32: 1234
  int64: 1234
`,
			want: uintConfig{
				Fielduint8:  math.MaxUint8, // actual 230 - unclear what this behavior should be
				Fielduint16: 1234,
				Fielduint32: 1234,
				Fielduint64: 1234,
				Fieldint8:   123,
				Fieldint16:  1234,
				Fieldint32:  1234,
				Fieldint64:  1234,
			},
			skip: true,
		},
		{
			name: "underflow int config map",
			conf: `
feature:
  uint8:  -123
  uint16: 1234
  uint32: 1234
  uint64: 1234
  int8:  123
  int16: 1234
  int32: 1234
  int64: 1234
`,
			want: uintConfig{
				Fielduint8:  0, // actual 133 - unclear what this behavior should be
				Fielduint16: 1234,
				Fielduint32: 1234,
				Fielduint64: 1234,
				Fieldint8:   123,
				Fieldint16:  1234,
				Fieldint32:  1234,
				Fieldint64:  1234,
			},
			skip: true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip {
				t.Skip("Skipping test case")
			}

			mockConfig := mock.NewFromYAML(t, tc.conf)
			mockConfig.SetKnown("feature")

			var feature = uintConfig{}
			err := unmarshalKeyReflection(mockConfig, "feature", &feature)
			assert.NoError(t, err, "%s failed to marshal: %s", tc.name, err)
			if err != nil {
				t.FailNow()
			}

			confvalues := reflect.ValueOf(feature)
			wantvalues := reflect.ValueOf(tc.want)

			for i := 0; i < confvalues.NumField(); i++ {
				wantType := strings.ReplaceAll(confvalues.Type().Field(i).Name, "Field", "")
				actual := confvalues.Field(i).Type().Name()
				assert.Equal(t, wantType, actual, "%s unexpected marshal type, want: %s got: %s", tc.name, wantType, actual)
				assert.True(t, reflect.DeepEqual(wantvalues.Field(i).Interface(), confvalues.Field(i).Interface()), "%s marshalled values not equal, want: %s, got: %s", tc.name, wantvalues.Field(i), confvalues.Field(i))
			}
		})
	}
}

type floatConfig struct {
	Fieldfloat32 float32 `yaml:"float32"`
	Fieldfloat64 float64 `yaml:"float64"`
}

func TestUnmarshalKeyAsFloat(t *testing.T) {
	testcases := []struct {
		name string
		conf string
		want floatConfig
		skip bool
	}{
		{
			name: "value float config map",
			conf: `
feature:
  float32: 12.34
  float64: 12.34
`,
			want: floatConfig{
				Fieldfloat32: 12.34,
				Fieldfloat64: 12.34,
			},
			skip: false,
		},
		{
			name: "missing field zero value float config map",
			conf: `
feature:
  float64: 12.34
`,
			want: floatConfig{
				Fieldfloat32: 0.0,
				Fieldfloat64: 12.34,
			},
			skip: false,
		},
		{
			name: "converts ints to float config map",
			conf: `
feature:
  float32: 12
  float64: 12
`,
			want: floatConfig{
				Fieldfloat32: 12.0,
				Fieldfloat64: 12.0,
			},
			skip: false,
		},
		{
			name: "converts negatives to float config map",
			conf: `
feature:
  float32: -12
  float64: -12.34
`,
			want: floatConfig{
				Fieldfloat32: -12.0,
				Fieldfloat64: -12.34,
			},
			skip: false,
		},
		{
			name: "starting decimal to float config map",
			conf: `
feature:
  float32: .34
  float64: -.34
`,
			want: floatConfig{
				Fieldfloat32: 0.34,
				Fieldfloat64: -0.34,
			},
			skip: false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip {
				t.Skip("Skipping test case")
			}

			mockConfig := mock.NewFromYAML(t, tc.conf)
			mockConfig.SetKnown("feature")

			var feature = floatConfig{}
			err := unmarshalKeyReflection(mockConfig, "feature", &feature)
			assert.NoError(t, err, "%s failed to marshal: %s", tc.name, err)
			if err != nil {
				t.FailNow()
			}

			confvalues := reflect.ValueOf(feature)
			wantvalues := reflect.ValueOf(tc.want)

			for i := 0; i < confvalues.NumField(); i++ {
				wantType := strings.ReplaceAll(confvalues.Type().Field(i).Name, "Field", "")
				actual := confvalues.Field(i).Type().Name()
				assert.Equal(t, wantType, actual, "%s unexpected marshal type, want: %s got: %s", tc.name, wantType, actual)
				assert.True(t, reflect.DeepEqual(wantvalues.Field(i).Interface(), confvalues.Field(i).Interface()), "%s marshalled values not equal, want: %s, got: %s", tc.name, wantvalues.Field(i), confvalues.Field(i))
			}
		})
	}
}

type stringConfig struct {
	Field string `yaml:"value"`
}

func TestUnmarshalKeyAsString(t *testing.T) {
	testcases := []struct {
		name string
		conf string
		want stringConfig
		skip bool
	}{
		{
			name: "string value config map",
			conf: `
feature:
  value: a string
`,
			want: stringConfig{
				Field: "a string",
			},
			skip: false,
		},
		{
			name: "quoted string config map",
			conf: `
feature:
  value: "12.34"
`,
			want: stringConfig{
				Field: "12.34",
			},
			skip: false,
		},
		{
			name: "missing field is a empty string",
			conf: `
feature:
  float64: 12.34
`,
			want: stringConfig{
				Field: string(""),
			},
			skip: false,
		},
		{
			name: "converts yaml parsed int to match struct",
			conf: `
feature:
  value: 42
`,
			want: stringConfig{
				Field: "42",
			},
			skip: false,
		},
		{
			name: "truncates large yaml floats instead of using exponents",
			conf: `
feature:
  value: 4.2222222222222222222222
`,
			want: stringConfig{
				Field: "4.222222222222222",
			},
			skip: false,
		},
		{
			name: "converts yaml parsed float to match struct",
			conf: `
feature:
  value: 4.2
`,
			want: stringConfig{
				Field: "4.2",
			},
			skip: false,
		},
		{
			name: "commas are part of the string and not a list",
			conf: `
feature:
  value: not, a, list
`,
			want: stringConfig{
				Field: "not, a, list",
			},
			skip: false,
		},
		{
			name: "parses special characters",
			conf: `
feature:
  value: ☺☻☹
`,
			want: stringConfig{
				Field: "☺☻☹",
			},
			skip: false,
		},
		{
			name: "does not parse invalid ascii to byte sequences",
			conf: `
feature:
  value: \xff-\xff
`,
			want: stringConfig{
				Field: `\xff-\xff`,
			},
			skip: false,
		},
		{
			name: "retains string utf-8",
			conf: `
feature:
  value: 日本語
`,
			want: stringConfig{
				Field: "日本語",
			},
			skip: false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip {
				t.Skip("Skipping test case")
			}

			mockConfig := mock.NewFromYAML(t, tc.conf)
			mockConfig.SetKnown("feature")

			var feature = stringConfig{}
			err := unmarshalKeyReflection(mockConfig, "feature", &feature)
			assert.NoError(t, err, "%s failed to marshal: %s", tc.name, err)
			if err != nil {
				t.FailNow()
			}

			confvalues := reflect.ValueOf(feature)
			wantvalues := reflect.ValueOf(tc.want)

			for i := 0; i < confvalues.NumField(); i++ {
				wantType := "string"
				actual := confvalues.Field(i).Type().Name()
				assert.Equal(t, wantType, actual, "%s unexpected marshal type, want: %s got: %s", tc.name, wantType, actual)
				assert.True(t, reflect.DeepEqual(wantvalues.Field(i).Interface(), confvalues.Field(i).Interface()), "%s marshalled values not equal, want: %s, got: %s", tc.name, wantvalues.Field(i), confvalues.Field(i))
			}
		})
	}
}

type featureConfigDiffCase struct {
	ENaBLEd bool
}

func TestUnmarshalKeyCaseInsensitive(t *testing.T) {
	confYaml := `
feature:
  EnABLeD: "true"
`
	mockConfig := mock.NewFromYAML(t, confYaml)
	mockConfig.SetKnown("feature")

	var feature = featureConfig{}
	err := unmarshalKeyReflection(mockConfig, "feature", &feature)
	assert.NoError(t, err)

	assert.Equal(t, feature.Enabled, true)

	var diffcase = featureConfigDiffCase{}
	err = unmarshalKeyReflection(mockConfig, "feature", &diffcase)
	assert.NoError(t, err)

	assert.Equal(t, diffcase.ENaBLEd, true)
}

func TestUnmarshalKeyMissing(t *testing.T) {
	confYaml := `
feature:
  enabled: "true"
`
	mockConfig := mock.NewFromYAML(t, confYaml)
	mockConfig.SetKnown("feature")

	// If the data from the config is missing, UnmarshalKey is a no-op, does
	// nothing, and returns no error
	var endpoints = []endpoint{}
	err := unmarshalKeyReflection(mockConfig, "config_providers", &endpoints)
	assert.NoError(t, err)
}

func TestUnmarshalKeyConversionErrors(t *testing.T) {
	t.Run("errors on string to int", func(t *testing.T) {
		confYaml := `
feature:
  enabled: "true"
`

		mockConfig := mock.NewFromYAML(t, confYaml)
		mockConfig.SetKnown("feature")

		feature := struct {
			Enabled int
		}{}

		err := unmarshalKeyReflection(mockConfig, "feature", &feature)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not convert to int")
	})

	t.Run("errors on string to float", func(t *testing.T) {
		confYaml := `
feature:
  enabled: "true"
`

		mockConfig := mock.NewFromYAML(t, confYaml)
		mockConfig.SetKnown("feature")

		feature := struct {
			Enabled float64
		}{}

		err := unmarshalKeyReflection(mockConfig, "feature", &feature)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not convert to float")
	})

	t.Run("errors on bad string to bool", func(t *testing.T) {
		confYaml := `
feature:
  enabled: elderberries
`

		mockConfig := mock.NewFromYAML(t, confYaml)
		mockConfig.SetKnown("feature")

		feature := struct {
			Enabled bool
		}{}

		err := unmarshalKeyReflection(mockConfig, "feature", &feature)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not convert to bool")
	})

	t.Run("errors on empty string bool ", func(t *testing.T) {
		confYaml := `
feature:
  enabled: ""
`

		mockConfig := mock.NewFromYAML(t, confYaml)
		mockConfig.SetKnown("feature")

		feature := struct {
			Enabled bool
		}{}

		err := unmarshalKeyReflection(mockConfig, "feature", &feature)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not convert to bool")
	})

	t.Run("errors on float to bool", func(t *testing.T) {
		confYaml := `
feature:
  enabled: 1.0
`

		mockConfig := mock.NewFromYAML(t, confYaml)
		mockConfig.SetKnown("feature")

		feature := struct {
			Enabled bool
		}{}

		err := unmarshalKeyReflection(mockConfig, "feature", &feature)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not convert to bool")
	})

	t.Run("errors on negative to uint", func(t *testing.T) {
		t.Skip("not implemented")
		confYaml := `
feature:
  enabled: -1
`

		mockConfig := mock.NewFromYAML(t, confYaml)
		mockConfig.SetKnown("feature")

		feature := struct {
			Enabled uint
		}{}

		err := unmarshalKeyReflection(mockConfig, "feature", &feature)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not convert to uint")
	})

	t.Run("errors on bool to int", func(t *testing.T) {
		confYaml := `
feature:
  enabled: true
`

		mockConfig := mock.NewFromYAML(t, confYaml)
		mockConfig.SetKnown("feature")

		feature := struct {
			Enabled int
		}{}

		err := unmarshalKeyReflection(mockConfig, "feature", &feature)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not convert to int")
	})

	t.Run("errors on bool to float", func(t *testing.T) {
		confYaml := `
feature:
  enabled: true
`

		mockConfig := mock.NewFromYAML(t, confYaml)
		mockConfig.SetKnown("feature")

		feature := struct {
			Enabled float64
		}{}

		err := unmarshalKeyReflection(mockConfig, "feature", &feature)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not convert to float")
	})

	t.Run("errors on bool to string", func(t *testing.T) {
		confYaml := `
feature:
  enabled: true
`

		mockConfig := mock.NewFromYAML(t, confYaml)
		mockConfig.SetKnown("feature")

		feature := struct {
			Enabled string
		}{}

		err := unmarshalKeyReflection(mockConfig, "feature", &feature)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not convert to string")
	})

	t.Run("errors on map to scalar type", func(t *testing.T) {
		confYaml := `
feature:
  enabled:
    key: value
`

		mockConfig := mock.NewFromYAML(t, confYaml)
		mockConfig.SetKnown("feature")

		feature := struct {
			Enabled string
		}{}

		err := unmarshalKeyReflection(mockConfig, "feature", &feature)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "can't copy into target: scalar required")
	})

	t.Run("errors on non-string map", func(t *testing.T) {
		confYaml := `
feature:
  enabled: true
`

		mockConfig := mock.NewFromYAML(t, confYaml)
		mockConfig.SetKnown("feature")

		feature := make(map[string]bool)

		err := unmarshalKeyReflection(mockConfig, "feature", &feature)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "only map[string]string supported currently")
	})
}

// A flag is provided as a struct tag after a field name separated by a comma that
// alters the decoding behavior, eg. struct { Foo string `json:"field1,omitempty"` }
//
// List of common package flags we take into consideration:
// * yaml.v2 flags:  omitempty, flow, inline
// * json flags: omitempty
// * mapstructure flags: squash, remain, omitempty
func TestUnmarshalKeyStructTagFlags(t *testing.T) {
	confYaml := `
feature:
  enabled: "true"
`
	want := "true"

	mockConfig := mock.NewFromYAML(t, confYaml)
	mockConfig.SetKnown("feature")

	t.Run("json omitempty", func(t *testing.T) {
		feature := struct {
			Enabled string `json:"enabled,omitempty"`
		}{}

		err := unmarshalKeyReflection(mockConfig, "feature", &feature)
		assert.NoError(t, err, "%s failed to marshal: %s", "json omitempty flag", err)

		assert.Equal(t, feature.Enabled, want, "unexpected marshal value, want: %s got: %s", want, feature.Enabled)
	})

	t.Run("yaml omitempty", func(t *testing.T) {
		feature := struct {
			Enabled string `yaml:"enabled,omitempty"`
		}{}

		err := unmarshalKeyReflection(mockConfig, "feature", &feature)
		assert.NoError(t, err, "%s failed to marshal: %s", "yaml omitempty flag", err)

		assert.Equal(t, feature.Enabled, want, "unexpected marshal value, want: %s got: %s", want, feature.Enabled)
	})

	t.Run("yaml flow", func(t *testing.T) {
		feature := struct {
			Enabled string `yaml:"enabled,flow"`
		}{}

		err := unmarshalKeyReflection(mockConfig, "feature", &feature)
		assert.NoError(t, err, "%s failed to marshal: %s", "yaml flow flag", err)

		assert.Equal(t, feature.Enabled, want, "unexpected marshal value, want: %s got: %s", want, feature.Enabled)
	})

	t.Run("yaml inline", func(t *testing.T) {
		feature := struct {
			Enabled string `yaml:"enabled,inline"`
		}{}

		err := unmarshalKeyReflection(mockConfig, "feature", &feature)
		assert.NoError(t, err, "%s failed to marshal: %s", "yaml inline flag", err)

		assert.Equal(t, feature.Enabled, want, "unexpected marshal value, want: %s got: %s", want, feature.Enabled)
	})

	t.Run("yaml multiple flags", func(t *testing.T) {
		feature := struct {
			Enabled string `yaml:"enabled,inline,flow"`
		}{}

		err := unmarshalKeyReflection(mockConfig, "feature", &feature)
		assert.NoError(t, err, "%s failed to marshal: %s", "yaml multiple flags", err)

		assert.Equal(t, feature.Enabled, want, "unexpected marshal value, want: %s got: %s", want, feature.Enabled)
	})

	t.Run("mapstructure remain", func(t *testing.T) {
		feature := struct {
			Enabled string `mapstructure:"enabled,remain"`
		}{}

		err := unmarshalKeyReflection(mockConfig, "feature", &feature)
		assert.NoError(t, err, "%s failed to marshal: %s", "mapstructure omitempty flag", err)

		assert.Equal(t, feature.Enabled, want, "unexpected marshal value, want: %s got: %s", want, feature.Enabled)
	})

	t.Run("mapstructure squash errors without option", func(t *testing.T) {
		feature := struct {
			Enabled string `mapstructure:"enabled,squash"`
		}{}

		err := unmarshalKeyReflection(mockConfig, "feature", &feature)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "EnableSquash")
	})

	t.Run("mapstructure omitempty", func(t *testing.T) {
		feature := struct {
			Enabled string `mapstructure:"enabled,omitempty"`
		}{}

		err := unmarshalKeyReflection(mockConfig, "feature", &feature)
		assert.NoError(t, err, "%s failed to marshal: %s", "mapstructure omitempty flag", err)

		assert.Equal(t, feature.Enabled, want, "unexpected marshal value, want: %s got: %s", want, feature.Enabled)
	})

	t.Run("mapstructure multiple flags", func(t *testing.T) {
		feature := struct {
			Enabled string `mapstructure:"enabled,remain,omitempty"`
		}{}

		err := unmarshalKeyReflection(mockConfig, "feature", &feature)
		assert.NoError(t, err, "%s failed to marshal: %s", "mapstructure multiple flags", err)

		assert.Equal(t, feature.Enabled, want, "unexpected marshal value, want: %s got: %s", want, feature.Enabled)
	})

	t.Run("mapstructure squash multiple flags errors without option", func(t *testing.T) {
		t.Skip("FIXME")

		feature := struct {
			Enabled string `mapstructure:"enabled,remain,squash"`
		}{}

		err := unmarshalKeyReflection(mockConfig, "feature", &feature)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "EnableSquash")
	})
}

type expectation struct {
	fieldName    string
	specifierSet map[string]struct{}
	skip         bool
}

func TestFieldNameToKey(t *testing.T) {
	target := struct {
		A string `json:"a,omitempty"`
		B string `yaml:"b,omitempty"`
		C string `yaml:"c,flow"`
		D string `yaml:"d,inline"`
		E string `yaml:"e,inline,omitempty"`
		F string `mapstructure:"f,squash"`
		G string `mapstructure:"g,remain"`
		H string `mapstructure:"h,omitempty"`
		I string `mapstructure:"i,remain,squash"`
		J string `mapstructure:",squash"`
		// tags that aren't yaml, json, or mapstructure are ignored
		K string `apple:"orange" mapstructure:"k,squash"`
	}{}

	expectedSelectorSet := []expectation{
		{
			fieldName:    "a",
			specifierSet: map[string]struct{}{"omitempty": {}},
			skip:         false,
		},
		{
			fieldName:    "b",
			specifierSet: map[string]struct{}{"omitempty": {}},
			skip:         false,
		},
		{
			fieldName:    "c",
			specifierSet: map[string]struct{}{"flow": {}},
			skip:         false,
		},
		{
			fieldName:    "d",
			specifierSet: map[string]struct{}{"inline": {}},
			skip:         false,
		},
		{
			fieldName:    "e",
			specifierSet: map[string]struct{}{"inline": {}, "omitempty": {}},
			skip:         true,
		},
		{
			fieldName:    "f",
			specifierSet: map[string]struct{}{"squash": {}},
			skip:         false,
		},
		{
			fieldName:    "g",
			specifierSet: map[string]struct{}{"remain": {}},
			skip:         false,
		},
		{
			fieldName:    "h",
			specifierSet: map[string]struct{}{"omitempty": {}},
			skip:         false,
		},
		{
			fieldName:    "i",
			specifierSet: map[string]struct{}{"remain": {}, "omitempty": {}},
			skip:         true,
		},
		{
			fieldName:    "j",
			specifierSet: map[string]struct{}{"squash": {}},
			skip:         false,
		},
		{
			fieldName:    "k",
			specifierSet: map[string]struct{}{"squash": {}},
		},
	}
	targetType := reflect.ValueOf(target).Type()
	assert.Equal(t, targetType.NumField(), len(expectedSelectorSet), "test cases and expectations are not equal length")
	for i := 0; i < targetType.NumField(); i++ {
		f := targetType.Field(i)

		t.Run(string(f.Tag), func(t *testing.T) {
			if expectedSelectorSet[i].skip {
				t.Skip("Skipping test case")
			}

			actualName, actualSpecifiers := fieldNameToKey(f)

			assert.Equal(t, actualName, expectedSelectorSet[i].fieldName)
			for k := range expectedSelectorSet[i].specifierSet {
				assert.Contains(t, actualSpecifiers, k)
			}
		})
	}
}

type squashConfig struct {
	Host     string
	Endpoint endpoint `mapstructure:",squash"`
}

func TestUnmarshalKeyWithSquash(t *testing.T) {
	confYaml := `
service:
  host: datad0g.com
  name: intake
  apikey: abc1
`
	mockConfig := mock.NewFromYAML(t, confYaml)
	mockConfig.SetKnown("service")
	var svc = squashConfig{}

	t.Run("squash flag succeeds with option", func(t *testing.T) {
		err := unmarshalKeyReflection(mockConfig, "service", &svc, EnableSquash)
		assert.NoError(t, err)

		assert.Equal(t, svc.Host, "datad0g.com")
		assert.Equal(t, svc.Endpoint.Name, "intake")
		assert.Equal(t, svc.Endpoint.APIKey, "abc1")
	})
}

func TestUnmarshalKeysToMap(t *testing.T) {
	confYaml := `
service:
  host: datad0g.com
  name: intake
  apikey: abc1
  the_great_question: 42
`
	mockConfig := mock.NewFromYAML(t, confYaml)
	mockConfig.SetKnown("service")
	var svc = make(map[string]string)

	err := unmarshalKeyReflection(mockConfig, "service", &svc)
	assert.NoError(t, err)

	assert.Equal(t, svc["host"], "datad0g.com")
	assert.Equal(t, svc["name"], "intake")
	assert.Equal(t, svc["apikey"], "abc1")
	assert.Equal(t, svc["the_great_question"], "42")
}

func TestMapGetChildNotFound(t *testing.T) {
	m := map[string]interface{}{"a": "apple", "b": "banana"}
	n, err := nodetreemodel.NewNodeTree(m, model.SourceDefault)
	assert.NoError(t, err)

	val, err := n.GetChild("a")
	assert.NoError(t, err)
	str, err := val.(nodetreemodel.LeafNode).GetString()
	assert.NoError(t, err)
	assert.Equal(t, str, "apple")

	_, err = n.GetChild("c")
	assert.Error(t, err)
	assert.Equal(t, err.Error(), "not found")

	inner, ok := n.(nodetreemodel.InnerNode)
	assert.True(t, ok)
	assert.Equal(t, inner.ChildrenKeys(), []string{"a", "b"})
}
