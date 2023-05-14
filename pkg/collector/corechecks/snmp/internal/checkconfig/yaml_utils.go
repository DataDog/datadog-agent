// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checkconfig

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/snmp/snmpintegration"
)

// StringArray is list of string with a yaml un-marshaller that support both array and string.
// See test file for example usage.
// Credit: https://github.com/go-yaml/yaml/issues/100#issuecomment-324964723
type StringArray []string

// Number can unmarshal yaml string or integer
type Number int

// Boolean can unmarshal yaml string or bool value
type Boolean bool

// InterfaceConfigs can unmarshal yaml []snmpintegration.InterfaceConfig or interface configs in json format
// Example of interface configs in json format: `[{"match_field":"name","match_value":"eth0","in_speed":25,"out_speed":10}]`
type InterfaceConfigs []snmpintegration.InterfaceConfig

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

// UnmarshalYAML unmarshalls Number
func (n *Number) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var integer int
	err := unmarshal(&integer)
	if err != nil {
		var str string
		err := unmarshal(&str)
		if err != nil {
			return err
		}
		num, err := strconv.Atoi(str)
		if err != nil {
			return err
		}
		*n = Number(num)
	} else {
		*n = Number(integer)
	}
	return nil
}

// UnmarshalYAML unmarshalls Boolean
func (b *Boolean) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var value bool
	err := unmarshal(&value)
	if err != nil {
		var str string
		err := unmarshal(&str)
		if err != nil {
			return err
		}
		switch str {
		case "true":
			value = true
		case "false":
			value = false
		default:
			return fmt.Errorf("cannot convert `%s` to boolean", str)
		}
		value = str == "true"
		*b = Boolean(value)
	} else {
		*b = Boolean(value)
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
			multi = append(multi, MetricTagConfig{symbolTag: tag})
		}
	}
	*mtcl = multi
	return nil
}

// UnmarshalYAML unmarshalls InterfaceConfigs
func (ic *InterfaceConfigs) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var ifConfigs []snmpintegration.InterfaceConfig
	err := unmarshal(&ifConfigs)
	if err != nil {
		var ifConfigJson string
		err := unmarshal(&ifConfigJson)
		if err != nil {
			return fmt.Errorf("cannot unmarshall to string: %s", err)
		}
		if ifConfigJson == "" {
			return nil
		}
		err = json.Unmarshal([]byte(ifConfigJson), &ifConfigs)
		if err != nil {
			return fmt.Errorf("cannot unmarshall json to []snmpintegration.InterfaceConfig: %s", err)
		}
	}
	*ic = ifConfigs
	return nil
}
