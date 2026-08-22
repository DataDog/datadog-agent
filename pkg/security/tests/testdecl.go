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
// newTestModule rebuilds the eBPF manager whenever a test's static config
// differs from the live module's, which costs seconds to tens of seconds
// depending on the kernel. Declaring it makes the partition into runs that each
// build the module once computable at init() time, without running a test --
// see testruns.go. A test that declares nothing gets the default config, which
// is most of the suite and one shared run.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// declaredConfig is the static config declared for one test function.
type declaredConfig struct {
	name      string
	opts      testOpts
	signature string

	// inlineConfig marks a test that passes its config to newTestModule itself
	// and so builds its own module; reason says why it cannot share one.
	inlineConfig bool
	reason       string

	// moduleBuilds is how many modules an inline-config test builds, >1 only for
	// the tests that switch config mid-run. Feeds testRun.ExpectedRebuilds.
	moduleBuilds int

	declaredAt string
}

// declaredConfigs is keyed by test function name ("TestFoo"), populated at
// init() time by the declare* functions.
var declaredConfigs = map[string]*declaredConfig{}

type declOption func(*declaredConfig)

func buildsModules(n int) declOption {
	return func(d *declaredConfig) { d.moduleBuilds = n }
}

// declare records the static config a test needs, from a package-level var:
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
	if bad := unshareableFields(opts); len(bad) > 0 {
		panic(fmt.Sprintf("declare(%s): a config setting %s never compares equal to "+
			"the live module's, so it would rebuild inside its own group: pass it with "+
			"withStaticOpts and declare the test with declareInlineConfig",
			d.name, strings.Join(bad, ", ")))
	}
	return register(d, mods)
}

// declareInlineConfig marks a test that passes its config to newTestModule
// itself, because the config is only knowable once the test runs, holds a
// callback, or the test deliberately builds several modules. It is scheduled
// apart from the declared groups, and reason records why.
func declareInlineConfig(fn any, reason string, mods ...declOption) struct{} {
	return register(&declaredConfig{
		name:         testFuncName(fn),
		inlineConfig: true,
		reason:       reason,
		moduleBuilds: 1,
		declaredAt:   declarationSite(),
	}, mods)
}

// Recurring declareInlineConfig reasons.
const (
	// reasonTempDir covers the activity-dump and security-profile tests, whose
	// t.TempDir() lands in activityDumpLocalStorageDirectory or
	// securityProfileDir, so no two of them ever have equal configs.
	reasonTempDir = "config embeds t.TempDir()"

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

// declarationSite returns the file:line of the declare* call, so an error can
// point at a declaration that may be in any file in the package.
func declarationSite() string {
	// 0 = declarationSite, 1 = declare/declareInlineConfig, 2 = the caller.
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

// lookupDeclaredConfig walks up a t.Name() subtest path so that a subtest is
// covered by its parent's declaration: "TestX/a/b", then "TestX/a", then "TestX".
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

// unshareableFields reports the fields of opts whose identity, rather than
// their content, decides equality: testOpts.Equal is reflect.DeepEqual, which
// calls two non-nil funcs different even when they are the same function.
func unshareableFields(opts testOpts) []string {
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
// Invariant: for any config unshareableFields accepts,
// a.Equal(b) == (signature(a) == signature(b)). The signature decides which
// tests share a run, Equal decides whether newTestModule really reuses the
// module; if they disagree, grouping silently stops paying off.
func configSignature(opts testOpts) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%#v", opts)))
	return hex.EncodeToString(sum[:6])
}

func defaultConfigSignature() string {
	return configSignature(testOpts{})
}

// nonDefaultFields renders what a group actually is, so -list-groups shows
// more than a hash.
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

// resolveStaticOpts decides which static config a newTestModule call runs with:
// the call site's withStaticOpts when it supplied one, otherwise the config
// declared for this test, otherwise the default config.
func resolveStaticOpts(t testing.TB, opts *tmOpts) {
	decl, declared := lookupDeclaredConfig(t.Name())

	switch {
	case opts.staticOptsSet && declared && !decl.inlineConfig:
		// Letting the call site win would run the test in a group whose config
		// it does not use, rebuilding the module in the middle of that group.
		t.Fatalf("%s passes withStaticOpts but is also declared at %s: drop one of "+
			"the two, or switch the declaration to declareInlineConfig if the test "+
			"supplies its own config at the call site", t.Name(), decl.declaredAt)

	case (opts.staticOptsSet || opts.forceReload) && !declared:
		// Both flags mean "build me my own module", and undeclared puts that
		// build inside the default run, which is scheduled to cost exactly one.
		// newTestModule applies fopts before calling us, so forceReload is set.
		t.Fatalf("%s builds a module of its own -- it passes withStaticOpts or "+
			"withForceReload -- but declares nothing, so it would do it inside the "+
			"default run: add declareInlineConfig(%s, \"<why it cannot share one>\") "+
			"next to the test", t.Name(), t.Name())

	case declared && decl.inlineConfig && !opts.staticOptsSet && !opts.forceReload:
		// The converse: the declaration promises a module of its own, so the
		// test is scheduled apart and charged a rebuild, but this call takes the
		// default config and would just ride the live module.
		t.Fatalf("%s is declared at %s as supplying its own config, but this "+
			"newTestModule call passes neither withStaticOpts nor withForceReload: "+
			"supply one, or drop the declaration so the test joins the default run",
			t.Name(), decl.declaredAt)

	case !opts.staticOptsSet && declared && !decl.inlineConfig:
		opts.staticOpts = decl.opts
	}
}
