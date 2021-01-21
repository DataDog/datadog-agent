package snmp

// StringArray is list of string with a yaml un-marshaller that support both array and string.
// See test file for example usage.
// Credit: https://github.com/go-yaml/yaml/issues/100#issuecomment-324964723
type StringArray []string

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
