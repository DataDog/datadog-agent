package testuutil

import (
	"fmt"
	"os"
	"testing"

	"github.com/StackVista/stackstate-agent/rtloader/test/helpers"
)

var (
	stdout       string
	stderr       string
	setException bool
	exception    string
	retCode      int
	args         []string
)

func resetTest() {
	stdout = ""
	stderr = ""
	setException = false
	exception = ""
	retCode = 0
	args = nil
}

func TestMain(m *testing.M) {
	err := setUp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up tests: %v", err)
		os.Exit(-1)
	}

	os.Exit(m.Run())
}

func TestSubprocessOutputWrongArg(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := fmt.Sprintf(`_util.subprocess_output()`)
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "TypeError: get_subprocess_output() takes at least 1 argument (0 given)" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSubprocessOutputEmptyList(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	code := fmt.Sprintf(`_util.subprocess_output([])`)
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "TypeError: invalid command: empty list" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSubprocessOutput(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	stdout = "/tmp"
	code := fmt.Sprintf(`
	stdout, stderr, ret = _util.subprocess_output(["ls"], False)
	with open(r'%s', 'w') as f:
		f.write(stdout + " | " + stderr + " | " + str(ret))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "/tmp |  | 0" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestGetSubprocessOutput(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	stdout = "/tmp"
	code := fmt.Sprintf(`
	stdout, stderr, ret = _util.get_subprocess_output(["ls"], False)
	with open(r'%s', 'w') as f:
		f.write(stdout + " | " + stderr + " | " + str(ret))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "/tmp |  | 0" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestGetSubprocessOutputStderr(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	stdout = "/tmp"
	stderr = "some error"
	code := fmt.Sprintf(`
	stdout, stderr, ret = _util.get_subprocess_output(["ls"], False)
	with open(r'%s', 'w') as f:
		f.write(stdout + " | " + stderr + " | " + str(ret))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "/tmp | some error | 0" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestGetSubprocessOutputRetCode(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	stdout = "/tmp"
	retCode = 21
	code := fmt.Sprintf(`
	stdout, stderr, ret = _util.get_subprocess_output(["ls"], False)
	with open(r'%s', 'w') as f:
		f.write(stdout + " | " + stderr + " | " + str(ret))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "/tmp |  | 21" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestGetSubprocessOutputStderrRetCode(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	stdout = "/tmp"
	stderr = "some error"
	retCode = 21
	code := fmt.Sprintf(`
	stdout, stderr, ret = _util.get_subprocess_output(["ls"], False)
	with open(r'%s', 'w') as f:
		f.write(stdout + " | " + stderr + " | " + str(ret))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "/tmp | some error | 21" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestGetSubprocessOutputErrorNotList(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	out, err := run(`_util.get_subprocess_output("ls", False)`)

	if err != nil {
		t.Fatal(err)
	}
	if out != "TypeError: command args not a list" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestGetSubprocessOutputErrorNotBool(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	out, err := run(`_util.get_subprocess_output(["ls"], 1)`)

	if err != nil {
		t.Fatal(err)
	}
	if out != "TypeError: bad raise_on_empty argument: should be bool" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestGetSubprocessOutputErrorCommandArgsNotString(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	out, err := run(`_util.get_subprocess_output(["ls", 123], False)`)

	if err != nil {
		t.Fatal(err)
	}
	if out != "TypeError: command argument must be valid strings" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSubprocessOutputRaiseEmptyStdout(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	stdout = "" // setting empty output
	code := fmt.Sprintf(`_util.subprocess_output(["ls"], True)`)
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "SubprocessOutputEmptyError: get_subprocess_output expected output but had none." {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSubprocessOutputRaiseException(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	setException = true
	exception = "THIS IS AN ERROR FROM GO"
	code := fmt.Sprintf(`_util.subprocess_output(["ls"], False)`)
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "Exception: THIS IS AN ERROR FROM GO" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSubprocessOutputRaiseEmptyException(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	setException = true
	exception = ""
	code := fmt.Sprintf(`_util.subprocess_output(["ls"], False)`)
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "Exception:" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}
