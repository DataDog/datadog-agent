package snmp

import "strconv"

// StringArray is list of string with a yaml un-marshaller that support both array and string.
// See test file for example usage.
// Credit: https://github.com/go-yaml/yaml/issues/100#issuecomment-324964723
type StringArray []string

// Number can unmarshal yaml string or integer
type Number int

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
