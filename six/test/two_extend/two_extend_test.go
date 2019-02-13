package two_extend

import "testing"

func TestExtend(t *testing.T) {
	output, err := extend()

	if err != nil {
		t.Fatal(err)
	}

	if output != "I'm extending Python!\n" {
		t.Errorf("Unexpected printed value: '%s'", output)
	}
}
