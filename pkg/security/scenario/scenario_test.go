package scenario

import (
	"bytes"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
)

func TestSimpleScenario(t *testing.T) {
	yaml := bytes.NewBufferString(`---
name: SimpleScenario
event:
  process:
    name: ls
count: 10000
`)

	scenario, err := NewScenario(yaml)
	if err != nil {
		t.Fatal(err)
	}

	rule, err := ast.ParseRule(`process.name == "ls"`)
	if err != nil {
		t.Fatal(err)
	}

	result, err := scenario.Evaluate(rule)
	if err != nil {
		t.Error(err)
	}

	if !result {
		t.Errorf("expected scenario to return true")
	}
}
