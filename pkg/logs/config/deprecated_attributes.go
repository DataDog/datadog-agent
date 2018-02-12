// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

// DeprecatedAttribute represents a deprecated attribute that is used in a configuration file
type DeprecatedAttribute struct {
	Name        string
	Replacement string
}

// list of all deprecated attributes
var deprecatedAttributes = []DeprecatedAttribute{
	{
		Name:        "log_enabled",
		Replacement: "logs_enabled",
	},
}

// GetDeprecatedAttributesInUse returns the list of all deprecated attributes used in the agent configuration file
func GetDeprecatedAttributesInUse() []DeprecatedAttribute {
	deprecatedAttributesInUse := []DeprecatedAttribute{}
	for _, deprecatedAttribute := range deprecatedAttributes {
		if LogsAgent.IsSet(deprecatedAttribute.Name) {
			deprecatedAttributesInUse = append(deprecatedAttributesInUse, deprecatedAttribute)
		}
	}
	return deprecatedAttributesInUse
}
