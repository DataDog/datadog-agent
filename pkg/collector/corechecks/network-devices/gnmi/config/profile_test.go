// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func mkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestMetricConfigSubscriptionKeys(t *testing.T) {
	interfaceMetric := MetricConfig{
		Tags: map[string]string{"interface": "name"},
	}
	assert.Equal(t, map[string]string{"interface": "name"}, interfaceMetric.SubscriptionKeys())

	componentMetric := MetricConfig{
		Keys: map[string]string{"component": "name"},
		Tags: map[string]string{"cpu": "name"},
	}
	assert.Equal(t, map[string]string{"component": "name"}, componentMetric.SubscriptionKeys())
}

func TestLoadProfile(t *testing.T) {
	mockConfig := configmock.New(t)
	profilesRoot := t.TempDir()
	mockConfig.SetInTest("confd_path", profilesRoot)

	profileDir := filepath.Join(profilesRoot, "gnmi.d", profilesFolder)
	require.NoError(t, writeTestProfile(profileDir, "cisco.yaml", validProfileYAML))
	require.NoError(t, writeTestProfile(profileDir, "enum.yaml", `
metrics:
  - path: /openconfig/interfaces/interface/state/admin-status
    metric: snmp.ifAdminStatus
    type: gauge
    value_map:
      UP: 1
      DOWN: 2
`))

	tests := []struct {
		name        string
		profileRef  string
		wantName    string
		wantMetrics int
		errContains string
	}{
		{
			name:        "load by profile name",
			profileRef:  "cisco",
			wantName:    "cisco",
			wantMetrics: 1,
		},
		{
			name:        "load by profile name with yaml extension",
			profileRef:  "cisco.yaml",
			wantName:    "cisco",
			wantMetrics: 1,
		},
		{
			name:        "load profile with value_map",
			profileRef:  "enum",
			wantName:    "enum",
			wantMetrics: 1,
		},
		{
			name:        "missing profile",
			profileRef:  "missing",
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, err := LoadProfile(tt.profileRef)
			if tt.errContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantName, profile.Name)
			assert.Len(t, profile.Metrics, tt.wantMetrics)
		})
	}
}

func TestLoadProfileAbsolutePath(t *testing.T) {
	profileDir := t.TempDir()
	profilePath := filepath.Join(profileDir, "custom.yaml")
	require.NoError(t, writeFile(profilePath, validProfileYAML))

	profile, err := LoadProfile(profilePath)
	require.NoError(t, err)
	assert.Equal(t, "custom", profile.Name)
	assert.Equal(t, profilePath, profile.Path)
}

