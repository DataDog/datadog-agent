package profiledefinition

// KeyValue used to represent mapping
// Used for RC compatibility (map to list)
type KeyValue struct {
	Key   string `yaml:"key" json:"key"`
	Value string `yaml:"value" json:"value"`
}

// KeyValueList is a list of mapping key values
type KeyValueList []KeyValue

func (kvl *KeyValueList) ToMap() map[string]string {
	mapping := make(map[string]string)
	for _, item := range *kvl {
		mapping[item.Key] = item.Value
	}
	return mapping
}
