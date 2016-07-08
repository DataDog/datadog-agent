package py

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/sbinet/go-python"
)

func TestLoad(t *testing.T) {
	l := NewPythonCheckLoader()
	config := check.Config{Name: "testcheck"}
	config.Instances = append(config.Instances, []byte("foo: bar"))
	config.Instances = append(config.Instances, []byte("bar: baz"))

	instances, err := l.Load(config)
	if err != nil {
		t.Fatalf("Expected nil, found: %v", err)
	}
	if len(instances) != 2 {
		t.Fatalf("Expected len 2, found: %d", len(instances))
	}

	config = check.Config{Name: "doesntexist"}
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
