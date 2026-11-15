// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

// ToLenientBool converts a scalar from a generically parsed YAML map into a
// boolean, accepting real booleans, the true/false strings and the YAML 1.1
// spellings (yes/no/on/off/y/n) that yaml v2 resolved but v3 leaves as
// strings. Only use it for options documented as booleans. ok is false when
// the value is not an accepted spelling (including nil).
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
