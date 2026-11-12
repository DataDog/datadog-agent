// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

// ToLenientBool converts a scalar read from a generically parsed YAML map
// into a boolean. It accepts real booleans, the true/false strings, and the
// YAML 1.1 boolean spellings (yes/no/on/off/y/n, in the case variants the
// go.yaml.in/yaml/v2 parser accepted).
//
// It exists so checks migrated from go.yaml.in/yaml/v2 to go.yaml.in/yaml/v4
// keep accepting configuration files written with YAML 1.1 booleans, where
// yaml v4 only treats true/false as booleans. Apply it only to options that
// are documented as booleans: elsewhere the legacy spellings are legitimate
// strings and must not be converted.
//
// ok is false when value is not one of the accepted spellings, including
// when it is nil (missing key).
func ToLenientBool(value interface{}) (b bool, ok bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		switch v {
		case "true", "True", "TRUE",
			"yes", "Yes", "YES",
			"on", "On", "ON",
			"y", "Y":
			return true, true
		case "false", "False", "FALSE",
			"no", "No", "NO",
			"off", "Off", "OFF",
			"n", "N":
			return false, true
		}
	}
	return false, false
}
