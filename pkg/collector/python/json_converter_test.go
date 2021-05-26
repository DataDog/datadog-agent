// +build python,test

package python

import "testing"

func TestConvertingMapWithDifferentTypes(t *testing.T) {
	testConvertingMapWithDifferentTypes(t)
}

func TestConvertingYamlToJsonMap(t *testing.T) {
	testConvertingInnerMapsWithStringKey(t)
}

func TestConvertingANonMapYaml(t *testing.T) {
	testConvertingNonMapYaml(t)
}

func TestConvertingNonStringKeysYaml(t *testing.T) {
	testConvertingNonStringKeysYaml(t)
}
