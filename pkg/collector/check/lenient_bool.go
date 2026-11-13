// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

// ToLenientBool converts a scalar read from a generically parsed YAML map
// into a boolean. It accepts real booleans and the YAML 1.1 boolean
// spellings (yes/no/on/off/y/n, in the case variants the go.yaml.in/yaml/v2
// parser accepted).
//
// It exists so checks reading their options from generic maps keep behaving
// like the yaml v2 parser when migrated to yaml v4, where only true/false
// resolve to booleans: an unquoted `use_mount: yes` in a verbatim YAML
// config (e.g. an autodiscovery template) still enables the option.
// Configuration files from conf.d are not affected either way, as their
// values are normalized when the files are read.
//
// String values "true" and "false" are deliberately NOT accepted: under
// yaml v2 a quoted boolean (or a JSON string, e.g. in a container or
// Kubernetes annotation) was left as a string and ignored by the generic
// map reads, and fleet data shows many customers with such values on disk
// check options — honoring them would silently change their metrics.
// Apply the helper only to options that are documented as booleans:
// elsewhere the legacy spellings are legitimate strings and must not be
// converted.
//
// ok is false when value is not one of the accepted spellings, including
// when it is nil (missing key).
func ToLenientBool(value interface{}) (b bool, ok bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		switch v {
		case "yes", "Yes", "YES",
			"on", "On", "ON",
			"y", "Y":
			return true, true
		case "no", "No", "NO",
			"off", "Off", "OFF",
			"n", "N":
			return false, true
		}
	}
	return false, false
}
