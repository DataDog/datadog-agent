package tests

import (
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/policy"
)

func TestMacros(t *testing.T) {
	macros := []*policy.MacroDefinition{
		{
			ID:         "testmacro",
			Expression: `"/test"`,
		},
		{
			ID:         "testmacro2",
			Expression: `["/test"]`,
		},
	}

	rules := []*policy.RuleDefinition{
		{
			ID:         "test-rule",
			Expression: `testmacro in testmacro2 && mkdir.filename in testmacro2`,
		},
	}

	test, err := newSimpleTest(macros, rules)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	// Simple generate an event
	if err := os.Mkdir("/tmp/test", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.Remove("/tmp/test")

	event, err := test.client.GetEvent(3 * time.Second)
	if event.GetType() != "mkdir" {
		t.Errorf("expected mkdir event, got %s", event.GetType())
	}
}