func TestValidateProfileDefinition(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		errContains string
	}{
		{
			name: "valid profile",
			raw:  validProfileYAML,
		},
		{
			name: "empty metrics",
			raw: `
metrics: []
`,
			errContains: "at least one metric",
		},
		{
			name: "empty path",
			raw: `
metrics:
  - path: " "
    metric: snmp.ifHCInOctets
    type: monotonic_count
`,
			errContains: "`path` must not be empty",
		},
		{
			name: "empty metric name",
			raw: `
metrics:
  - path: /openconfig/interfaces/interface/state/counters/in-octets
    metric: " "
    type: monotonic_count
`,
			errContains: "`metric` must not be empty",
		},
		{
			name: "unsupported metric type",
			raw: `
metrics:
  - path: /openconfig/interfaces/interface/state/counters/in-octets
    metric: snmp.ifHCInOctets
    type: rate
`,
			errContains: "unsupported metric `type`",
		},
		{
			name: "duplicate explicit key name",
			raw: `
metrics:
  - path: /openconfig/network-instances/network-instance/protocols/protocol/state/session-count
    metric: snmp.sessionCount
    type: gauge
    keys:
      network-instance: name
      protocol: name
`,
			errContains: `gNMI key "name" is used by both path segments "network-instance" and "protocol"`,
		},
		{
			name: "duplicate tag-derived key name",
			raw: `
metrics:
  - path: /openconfig/network-instances/network-instance/protocols/protocol/state/session-count
    metric: snmp.sessionCount
    type: gauge
    tags:
      network-instance: name
      protocol: name
`,
			errContains: `gNMI key "name" is used by both path segments "network-instance" and "protocol"`,
		},
		{
			name: "distinct key names",
			raw: `
metrics:
  - path: /openconfig/network-instances/network-instance/protocols/protocol/state/session-count
    metric: snmp.sessionCount
    type: gauge
    keys:
      network-instance: name
      protocol: identifier
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			profilesRoot := t.TempDir()
			mockConfig.SetInTest("confd_path", profilesRoot)

			profileDir := filepath.Join(profilesRoot, "gnmi.d", profilesFolder)
			require.NoError(t, writeTestProfile(profileDir, "test.yaml", tt.raw))

			_, err := LoadProfile("test")
			if tt.errContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestResolveProfilePath(t *testing.T) {
	mockConfig := configmock.New(t)
	profilesRoot := t.TempDir()
	mockConfig.SetInTest("confd_path", profilesRoot)

	profileDir := filepath.Join(profilesRoot, "gnmi.d", profilesFolder)
	require.NoError(t, writeTestProfile(profileDir, "cisco.yaml", validProfileYAML))

	got, err := resolveProfilePath("cisco")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(profileDir, "cisco.yaml"), got)
}

func TestLoadProfileWithMetadataExtend(t *testing.T) {
	mockConfig := configmock.New(t)
	profilesRoot := t.TempDir()
	mockConfig.SetInTest("confd_path", profilesRoot)

	profileDir := filepath.Join(profilesRoot, "gnmi.d", profilesFolder)
	require.NoError(t, writeTestProfile(profileDir, "_openconfig-metadata.yaml", `
metadata:
  device:
    hostname: /openconfig/system/state/hostname
  interface:
    keys:
      interface: name
    ifindex: /openconfig/interfaces/interface/state/ifindex
  ip_address:
    keys:
      interface: name
    ip: /openconfig/interfaces/interface/subinterfaces/subinterface/ipv4/addresses/address/state/ip
`))
	require.NoError(t, writeTestProfile(profileDir, "with-metadata.yaml", `
extends:
  - _openconfig-metadata
metrics:
  - path: /openconfig/interfaces/interface/state/counters/in-octets
    metric: snmp.ifHCInOctets
    type: monotonic_count
`))

	profile, err := LoadProfile("with-metadata")
	require.NoError(t, err)
	assert.Equal(t, "/openconfig/system/state/hostname", profile.Metadata.Device.Hostname)
	assert.Equal(t, "/openconfig/interfaces/interface/state/ifindex", profile.Metadata.Interface.IfIndex)
	assert.Equal(t, "/openconfig/interfaces/interface/subinterfaces/subinterface/ipv4/addresses/address/state/ip", profile.Metadata.IPAddress.IP)
}

func TestLoadProfileWithDiamondExtends(t *testing.T) {
	mockConfig := configmock.New(t)
	profilesRoot := t.TempDir()
	mockConfig.SetInTest("confd_path", profilesRoot)

	profileDir := filepath.Join(profilesRoot, "gnmi.d", profilesFolder)
	require.NoError(t, writeTestProfile(profileDir, "base.yaml", `
metadata:
  device:
    hostname: /openconfig/system/state/hostname
  interface:
    keys:
      interface: name
    ifindex: /openconfig/interfaces/interface/state/ifindex
`))
	require.NoError(t, writeTestProfile(profileDir, "left.yaml", "extends:\n  - base\n"))
	require.NoError(t, writeTestProfile(profileDir, "right.yaml", "extends:\n  - base\n"))
	require.NoError(t, writeTestProfile(profileDir, "diamond.yaml", `
extends:
  - left
  - right
metrics:
  - path: /openconfig/interfaces/interface/state/counters/in-octets
    metric: snmp.ifHCInOctets
    type: monotonic_count
`))

	profile, err := LoadProfile("diamond")
	require.NoError(t, err)
	assert.Equal(t, "/openconfig/system/state/hostname", profile.Metadata.Device.Hostname)
}

func TestLoadProfileWithTopologyExtend(t *testing.T) {
	mockConfig := configmock.New(t)
	profilesRoot := t.TempDir()
	mockConfig.SetInTest("confd_path", profilesRoot)

	profileDir := filepath.Join(profilesRoot, "gnmi.d", profilesFolder)
	require.NoError(t, writeTestProfile(profileDir, "_openconfig-lldp.yaml", `
topology:
  lldp:
    keys:
      interface: name
      neighbor: id
    chassis_id: /openconfig/lldp/interfaces/interface/neighbors/neighbor/state/chassis-id
    chassis_id_type: /openconfig/lldp/interfaces/interface/neighbors/neighbor/state/chassis-id-type
    port_id: /openconfig/lldp/interfaces/interface/neighbors/neighbor/state/port-id
    port_id_type: /openconfig/lldp/interfaces/interface/neighbors/neighbor/state/port-id-type
    system_name: /openconfig/lldp/interfaces/interface/neighbors/neighbor/state/system-name
    system_description: /openconfig/lldp/interfaces/interface/neighbors/neighbor/state/system-description
    port_description: /openconfig/lldp/interfaces/interface/neighbors/neighbor/state/port-description
    management_address: /openconfig/lldp/interfaces/interface/neighbors/neighbor/state/management-address
`))
	require.NoError(t, writeTestProfile(profileDir, "with-topology.yaml", `
extends:
  - _openconfig-lldp
metrics:
  - path: /openconfig/interfaces/interface/state/counters/in-octets
    metric: snmp.ifHCInOctets
    type: monotonic_count
`))

	profile, err := LoadProfile("with-topology")
	require.NoError(t, err)
	assert.Equal(t, "/openconfig/lldp/interfaces/interface/neighbors/neighbor/state/chassis-id", profile.Topology.LLDP.ChassisID)
	require.NotEmpty(t, profile.Topology.SubscriptionPaths())
}

// Metrics defined in extended profiles are inherited; the extending profile
// overrides a metric with the same path and name, and a profile reached
// through several extends contributes its metrics once.
func TestLoadProfileExtendsMetrics(t *testing.T) {
	mockConfig := configmock.New(t)
	profilesRoot := t.TempDir()
	mockConfig.SetInTest("confd_path", profilesRoot)

	profileDir := filepath.Join(profilesRoot, "gnmi.d", profilesFolder)
	require.NoError(t, writeTestProfile(profileDir, "_base.yaml", `
metrics:
  - path: /openconfig/system/state/uptime
    metric: snmp.sysUpTime
    type: gauge
  - path: /openconfig/interfaces/interface/state/counters/in-octets
    metric: snmp.ifHCInOctets
    type: gauge
    tags:
      interface: name
`))
	require.NoError(t, writeTestProfile(profileDir, "_left.yaml", "extends:\n  - _base\n"))
	require.NoError(t, writeTestProfile(profileDir, "_right.yaml", "extends:\n  - _base\n"))
	require.NoError(t, writeTestProfile(profileDir, "device.yaml", `
extends:
  - _left
  - _right
metrics:
  - path: /openconfig/interfaces/interface/state/counters/in-octets
    metric: snmp.ifHCInOctets
    type: monotonic_count
    tags:
      interface: name
  - path: /openconfig/interfaces/interface/state/counters/out-octets
    metric: snmp.ifHCOutOctets
    type: monotonic_count
    tags:
      interface: name
`))

	profile, err := LoadProfile("device")
	require.NoError(t, err)

	names := make([]string, 0, len(profile.Metrics))
	for _, metric := range profile.Metrics {
		names = append(names, metric.Metric)
	}
	assert.Equal(t, []string{"snmp.sysUpTime", "snmp.ifHCInOctets", "snmp.ifHCOutOctets"}, names)
	assert.Equal(t, MetricTypeMonotonicCount, profile.Metrics[1].Type, "the extending profile overrides the inherited definition")
}

// TestMergeMetadataConfigPreservesAllFields guards against a merge* helper silently
// dropping a sub-block or field: fillNonZero sets every leaf field to a unique value
// via reflection, so a newly added field is covered automatically without updating
// this test by hand.
func TestMergeMetadataConfigPreservesAllFields(t *testing.T) {
	override := fillNonZero(t, MetadataConfig{}).(MetadataConfig)
	merged := mergeMetadataConfig(MetadataConfig{}, override)
	assert.Equal(t, override, merged, "mergeMetadataConfig dropped a field: add it to the merge function")
}

func TestMergeTopologyConfigPreservesAllFields(t *testing.T) {
	override := fillNonZero(t, TopologyConfig{}).(TopologyConfig)
	merged := mergeTopologyConfig(TopologyConfig{}, override)
	assert.Equal(t, override, merged, "mergeTopologyConfig dropped a field: add it to the merge function")
}

// fillNonZero returns a copy of v with every string field set to a unique
// non-empty value and every map[string]string field set to a single-entry map,
// recursing into nested structs.
func fillNonZero(t *testing.T, v any) any {
	t.Helper()
	counter := 0
	out := reflect.New(reflect.TypeOf(v)).Elem()
	fillNonZeroValue(out, &counter)
	return out.Interface()
}

func fillNonZeroValue(v reflect.Value, counter *int) {
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			fillNonZeroValue(v.Field(i), counter)
		}
	case reflect.String:
		*counter++
		v.SetString(fmt.Sprintf("value-%d", *counter))
	case reflect.Map:
		*counter++
		key := fmt.Sprintf("key-%d", *counter)
		value := fmt.Sprintf("value-%d", *counter)
		v.Set(reflect.MakeMap(v.Type()))
		v.SetMapIndex(reflect.ValueOf(key), reflect.ValueOf(value))
	}
}

// Both the check and CLI resolve profiles through these configuration-only
// entry points, before constructing a client or attempting a connection.
func TestLoadProfileRejectsMalformedSelectors(t *testing.T) {
	selectors := []string{
		"/openconfig/interfaces/interface[name=eth0/state/name",
		"/openconfig/system//state/hostname",
		"/openconfig/interfaces/interface[name=eth0][name=eth1]/state/name",
		"bad origin:/system/state/hostname",
		"/openconfig",
		" ",
	}
	for _, field := range profileSelectorFields() {
		for _, selector := range selectors {
			for _, inherited := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/inherited=%t", field, selector, inherited), func(t *testing.T) {
					mockConfig := configmock.New(t)
					mockConfig.SetInTest("confd_path", t.TempDir())
					profileDir := getProfilesRoot()
					raw := validSelectorProfile()
					setProfileSelector(&raw, field, selector)
					if inherited {
						writeRawTestProfile(t, profileDir, "base.yaml", raw)
						raw = rawProfileDefinition{Extends: []string{"base"}}
					}
					writeRawTestProfile(t, profileDir, "test.yaml", raw)

					profile, err := LoadProfile("test")
					require.Error(t, err)
					assert.Nil(t, profile)
					assert.Contains(t, err.Error(), `validate profile "test"`)
					assert.Contains(t, err.Error(), field)
					if selector != " " {
						assert.Contains(t, err.Error(), fmt.Sprintf("%q", selector))
					}

					cfg, err := NewCheckConfig(integration.Data("address: example.invalid\nusername: user\npassword: secret\nprofile: test\n"))
					require.Error(t, err)
					assert.Nil(t, cfg)
					assert.Contains(t, err.Error(), field)
				})
			}
		}
	}
}

func TestLoadProfileRejectsAmbiguousSelectorBindings(t *testing.T) {
	fields := []string{
		"metrics[0].path",
		"metadata.device.component_type",
		"metadata.interface.ifindex",
		"metadata.ip_address.ip",
		"topology.lldp.chassis_id",
	}
	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			raw := validSelectorProfile()
			var selector string
			switch field {
			case "metrics[0].path":
				selector = "/interfaces/interface/interface/state/counters/in-octets"
			case "metadata.device.component_type":
				selector = "/components/component/component/state/type"
			case "metadata.interface.ifindex":
				selector = "/interfaces/interface/interface/state/ifindex"
			case "metadata.ip_address.ip":
				selector = "/interfaces/interface/subinterfaces/subinterface/addresses/address/address/state/ip"
			case "topology.lldp.chassis_id":
				selector = "/lldp/interfaces/interface/neighbors/neighbor/neighbor/state/chassis-id"
			}
			setProfileSelector(&raw, field, selector)
			profileDir := t.TempDir()
			writeRawTestProfile(t, profileDir, "test.yaml", raw)
			_, err := LoadProfile(filepath.Join(profileDir, "test.yaml"))
			require.Error(t, err)
			assert.Contains(t, err.Error(), field)
			assert.Contains(t, err.Error(), "ambiguous key binding")
		})
	}
}

func TestLoadProfileBareMetadataSelectorBindings(t *testing.T) {
	for _, tc := range []struct {
		field    string
		selector string
		segment  string
	}{
		{"metadata.interface.ifindex", "interfaces/interface/state/ifindex", "interface"},
		{"metadata.device.component_type", "components/component/state/type", "component"},
	} {
		for _, ambiguous := range []bool{false, true} {
			for _, inherited := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/ambiguous=%t/inherited=%t", tc.field, ambiguous, inherited), func(t *testing.T) {
					mockConfig := configmock.New(t)
					mockConfig.SetInTest("confd_path", t.TempDir())
					selector := tc.selector
					if ambiguous {
						selector = strings.Replace(selector, "/"+tc.segment+"/", "/"+tc.segment+"/"+tc.segment+"/", 1)
					}
					raw := validSelectorProfile()
					setProfileSelector(&raw, tc.field, selector)
					if inherited {
						writeRawTestProfile(t, getProfilesRoot(), "base.yaml", raw)
						raw = rawProfileDefinition{Extends: []string{"base"}}
					}
					writeRawTestProfile(t, getProfilesRoot(), "test.yaml", raw)

					profile, err := LoadProfile("test")
					cfg, configErr := NewCheckConfig(integration.Data("address: example.invalid\nusername: user\npassword: secret\nprofile: test\n"))
					if ambiguous {
						require.Error(t, err)
						assert.Nil(t, profile)
						require.Error(t, configErr)
						assert.Nil(t, cfg)
						for _, loadErr := range []error{err, configErr} {
							assert.Contains(t, loadErr.Error(), tc.field)
							assert.Contains(t, loadErr.Error(), fmt.Sprintf("%q", selector))
							assert.Contains(t, loadErr.Error(), "ambiguous key binding")
						}
						return
					}
					require.NoError(t, err)
					require.NoError(t, configErr)
					assert.Equal(t, selector, getProfileSelector(&rawProfileDefinition{Metadata: profile.Metadata}, tc.field))
					var subscription *PathSubscriptionConfig
					for _, path := range profile.Metadata.SubscriptionPaths() {
						if path.Path == NormalizePath(selector) {
							subscription = &path
							break
						}
					}
					require.NotNil(t, subscription)
					assert.Equal(t, map[string]string{tc.segment: "name"}, subscription.Tags)
					_, err = CompilePath(subscription.Path, subscription.Tags)
					require.NoError(t, err)
				})
			}
		}
	}
}

func TestLoadProfileValidSelectorFormsAndOverrides(t *testing.T) {
	for _, prefix := range []string{"", "/openconfig/", "openconfig:/", "native:/", "native:/vendor-module:"} {
		t.Run(prefix, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetInTest("confd_path", t.TempDir())
			profileDir := getProfilesRoot()
			base := validSelectorProfile()
			for _, field := range profileSelectorFields() {
				setProfileSelector(&base, field, prefix+strings.TrimPrefix(getProfileSelector(&base, field), "/openconfig/"))
			}
			writeRawTestProfile(t, profileDir, "base.yaml", base)
			override := rawProfileDefinition{
				Extends: []string{"base"},
				Metrics: append([]MetricConfig(nil), base.Metrics...),
				Metadata: MetadataConfig{Device: DeviceMetadataConfig{
					Hostname: prefix + `system/state/hostname[name="router/a"]`,
				}},
			}
			override.Metrics[0].Type = MetricTypeMonotonicCount
			writeRawTestProfile(t, profileDir, "test.yaml", override)

			profile, err := LoadProfile("test")
			require.NoError(t, err)
			require.Len(t, profile.Metrics, 1)
			assert.Equal(t, override.Metrics[0].Path, profile.Metrics[0].Path)
			assert.Equal(t, override.Metrics[0].Metric, profile.Metrics[0].Metric)
			assert.Equal(t, override.Metrics[0].Type, profile.Metrics[0].Type)
			assert.Equal(t, override.Metrics[0].SubscriptionKeys(), profile.Metrics[0].SubscriptionKeys())
			assert.Equal(t, override.Metadata.Device.Hostname, profile.Metadata.Device.Hostname)
			assert.Equal(t, base.Metadata.Interface, profile.Metadata.Interface)
			assert.Equal(t, base.Topology, profile.Topology)
		})
	}
}

func validSelectorProfile() rawProfileDefinition {
	return rawProfileDefinition{
		Metrics: []MetricConfig{{
			Path:   "/openconfig/interfaces/interface/state/counters/in-octets",
			Metric: "snmp.ifHCInOctets", Type: MetricTypeGauge,
			Tags: map[string]string{"interface": "name"},
		}},
		Metadata: DefaultOpenConfigMetadata(),
		Topology: DefaultOpenConfigLLDP(),
	}
}

func writeRawTestProfile(t *testing.T, dir, name string, raw rawProfileDefinition) {
	t.Helper()
	buf, err := yaml.Marshal(raw)
	require.NoError(t, err)
	require.NoError(t, writeTestProfile(dir, name, string(buf)))
}

// Enumerate selector fields by YAML names so every metadata and topology field
// is tested, including fields added in the future.
func profileSelectorFields() []string {
	fields := []string{"metrics[0].path"}
	var walk func(reflect.Type, string)
	walk = func(typ reflect.Type, prefix string) {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name := prefix + "." + field.Tag.Get("yaml")
			switch field.Type.Kind() {
			case reflect.Struct:
				walk(field.Type, name)
			case reflect.String:
				fields = append(fields, name)
			}
		}
	}
	walk(reflect.TypeOf(MetadataConfig{}), "metadata")
	walk(reflect.TypeOf(TopologyConfig{}), "topology")
	return fields
}

func profileSelectorValue(raw *rawProfileDefinition, field string) reflect.Value {
	if field == "metrics[0].path" {
		return reflect.ValueOf(&raw.Metrics[0].Path).Elem()
	}
	value := reflect.ValueOf(raw).Elem()
	for _, name := range strings.Split(field, ".") {
		typ := value.Type()
		for i := 0; i < typ.NumField(); i++ {
			if typ.Field(i).Tag.Get("yaml") == name {
				value = value.Field(i)
				break
			}
		}
	}
	return value
}

func setProfileSelector(raw *rawProfileDefinition, field, selector string) {
	profileSelectorValue(raw, field).SetString(selector)
}

func getProfileSelector(raw *rawProfileDefinition, field string) string {
	return profileSelectorValue(raw, field).String()
}
