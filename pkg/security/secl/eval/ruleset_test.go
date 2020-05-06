package eval

import (
	"syscall"
	"testing"
)

type fakeHandler struct {
	discriminators []string
}

func (f *fakeHandler) RuleMatch(rule *Rule, event Event) {

}
func (f *fakeHandler) DiscriminatorDiscovered(event Event, field string) {
	f.discriminators = append(f.discriminators, field)
}

func TestRuleSet(t *testing.T) {
	event := testEvent{
		kind: "fs",
		process: testProcess{
			name:   "abc",
			uid:    123,
			isRoot: true,
		},
		open: testOpen{
			filename: "/usr/local/bin/rootkit",
			flags:    syscall.O_RDONLY,
		},
	}

	f := &fakeHandler{}

	m := &testModel{}
	rs := NewRuleSet(m, true)
	rs.AddListener(f)

	_, err := rs.AddRule("r1", `(open.filename =~ "/sbin/*" || open.filename =~ "/usr/sbin/*") && open.flags & O_CREAT > 0 && process.uid != 0`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rs.AddRule("r2", `(open.filename =~ "/var/run/*") && open.flags & O_CREAT > 0`)
	if err != nil {
		t.Fatal(err)
	}

	rs.Evaluate(&event)

	t.Logf("Event: %+v", f.discriminators)
}
