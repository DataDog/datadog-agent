// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

// YAMLToBool converts a scalar read from a generically parsed YAML map into a
// boolean, with some YAML 1.1 backward compatibility (yes/no/on/off).
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
