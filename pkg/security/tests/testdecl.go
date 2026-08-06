// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package tests holds tests related files
package tests

// The declaration registry lets a test state the static module config it needs
// next to the test function instead of at its newTestModule call site:
//
//	var _ = declare(TestDNSResolver, testOpts{networkIngressEnabled: true})
//
//	func TestDNSResolver(t *testing.T) {
//		test, err := newTestModule(t, nil, ruleDefs)
//
// newTestModule rebuilds the eBPF manager whenever a test's static config
// differs from the live module's, and a rebuild costs seconds to tens of
// seconds depending on the kernel. Declaring the config makes the partition of
// the suite into "runs that each build the module once" computable at init()
// time, without executing a single test -- see testRuns and -cws-list-groups.
//
// A test with no declaration uses the default config, which is the common case
// (most of the suite), and all of those tests share a single run.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// declaredConfig is the static config declared for one test function.
type declaredConfig struct {
	name      string
	opts      testOpts
	signature string

	// needsFresh marks a test that must run against a module built from
	// scratch even when the live one already has a matching config.
	needsFresh bool

	// ungrouped marks a test that cannot join a declared group, because its
	// config embeds a value only known at run time (which(t, "sleep"),
	// t.TempDir()), because it holds a callback, or because it deliberately
	// builds several different modules. Those tests keep passing withStaticOpts
	// at the call site and are scheduled apart; the declaration exists so that
	// the exception is explicit and reviewable rather than invisible.
	ungrouped bool
	reason    string

	// moduleBuilds is how many modules an ungrouped test builds. Almost always
	// one; a test that switches config mid-run declares more, so that the
	// rebuild guardrail stays exact instead of being merely an upper bound.
	moduleBuilds int

	declaredAt string
}

// declaredConfigs is keyed by test function name ("TestFoo"). Populated at
// init() time by the declare* functions, read-only afterwards.
var declaredConfigs = map[string]*declaredConfig{}

// declOption modifies a declaration.
type declOption func(*declaredConfig)

// needsFreshModule marks a test as requiring a module built from scratch. It is
// the declared form of withForceReload(): a test that needs a module nobody
// else has touched cannot share one, so the scheduler has to know about it to
// predict how many rebuilds a run will cost.
func needsFreshModule() declOption {
	return func(d *declaredConfig) { d.needsFresh = true }
}

// buildsModules declares how many modules an ungrouped test builds, for the
// tests that switch static config partway through. Defaults to one.
func buildsModules(n int) declOption {
	return func(d *declaredConfig) { d.moduleBuilds = n }
}

// declare records the static config a test needs, so that newTestModule can
// pick it up from t.Name() and the scheduler can group the test with others
// that use the same config. It is meant to be called from a package-level var:
//
//	var _ = declare(TestFoo, testOpts{networkIngressEnabled: true})
//
// fn is the test function itself, not its name, so a rename cannot leave the
// declaration pointing at nothing.
func declare(fn any, opts testOpts, mods ...declOption) struct{} {
	d := &declaredConfig{
		name:       testFuncName(fn),
		opts:       opts,
		signature:  configSignature(opts),
		declaredAt: declarationSite(),
	}
	if bad := undeclarableFields(opts); len(bad) > 0 {
		panic(fmt.Sprintf("declare(%s): cannot declare a config that sets %s: "+
			"testOpts.Equal is reflect.DeepEqual, which reports any two non-nil funcs "+
			"as different, so such a config never matches a live module and would "+
			"rebuild it inside its own group. Keep passing withStaticOpts at the call "+
			"site and mark the test with declareUngrouped",
			d.name, strings.Join(bad, ", ")))
	}
	return register(d, mods)
}

// declareUngrouped marks a test that cannot share a module with any other:
// because its config is only knowable once it runs, because it holds a
// callback, or because it deliberately builds several. The test keeps passing
// withStaticOpts at its call site; declaring it records why it is exempt and
// lets the scheduler account for the rebuilds it will cost, instead of having
// them show up as unexplained ones in the default run.
func declareUngrouped(fn any, reason string, mods ...declOption) struct{} {
	return register(&declaredConfig{
		name:         testFuncName(fn),
		ungrouped:    true,
		reason:       reason,
		moduleBuilds: 1,
		declaredAt:   declarationSite(),
	}, mods)
}

