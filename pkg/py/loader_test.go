package py

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/loader"
	"github.com/sbinet/go-python"
	"gopkg.in/yaml.v2"
)

func TestLoad(t *testing.T) {
	l := NewPythonCheckLoader()

	var configData loader.RawConfigMap
	yamlFile, err := ioutil.ReadFile("tests/testcheck.yaml")
	yaml.Unmarshal(yamlFile, &configData)
	fmt.Println(configData)

	config := loader.CheckConfig{Name: "testcheck", Data: configData}
	instances, err := l.Load(config)
	if err != nil {
		t.Fatalf("Expected nil, found: %v", err)
	}
	if len(instances) != 2 {
		t.Fatalf("Expected len 2, found: %d", len(instances))
	}

	config = loader.CheckConfig{Name: "doesntexist", Data: configData}
	instances, err = l.Load(config)
	if err == nil {
		t.Fatal("Expected err, found: nil")
	}
	if len(instances) != 0 {
		t.Fatalf("Expected len 0, found: %d", len(instances))
	}

}

func TestNewPythonCheckLoader(t *testing.T) {
	// Lock the GIL and release it at the end of the run
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	loader := NewPythonCheckLoader()
	if loader == nil {
		t.Fatalf("Expected PythonCheckLoader instance, found nil")
	}
}
