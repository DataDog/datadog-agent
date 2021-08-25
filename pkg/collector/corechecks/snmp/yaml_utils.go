package snmp

import (
	"fmt"
	"strconv"
)

// StringArray is list of string with a yaml un-marshaller that support both array and string.
// See test file for example usage.
// Credit: https://github.com/go-yaml/yaml/issues/100#issuecomment-324964723
type StringArray []string

// Number can unmarshal yaml string or integer
type Number int

// Boolean can unmarshal yaml string or bool value
type Boolean bool

//UnmarshalYAML unmarshalls StringArray
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

//UnmarshalYAML unmarshalls Number
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

//UnmarshalYAML unmarshalls Boolean
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

//UnmarshalYAML unmarshalls metricTagConfigList
func (a *metricTagConfigList) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var multi []metricTagConfig
	err := unmarshal(&multi)
	if err != nil {
		var tags []string
		err := unmarshal(&tags)
		if err != nil {
			return err
		}
		multi = []metricTagConfig{}
		for _, tag := range tags {
			multi = append(multi, metricTagConfig{symbolTag: tag})
		}
	}
	*a = multi
	return nil
}
