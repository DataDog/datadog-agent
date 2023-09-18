// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profiledefinition

import (
	"fmt"
)

// StringArray is list of string with a yaml un-marshaller that support both array and string.
// See test file for example usage.
// Credit: https://github.com/go-yaml/yaml/issues/100#issuecomment-324964723
type StringArray []string

// MappingArray is list of key-value mapping with a yaml un-marshaller that support both map[string]string and []KeyValue.
type MappingArray []KeyValue

// UnmarshalYAML unmarshalls StringArray
func (a *StringArray) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var multi []string
	err := unmarshal(&multi)
	if err != nil {
		var single string
		err := unmarshal(&single)
		if err != nil {
			return err
		}
		*a = []string{single}
	} else {
		*a = multi
	}
	return nil
}

// UnmarshalYAML unmarshalls StringArray
func (a *MappingArray) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var keyValueList []KeyValue
	err := unmarshal(&keyValueList)
	if err != nil {
		var mapping map[string]string
		err := unmarshal(&mapping)
		if err != nil {
			return err
		}
		var keyValueListFromMapping []KeyValue
		for key, value := range mapping {
			keyValueListFromMapping = append(keyValueListFromMapping, KeyValue{Key: key, Value: value})
		}
		*a = keyValueListFromMapping
	} else {
		// Check if key used multiple times
		alreadyProcessedKeys := make(map[string]bool)
		for _, entry := range keyValueList {
			if alreadyProcessedKeys[entry.Key] {
				return fmt.Errorf("same key used multiple times: %s", entry.Key)
			}
			alreadyProcessedKeys[entry.Key] = true
		}
		*a = keyValueList
	}
	return nil
}

// UnmarshalYAML unmarshalls MetricTagConfigList
func (mtcl *MetricTagConfigList) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var multi []MetricTagConfig
	err := unmarshal(&multi)
	if err != nil {
		var tags []string
		err := unmarshal(&tags)
		if err != nil {
			return err
		}
		multi = []MetricTagConfig{}
		for _, tag := range tags {
			multi = append(multi, MetricTagConfig{SymbolTag: tag})
		}
	}
	*mtcl = multi
	return nil
}
