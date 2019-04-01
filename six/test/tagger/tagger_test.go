package testtagger

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

func TestGetTags(t *testing.T) {
	code := fmt.Sprintf(`
	import json
	with open(r'%s', 'w') as f:
		f.write(json.dumps(tagger.get_tags("base", False)))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "[\"a\", \"b\", \"c\"]" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestGetTagsHighCard(t *testing.T) {
	code := fmt.Sprintf(`
	import json
	with open(r'%s', 'w') as f:
		f.write(json.dumps(tagger.get_tags("base", True)))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "[\"A\", \"B\", \"C\"]" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}

func TestGetTagsUnknown(t *testing.T) {
	code := fmt.Sprintf(`
	import json
	with open(r'%s', 'w') as f:
		f.write(json.dumps(tagger.get_tags("default_switch", True)))
	`, tmpfile.Name())
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "[]" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}
