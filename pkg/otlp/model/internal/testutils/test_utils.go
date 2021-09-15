package testutils

import "go.opentelemetry.io/collector/model/pdata"

func fillAttributeMap(attrs pdata.AttributeMap, mp map[string]string) {
	attrs.Clear()
	attrs.EnsureCapacity(len(mp))
	for k, v := range mp {
		attrs.Insert(k, pdata.NewAttributeValueString(v))
	}
}

// NewAttributeMap creates a new attribute map (string only)
// from a Go map
func NewAttributeMap(mp map[string]string) pdata.AttributeMap {
	attrs := pdata.NewAttributeMap()
	fillAttributeMap(attrs, mp)
	return attrs
}
