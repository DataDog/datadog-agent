// +build python,test

package python

import "testing"

func TestParsingMapWithDifferentTypes(t *testing.T) {
	testParsingMapWithDifferentTypes(t)
}

func TestParsingYamlToJsonMap(t *testing.T) {
	testParsingInnerMapsWithStringKey(t)
}

func TestParsingANonMapYaml(t *testing.T) {
	testErrorParsingNonMapYaml(t)
}

func TestErrorParsingNonStringKeys(t *testing.T) {
	testErrorParsingNonStringKeys(t)
}
