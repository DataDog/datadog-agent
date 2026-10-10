package eval

import (
	"strings"
	"testing"
)

// TestUnsupportedOperatorOnCIDRArray checks that an unsupported operator between a CIDR array
// field and a CIDR field is reported as an error rather than crashing the compiler.
//
// An ast.Comparison carries either a scalar comparison or an array one, never both, so the
// unknown-operator error of each branch has to read its operator from the branch it is in.
// Reading it from the other dereferences nil, and since rules come from policy configuration
// that turns a malformed rule into a crash of rule compilation rather than a rejection.
func TestUnsupportedOperatorOnCIDRArray(t *testing.T) {
	model := &testModel{}
	opts := newOptsWithParams(testConstants, nil)

	for _, expr := range []string{
		`network.ips >= network.ip`,
		`network.ips > network.ip`,
		`network.ips <= network.ip`,
		`network.ips < network.ip`,
	} {
		t.Run(expr, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("compiling `%s` panicked: %v", expr, r)
				}
			}()

			_, err := parseRule(expr, model, opts)
			if err == nil {
				t.Fatalf("expected `%s` to be rejected", expr)
			}
			if !strings.Contains(err.Error(), "operator") {
				t.Errorf("expected an unknown operator error, got: %s", err)
			}
		})
	}

	// the supported operators on that pair must keep working
	for _, expr := range []string{
		`network.ips == network.ip`,
		`network.ips != network.ip`,
	} {
		t.Run(expr, func(t *testing.T) {
			if _, err := parseRule(expr, model, opts); err != nil {
				t.Fatalf("expected `%s` to compile, got: %s", expr, err)
			}
		})
	}
}
