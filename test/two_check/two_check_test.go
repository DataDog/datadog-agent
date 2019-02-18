package two_check

import "testing"

func TestGetCheckAgent(t *testing.T) {
	check := getFakeCheck()

	if check == nil {
		t.Fatal("Check not found")
	}
}

func TestRunCheckAgent(t *testing.T) {
	res := runFakeCheck()

	if res == "" {
		t.Fatal("Run failed")
	}
}

func TestGetCheckClassExistingModule(t *testing.T) {
	moduleName := "fake_check"
	klass := getCheckClass(moduleName)

	if klass == nil {
		t.Fatalf("Check class not found in module %s", moduleName)
	}
}

func TestGetCheckClassUnexistingModule(t *testing.T) {
	moduleName := "unexising_module"
	klass := getCheckClass(moduleName)

	if klass != nil {
		t.Fatalf("Check class found in module %s", moduleName)
	}
}
