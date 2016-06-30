package py

import (
	"fmt"
	"io/ioutil"
	"testing"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/loader"
	"github.com/sbinet/go-python"
)

func getCheckInstance() *PythonCheck {
	// Lock the GIL and release it at the end of the run
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	module := python.PyImport_ImportModuleNoBlock("testcheck")
	checkClass := module.GetAttrString("TestCheck")
	return NewPythonCheck(checkClass, python.PyTuple_New(0))
}

// TODO check arguments as soon as the feature is complete
func TestNewPythonCheck(t *testing.T) {
	// Lock the GIL and release it at the end of the run
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	module := python.PyImport_ImportModuleNoBlock("testcheck")
	checkClass := module.GetAttrString("TestCheck")
	check := NewPythonCheck(checkClass, python.PyTuple_New(0))

	if check.Instance.IsInstance(checkClass) != 1 {
		t.Fatalf("Expected instance of class TestCheck, found: %s",
			python.PyString_AsString(check.Instance.GetAttrString("__class__")))
	}

	// this should fail b/c FooCheck constructors takes parameters
	fooClass := module.GetAttrString("FooCheck")
	check = NewPythonCheck(fooClass, python.PyTuple_New(0))

	if check != nil {
		t.Fatalf("nil expected, found: %v", check)
	}
}

func TestRun(t *testing.T) {
	check := getCheckInstance()
	if err := check.Run(); err != nil {
		t.Fatalf("Expected error nil, found: %s", err)
	}
}

func TestStr(t *testing.T) {
	check := getCheckInstance()
	name := "TestCheck"
	if check.String() != name {
		t.Fatalf("Expected %s, found: %v", name, check)
	}

	check.Instance = nil
	if check.String() != "" {
		t.Fatalf("Expected empty string, found: %v", check)
	}
}

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

func BenchmarkRun(b *testing.B) {
	check := getCheckInstance()
	for n := 0; n < b.N; n++ {
		check.Run()
	}
}
