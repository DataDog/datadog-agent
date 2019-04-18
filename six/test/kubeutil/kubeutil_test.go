package testkubeutil

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	err := setUp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up tests: %v", err)
		os.Exit(-1)
	}

	ret := m.Run()
	tearDown()

	os.Exit(ret)
}

func TestGetConnectionInfo(t *testing.T) {
	code := fmt.Sprintf(`
	d = kubeutil.get_connection_info()
	with open(r'%s', 'w') as f:
		f.write(",".join(sorted(d.keys())))
		f.write("-")
		f.write(",".join(sorted(d.values())))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "BarKey,FooKey-BarValue,FooValue" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestGetConnectionInfoNoKubeutil(t *testing.T) {
	returnNull = true
	defer func() { returnNull = false }()

	code := fmt.Sprintf(`
	d = kubeutil.get_connection_info()
	with open(r'%s', 'w') as f:
		f.write("{}".format(d))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "{}" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}