// Recurring declareUngrouped reasons.
const (
	// reasonTempDir covers the activity-dump and security-profile tests, which
	// each build their own t.TempDir() into activityDumpLocalStorageDirectory or
	// securityProfileDir. No two of them ever have equal static configs, so each
	// rebuilds the module and grouping was never going to help them.
	//
	// TODO(PR 3): neither field affects the eBPF program set -- both are only
	// rendered into the config file genTestConfigs writes. Letting the harness
	// own the directory and expose it (test.activityDumpDir()) would take them
	// out of testOpts entirely and turn these into ordinary declared tests. See
	// the "Most opts are static by accident" table in CWS_TEST_GROUPING_PLAN.md.
	reasonTempDir = "config embeds t.TempDir()"

	// reasonCallback covers the configs holding a func. reflect.DeepEqual
	// reports any two non-nil funcs as different, so such a test rebuilds on the
	// way in *and* forces the next test to rebuild on the way out.
	//
	// TODO(PR 2): move preStartCallback and ruleMatchHandler to dynamicTestOpts.
	// They are per-invocation hooks, not part of the static config.
	reasonCallback = "config holds a callback, which never compares equal"
)

func register(d *declaredConfig, mods []declOption) struct{} {
	for _, mod := range mods {
		mod(d)
	}
	if prev, ok := declaredConfigs[d.name]; ok {
		panic(fmt.Sprintf("declare(%s): already declared at %s (re-declared at %s)",
			d.name, prev.declaredAt, d.declaredAt))
	}
	declaredConfigs[d.name] = d
	return struct{}{}
}

// declarationSite returns the file:line of the declare* call, for error
// messages that have to point at one of two scattered declarations.
func declarationSite() string {
	// 0 = declarationSite, 1 = declare/declareRuntimeConfig, 2 = the caller.
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		return "unknown"
	}
	return fmt.Sprintf("%s:%d", file[strings.LastIndex(file, "/")+1:], line)
}

// testFuncName resolves a test function value to its bare name, turning
// "github.com/DataDog/datadog-agent/pkg/security/tests.TestFoo" into "TestFoo".
func testFuncName(fn any) string {
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func {
		panic(fmt.Sprintf("declare: want a test function, got %T", fn))
	}
	f := runtime.FuncForPC(v.Pointer())
	if f == nil {
		panic("declare: cannot resolve the name of the given function")
	}
	full := f.Name()
	name := full[strings.LastIndex(full, "/")+1:] // "tests.TestFoo"
	if i := strings.Index(name, "."); i >= 0 {
		name = name[i+1:] // "TestFoo"
	}
	if strings.ContainsAny(name, ".·") {
		panic(fmt.Sprintf("declare: %q is not a top-level test function; pass the "+
			"function itself, not a closure or a method value", full))
	}
	if !strings.HasPrefix(name, "Test") && !strings.HasPrefix(name, "Benchmark") {
		panic(fmt.Sprintf("declare: %q is neither a test nor a benchmark function", full))
	}
	return name
}

// lookupDeclaredConfig resolves the declaration covering a running test, whose
// t.Name() may be a subtest path. It walks up the path so that a subtest is
// covered by its parent's declaration: "TestX/a/b" tries "TestX/a/b", then
// "TestX/a", then "TestX".
func lookupDeclaredConfig(name string) (*declaredConfig, bool) {
	for {
		if d, ok := declaredConfigs[name]; ok {
			return d, true
		}
		i := strings.LastIndex(name, "/")
		if i < 0 {
			return nil, false
		}
		name = name[:i]
	}
}

// undeclarableFields reports the fields of opts whose *identity*, rather than
// their content, decides config equality: funcs, interfaces and pointers.
// testOpts.Equal is reflect.DeepEqual, which reports two non-nil funcs as
// different even when they are the same function, so a config holding one never
// compares equal to the live module's -- it would rebuild every time, including
// in the middle of the group it was supposed to share. Such a config cannot be
// declared as data; see declareRuntimeConfig.
func undeclarableFields(opts testOpts) []string {
	var bad []string
	v := reflect.ValueOf(opts)
	tp := v.Type()
	for i := range v.NumField() {
		switch v.Field(i).Kind() {
		// Only kinds reflect.Value.IsNil accepts; a slice is fine, DeepEqual
		// compares its contents.
		case reflect.Func, reflect.Interface, reflect.Map, reflect.Chan, reflect.Pointer:
			if !v.Field(i).IsNil() {
				bad = append(bad, tp.Field(i).Name)
			}
		}
	}
	return bad
}

