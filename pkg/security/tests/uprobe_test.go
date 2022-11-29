package tests

import (
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"testing"
)

// TODO
func TestExecBreakpoint(t *testing.T) {
	tests := []struct {
		name  string
		rule  *rules.RuleDefinition
		check func(event *sprobe.Event)
	}{
		{
			name: "breakpoint",
			rule: &rules.RuleDefinition{
				ID:         "test_exec_breakpoint",
				Expression: `exec.file.name == ~"*libssl.so*" && exec.symbol.name == "SSL_write"`,
			},
		},
	}

	var ruleList []*rules.RuleDefinition
	for _, test := range tests {
		ruleList = append(ruleList, test.rule)
	}

	testModule, err := newTestModule(t, nil, ruleList, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer testModule.Close()

	for _, test := range tests {
		testModule.Run(t, test.name, func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testModule.WaitSignal(t, func() error {
				return nil
			}, validateExecEvent(t, noWrapperType, func(event *sprobe.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, test.rule.ID)
				test.check(event)
			}))
		})
	}
}
