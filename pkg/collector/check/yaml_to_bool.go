// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

// YAMLToBool converts a scalar read from a generically parsed YAML map into a
// boolean, accepting real booleans, the lowercase true/false strings and the
// lowercase YAML 1.1 spellings yes/no/on/off that yaml v2 resolved to
// booleans but yaml v3 leaves as strings. Only use it for options documented
// as booleans. ok is false when the value is not an accepted spelling
// (including nil).
func YAMLToBool(value interface{}) (b bool, ok bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		switch v {
		case "true", "yes", "on":
			return true, true
		case "false", "no", "off":
			return false, true
		}
	}
	return false, false
}