// configSignature is a stable key for the equality class of a static config.
//
// Invariant: for any two configs that undeclarableFields accepts,
// a.Equal(b) == (configSignature(a) == configSignature(b)). Grouping relies on
// it: the signature decides which tests share a run, Equal decides whether
// newTestModule actually reuses the module. If the two disagree, grouping stops
// working without failing anything. TestConfigSignatureMatchesEqual checks it.
func configSignature(opts testOpts) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%#v", opts)))
	return hex.EncodeToString(sum[:6])
}

// defaultConfigSignature is the signature of the config a test gets when it
// declares nothing.
func defaultConfigSignature() string {
	return configSignature(testOpts{})
}

// nonDefaultFields renders the fields of opts that differ from the default
// config, so that -cws-list-groups says what a group actually is rather than
// just showing a hash.
func nonDefaultFields(opts testOpts) []string {
	var out []string
	v := reflect.ValueOf(opts)
	tp := v.Type()
	for i := range v.NumField() {
		if f := v.Field(i); !f.IsZero() {
			out = append(out, tp.Field(i).Name+"="+renderFieldValue(f))
		}
	}
	return out
}

var durationType = reflect.TypeOf(time.Duration(0))

// renderFieldValue formats a testOpts field without going through
// reflect.Value.Interface(), which panics on the unexported fields of testOpts.
func renderFieldValue(v reflect.Value) string {
	switch v.Kind() {
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.Type() == durationType {
			return time.Duration(v.Int()).String()
		}
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.String:
		return v.String()
	case reflect.Slice, reflect.Array:
		parts := make([]string, v.Len())
		for i := range parts {
			parts[i] = renderFieldValue(v.Index(i))
		}
		return "[" + strings.Join(parts, ",") + "]"
	default:
		return "<set>"
	}
}

// undeclaredStaticOpts records tests that pass withStaticOpts without declaring
// anything. They land in the default run and make it rebuild the module, which
// is the one thing grouping exists to avoid -- but while the migration is in
// progress this is a warning, not a failure. reportUndeclaredStaticOpts prints
// the tally at the end of the run.
var (
	undeclaredMu         sync.Mutex
	undeclaredStaticOpts = map[string]struct{}{}
)

func recordUndeclaredStaticOpts(name string) {
	undeclaredMu.Lock()
	defer undeclaredMu.Unlock()
	undeclaredStaticOpts[name] = struct{}{}
}

func reportUndeclaredStaticOpts() {
	undeclaredMu.Lock()
	defer undeclaredMu.Unlock()
	if len(undeclaredStaticOpts) == 0 {
		return
	}
	names := make([]string, 0, len(undeclaredStaticOpts))
	for name := range undeclaredStaticOpts {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Printf("[grouping] %d test(s) passed withStaticOpts without a declared "+
		"config, each costing the default run an extra module rebuild: %s\n",
		len(names), strings.Join(names, " "))
}

// resolveStaticOpts decides which static config a newTestModule call runs with:
// the call site's withStaticOpts when it supplied one, otherwise the config
// declared for this test, otherwise the default config.
func resolveStaticOpts(t testing.TB, opts *tmOpts) {
	decl, declared := lookupDeclaredConfig(t.Name())

	switch {
	case opts.staticOptsSet && declared && !decl.ungrouped:
		// Letting the call site win over the test's own declaration would put
		// the test in a group whose config it does not use, and rebuild the
		// module in the middle of that group.
		t.Fatalf("%s passes withStaticOpts but is also declared at %s: drop one of "+
			"the two, or switch the declaration to declareUngrouped if the config "+
			"is only known at run time", t.Name(), decl.declaredAt)

	case opts.staticOptsSet && !declared:
		recordUndeclaredStaticOpts(t.Name())

	case !opts.staticOptsSet && declared && !decl.ungrouped:
		opts.staticOpts = decl.opts
	}

	if declared && decl.needsFresh {
		opts.forceReload = true
	}
}
