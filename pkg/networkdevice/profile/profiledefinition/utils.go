// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profiledefinition

// KeyValue used to represent mapping
// Used for RC compatibility (map to list)
type KeyValue struct {
	Key   string `yaml:"key" json:"key"`
	Value string `yaml:"value" json:"value"`
}

// KeyValueList is a list of mapping key values
type KeyValueList []KeyValue

// ToMap convert KeyValueList to map[string]string
func (kvl *KeyValueList) ToMap() map[string]string {
	mapping := make(map[string]string)
	for _, item := range *kvl {
		mapping[item.Key] = item.Value
	}
	return mapping
}
