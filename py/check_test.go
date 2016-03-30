package py

import (
	"testing"

	"github.com/sbinet/go-python"
)

func getCheckInstance() *PythonCheck {
	// Lock the GIL and release it at the end of the run
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	module := python.PyImport_ImportModuleNoBlock("tests.testcheck")
	checkClass := module.GetAttrString("TestCheck")
	checkConfig, _ := getCheckConfig("tests", "testcheck")
	return NewPythonCheck(checkClass, checkConfig)
}

// TODO check arguments as soon as the feature is complete
func TestNewPythonCheck(t *testing.T) {
	// Lock the GIL and release it at the end of the run
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	module := python.PyImport_ImportModuleNoBlock("tests.testcheck")
	checkClass := module.GetAttrString("TestCheck")
	check := NewPythonCheck(checkClass, CheckConfig{})

	if check.Instance.IsInstance(checkClass) != 1 {
		t.Fatalf("Expected instance of class TestCheck, found: %s",
			python.PyString_AsString(check.Instance.GetAttrString("__class__")))
	}
}

func TestRun(t *testing.T) {
	check := getCheckInstance()
	result, err := check.Run()
	if err != nil {
		t.Fatalf("Expected error nil, found: %s", err)
	}
	out := `{"gauge": [{"Name": "foo", "Value": 0, "Tags": null}, {"Name": "foo", "Value": 1, "Tags": null}]}`
	if result.Result != out {
		t.Fatalf("Expected %s, found: %s", out, result.Result)
	}
	if result.Error != "" {
		t.Fatalf("Expected empty error string, found: %s", result.Error)
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

func TestCollectChecks(t *testing.T) {
	checks := CollectChecks([]string{"tests.testcheck", "doesnt.exist", "tests.foo", "tests.testcheck2"}, "tests")
	if len(checks) != 1 {
		t.Fatalf("Expected 1 check loaded, found: %d", len(checks))
	}
}

func BenchmarkRun(b *testing.B) {
	check := getCheckInstance()
	for n := 0; n < b.N; n++ {
		check.Run()
	}
}
