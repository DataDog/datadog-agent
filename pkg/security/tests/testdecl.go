// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package tests holds tests related files
package tests

// The declaration registry lets a test state the static module config it needs
// next to the test function:
//
//	var _ = declare(TestDNSResolver, testOpts{networkIngressEnabled: true})
//
// newTestModule rebuilds the eBPF module whenever a test's static config
// differs from the live one, so declaring it makes the grouping computable at
// init() time (testruns.go). A test that declares nothing gets the default
// config, which is most of the suite and one shared group.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// declaredConfig is the static config declared for one test function.
type declaredConfig struct {
	name      string
	opts      testOpts
	signature string

	// inlineConfig marks a test that passes its config to newTestModule itself
	// and so builds its own module.
	inlineConfig bool

	declaredAt string
}

// declaredConfigs is keyed by test function name ("TestFoo").
var declaredConfigs = map[string]*declaredConfig{}

// declare records the static config a test needs, from a package-level var.
// fn is the function itself, not its name, so a rename cannot leave the
// declaration pointing at nothing.
//
//	var _ = declare(TestFoo, testOpts{networkIngressEnabled: true})
func declare(fn any, opts testOpts) struct{} {
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
	return register(d)
}

// declareInlineConfig marks a test that passes its config to newTestModule
// itself. Such tests are scheduled apart from the declared groups.
func declareInlineConfig(fn any) struct{} {
	return register(&declaredConfig{
		name:         testFuncName(fn),
		inlineConfig: true,
		declaredAt:   declarationSite(),
	})
}

func register(d *declaredConfig) struct{} {
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
	// 0 = here, 1 = declare/declareInlineConfig, 2 = the caller.
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		return "unknown"
	}
	return fmt.Sprintf("%s:%d", file[strings.LastIndex(file, "/")+1:], line)
}

// testFuncName turns ".../pkg/security/tests.TestFoo" into "TestFoo".
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

// lookupDeclaredConfig walks up a t.Name() subtest path -- "TestX/a/b", then
// "TestX/a", then "TestX" -- so a subtest is covered by its parent.
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
		// Only kinds IsNil accepts; a slice is fine, DeepEqual compares it.
		case reflect.Func, reflect.Interface, reflect.Map, reflect.Chan, reflect.Pointer:
			if !v.Field(i).IsNil() {
				bad = append(bad, tp.Field(i).Name)
			}
		}
	}
	return bad
}

// configSignature keys the equality class of a static config. Invariant, for
// any config unshareableFields accepts: a.Equal(b) == (sig(a) == sig(b)). The
// signature decides which tests share a run, Equal decides whether
// newTestModule reuses the module; if they disagree, grouping stops paying off.
func configSignature(opts testOpts) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%#v", opts)))
	return hex.EncodeToString(sum[:6])
}

var defaultSignature = configSignature(testOpts{})

// resolveStaticOpts decides which static config a newTestModule call runs with:
// the call site's withStaticOpts when it supplied one, otherwise the config
// declared for this test, otherwise the default config. It is called after
// newTestModule has applied fopts, so forceReload is already set.
func resolveStaticOpts(t testing.TB, opts *tmOpts) {
	decl, declared := lookupDeclaredConfig(t.Name())

	switch {
	case opts.staticOptsSet && declared && !decl.inlineConfig:
		t.Fatalf("%s passes withStaticOpts but is also declared at %s: drop one of "+
			"the two, or switch the declaration to declareInlineConfig if the test "+
			"supplies its own config at the call site", t.Name(), decl.declaredAt)

	case (opts.staticOptsSet || opts.forceReload) && !declared:
		t.Fatalf("%s builds a module of its own -- it passes withStaticOpts or "+
			"withForceReload -- but declares nothing, so it would do it inside the "+
			"default group. If every field of that config is a compile-time value, "+
			"move it to declare(%s, testOpts{...}) so the test shares one module "+
			"with the others using the same config; if any of it is only knowable "+
			"at run time, or the test forces a reload, add declareInlineConfig(%s)",
			t.Name(), t.Name(), t.Name())

	case declared && decl.inlineConfig && !opts.staticOptsSet && !opts.forceReload:
		t.Fatalf("%s is declared at %s as supplying its own config, but this "+
			"newTestModule call passes neither withStaticOpts nor withForceReload: "+
			"supply one, or drop the declaration so the test joins the default run",
			t.Name(), decl.declaredAt)

	case declared && !decl.inlineConfig:
		opts.staticOpts = decl.opts
	}
}
