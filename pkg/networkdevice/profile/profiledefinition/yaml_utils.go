// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profiledefinition

import "fmt"

// StringArray is list of string with a yaml un-marshaller that support both array and string.
// See test file for example usage.
// Credit: https://github.com/go-yaml/yaml/issues/100#issuecomment-324964723
type StringArray []string

// UnmarshalYAML unmarshalls StringArray
func (a *StringArray) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var multi []string
	err := unmarshal(&multi)
	if err != nil {
		// we have to cache the error because calling unmarshal again *modifies err in place* for some reason.
		cachedErr := fmt.Errorf("%w", err)
		var single string
		err2 := unmarshal(&single)
		if err2 != nil {
			return cachedErr
		}
		*a = []string{single}
	} else {
		*a = multi
	}
	return nil
}

// UnmarshalYAML unmarshalls SymbolConfig
func (a *SymbolConfigCompat) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var symbol SymbolConfig
	err := unmarshal(&symbol)
	if err != nil {
		// we have to cache the error because calling unmarshal again *modifies err in place* for some reason.
		cachedErr := fmt.Errorf("%w", err)
		var str string
		err2 := unmarshal(&str)
		if err2 != nil {
			return cachedErr
		}
		*a = SymbolConfigCompat(SymbolConfig{Name: str})
	} else {
		*a = SymbolConfigCompat(symbol)
	}
	return nil
}

// UnmarshalYAML unmarshals a MetricTagConfigList.
// If the value is a valid []MetricTagConfig, it will be unmarshalled normally.
// If it's not, we try to unmarshal it as a list of strings, and save those as symbol tags.
func (mtcl *MetricTagConfigList) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var multi []MetricTagConfig
	err := unmarshal(&multi)
	if err != nil {
		// we have to cache the error because calling unmarshal again *modifies err in place* for some reason.
		cachedErr := fmt.Errorf("%w", err)
		var tags []string
		err2 := unmarshal(&tags)
		if err2 != nil {
			return cachedErr
		}
		multi = []MetricTagConfig{}
		for _, tag := range tags {
			multi = append(multi, MetricTagConfig{SymbolTag: tag})
		}
	}
	*mtcl = multi
	return nil
}
