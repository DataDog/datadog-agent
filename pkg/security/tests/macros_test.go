package tests

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/policy"
)

func TestMacros(t *testing.T) {
	macros := []*policy.MacroDefinition{
		{
			ID:         "testmacro",
			Expression: `"{{.Root}}/test"`,
		},
		{
			ID:         "testmacro2",
			Expression: `["{{.Root}}/test"]`,
		},
	}

	rules := []*policy.RuleDefinition{
		{
			ID:         "test-rule",
			Expression: `testmacro in testmacro2 && mkdir.filename in testmacro2`,
		},
	}

	test, err := newTestModule(macros, rules)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("test")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(testFile, 0777); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	event, err := test.GetEvent()
	if event.GetType() != "mkdir" {
		t.Errorf("expected mkdir event, got %s", event.GetType())
	}
}
