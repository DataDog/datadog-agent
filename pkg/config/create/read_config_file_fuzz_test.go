// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package create

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/nodetreemodel"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
)

func FuzzReadConfig(f *testing.F) {
	// Seed corpus from existing test cases - these represent known valid configurations
	f.Add(`network_devices:
  snmp_traps:
    enabled: true
    port: 1234
    bind_host: ok
    stop_timeout: 4
    namespace: abc`)

	f.Add(`network_devices:
  snmp_traps:
    port: 9876
    bind_host: ko`)

	f.Add(`a: orange
c:
  d: 1234`)

	f.Add(`a: orange
c:
  d: 1234
  unknown: key`)

	f.Add(`a: orange
c: 1234`)

	// Edge cases for YAML parsing
	f.Add(``)
	f.Add(`key: value`)
	f.Add(`nested:
  deeply:
    very:
      much: true`)
	f.Add(`list:
  - item1
  - item2
  - item3`)
	f.Add(`mixed:
  string: "text"
  number: 42
  boolean: true
  null_value: null
  list: [1, 2, 3]`)

	// Invalid YAML that should trigger error handling paths
	f.Add(`{invalid yaml`)
	f.Add(`- invalid
  yaml`)
	f.Add(`key: value
  invalid: indentation`)

	// Specific cases to trigger the nil pointer panic in GetBool/GetNodeValue
	// This happens when a key is not found and getNodeValue tries to cast missingLeaf to InnerNode
	f.Add(`dogstatsd_entity_id_precedence: ""`) // Empty string that causes cast.ToBoolE to fail
	f.Add(`some_missing_key: ""`)               // Another empty string case
	f.Add(`nested:
  missing_child: ""`) // Empty string in nested structure

	// More targeted cases to reproduce the exact panic scenario
	f.Add(`dogstatsd_entity_id_precedence: ""
other_key: value`) // Mix of empty and valid values
	f.Add(`
dogstatsd_entity_id_precedence: ""
`) // Empty string with whitespace
	f.Add(`dogstatsd_entity_id_precedence:
other: value`) // Null value (YAML parses as nil)

	f.Fuzz(func(_ *testing.T, yamlContent string) {
		cfg := nodetreemodel.NewNodeTreeConfig("test", "TEST", nil) // nolint: forbidigo // legit use case
		cfg.SetDefault("a", "default_a")
		cfg.SetDefault("c.d", 42)
		cfg.SetDefault("network_devices.snmp_traps.enabled", false)
		cfg.SetDefault("network_devices.snmp_traps.port", 0)
		cfg.BuildSchema()

		// ReadConfig is the main method under test
		err := cfg.ReadConfig(strings.NewReader(yamlContent))

		// Exercise all getter methods regardless of error state
		// This tests error handling and ensures fuzzer explores all code paths
		// Basic getters - test scalar value retrieval
		cfg.Get("a")
		cfg.GetString("a")
		cfg.GetBool("network_devices.snmp_traps.enabled")
		cfg.GetInt("network_devices.snmp_traps.port")
		cfg.GetInt32("mixed.number")
		cfg.GetInt64("mixed.number")
		cfg.GetFloat64("mixed.number")
		cfg.GetDuration("mixed.number")
		cfg.GetSizeInBytes("mixed.number")

		// Collection getters - test complex type handling
		cfg.GetStringSlice("list")
		cfg.GetFloat64Slice("list")
		cfg.GetStringMap("mixed")
		cfg.GetStringMapString("mixed")
		cfg.GetStringMapStringSlice("mixed")

		// Metadata getters - test source tracking and schema introspection
		cfg.GetSource("a")
		cfg.GetAllSources("a")
		cfg.IsSet("a")
		cfg.IsConfigured("a")
		cfg.IsKnown("a")

		// Schema introspection - test tree structure access
		cfg.AllKeysLowercased()
		cfg.AllSettings()
		cfg.AllSettingsWithoutDefault()
		cfg.AllSettingsBySource()
		cfg.GetKnownKeysLowercased()
		cfg.GetSubfields("network_devices")
		cfg.GetSubfields("c")

		// Configuration metadata - test config state inspection
		cfg.ConfigFileUsed()
		cfg.ExtraConfigFilesUsed()
		cfg.GetEnvVars()
		cfg.GetProxies()
		cfg.GetSequenceID()
		cfg.Warnings()

		// Debug methods - test string representation generation
		cfg.Stringify("all")
		cfg.Stringify("default")
		cfg.Stringify("file")

		// Test error conditions don't crash
		if err == nil {
			// Only test additional operations if ReadConfig succeeded
			// Test unmarshal functionality on valid configs
			var result map[string]interface{}
			structure.UnmarshalKey(cfg, "", &result)
			structure.UnmarshalKey(cfg, "c", &result)

			// Test settings retrieval with sequence ID
			_, _ = cfg.AllFlattenedSettingsWithSequenceID()
		}
	})
}
