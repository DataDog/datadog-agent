// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profiledefinition

import (
	"errors"

	"gopkg.in/yaml.v2"
)

// unmarshalWithFirstError calls unmarshal(arg), but if the result is a
// yaml.TypeError it replaces the errors in it with the errors from the original
// error.
//
// This is needed because if you call `unmarshal` twice in the same method, and
// it generates an error both times, the returned errors are actually the same
// object, so the second call will modify the previously-returned error value,
// overwriting whatever the original errors were. Usually this is not what we
// want when we have the pattern "try to parse the input as the expected type,
// and if that fails try parsing it as a legacy format" - if both fail, we want
// the error for the expected type.
//
// For example, if the main type is a struct but you also support a legacy type
// where it was just a string, using this method ensures that a malformed struct
// will get you an error message like "field X not found in type Y" instead of
// "cannot unmarshal !!map into string".
func unmarshalWithFirstError(err error, unmarshal func(any) error, arg any) error {
	var typeErrors []string
	if typeErr, ok := err.(*yaml.TypeError); ok {
		typeErrors = append(typeErrors, typeErr.Errors...)
	}
	newErr := unmarshal(arg)
	if newErr != nil {
		if typeErr, ok := newErr.(*yaml.TypeError); len(typeErrors) > 0 && ok {
			typeErr.Errors = typeErrors
			return typeErr
		}
		return newErr
	}
	return nil
}

// StringArray is list of string with a yaml un-marshaller that support both array and string.
// See test file for example usage.
// Credit: https://github.com/go-yaml/yaml/issues/100#issuecomment-324964723
type StringArray []string

// UnmarshalYAML unmarshalls StringArray
func (a *StringArray) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var multi []string
	err := unmarshal(&multi)
	if err != nil {
		var single string
		if err := unmarshalWithFirstError(err, unmarshal, &single); err != nil {
			return err
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
		var str string
		if err := unmarshalWithFirstError(err, unmarshal, &str); err != nil {
			return err
		}
		*a = SymbolConfigCompat(SymbolConfig{Name: str})
	} else {
		*a = SymbolConfigCompat(symbol)
	}
	return nil
}

// UnmarshalYAML unmarshalls MetricTagConfigList
func (mtcl *MetricTagConfigList) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var multi []MetricTagConfig
	err := unmarshal(&multi)
	if err != nil {
		var tags []string
		if err := unmarshalWithFirstError(err, unmarshal, &tags); err != nil {
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

// UnmarshalYAML unmarshalls MetricsConfig
func (mc *MetricsConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type metricsConfig MetricsConfig
	err := unmarshal((*metricsConfig)(mc))

	var rawData map[string]interface{}
	if err := unmarshalWithFirstError(err, unmarshal, &rawData); err != nil {
		return err
	}

	mibString, mibIsString := rawData["MIB"].(string)
	if mibIsString && mibString != "" {
		symbolString, symbolIsString := rawData["symbol"].(string)
		if symbolIsString && symbolString != "" {
			return errors.Join(err, ErrLegacySymbolType)
		}
	}

	return err
}
