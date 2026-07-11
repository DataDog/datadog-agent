// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package dd_agent_go_test

import (
	"go/build/constraint"
	"os"
	"path/filepath"
	"testing"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

func mustParse(t *testing.T, line string) constraint.Expr {
	t.Helper()
	e, err := constraint.Parse(line)
	if err != nil {
		t.Fatalf("parse %q: %v", line, err)
	}
	return e
}

func TestRequiresLinuxBPF(t *testing.T) {
	for _, tc := range []struct {
		line string
		want bool
	}{
		{"//go:build linux_bpf", true},
		{"//go:build linux_bpf && linux", true},
		{"//go:build linux && linux_bpf", true},
		{"//go:build !linux_bpf", false},
		{"//go:build linux", false},
		{"//go:build windows", false},
		{"//go:build linux_bpf || linux", false}, // satisfiable without the tag
	} {
		t.Run(tc.line, func(t *testing.T) {
			if got := requiresLinuxBPF(mustParse(t, tc.line)); got != tc.want {
				t.Errorf("requiresLinuxBPF(%q) = %v, want %v", tc.line, got, tc.want)
			}
		})
	}
}

// writeGoFile writes a minimal Go file with an optional //go:build header and
// returns its base name.
func writeGoFile(t *testing.T, dir, name, header string) string {
	t.Helper()
	body := ""
	if header != "" {
		body = header + "\n\n"
	}
	body += "package x\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return name
}

func TestSrcsRequireLinuxBPF(t *testing.T) {
	dir := t.TempDir()
	bpf := writeGoFile(t, dir, "bpf.go", "//go:build linux_bpf")
	plain := writeGoFile(t, dir, "plain.go", "")
	linux := writeGoFile(t, dir, "linux.go", "//go:build linux")

	for _, tc := range []struct {
		name string
		srcs []string
		want bool
	}{
		{"gated file", []string{bpf}, true},
		{"unconstrained file", []string{plain}, false},
		{"platform-only file", []string{linux}, false},
		{"mix with one gated", []string{plain, bpf}, true},
		{"missing file is ignored", []string{"does_not_exist.go"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := srcsRequireLinuxBPF(tc.srcs, dir); got != tc.want {
				t.Errorf("srcsRequireLinuxBPF(%v) = %v, want %v", tc.srcs, got, tc.want)
			}
		})
	}
}

func newLinuxBPFLang() *lang { return NewLanguage().(*lang) }

// applyResult wraps rules in a GenerateResult and runs applyLinuxBPF against the
// given dir and existing file.
func applyResult(dir string, file *rule.File, rules ...*rule.Rule) language.GenerateResult {
	res := language.GenerateResult{Gen: rules, Imports: make([]interface{}, len(rules))}
	return newLinuxBPFLang().applyLinuxBPF(res, language.GenerateArgs{Dir: dir, File: file})
}

func TestApplyLinuxBPF_TagsAndGotags(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "bpf.go", "//go:build linux_bpf")
	writeGoFile(t, dir, "plain.go", "")

	lib := rule.NewRule("go_library", "mylib")
	lib.SetAttr("srcs", []string{"bpf.go"})
	plainLib := rule.NewRule("go_library", "plainlib")
	plainLib.SetAttr("srcs", []string{"plain.go"})
	bin := rule.NewRule("go_binary", "mybin")
	bin.SetAttr("embed", []string{":mylib"})
	test := rule.NewRule("go_test", "mylib_test")
	test.SetAttr("embed", []string{":mylib"})
	plainTest := rule.NewRule("go_test", "plainlib_test")
	plainTest.SetAttr("embed", []string{":plainlib"})

	applyResult(dir, nil, lib, plainLib, bin, test, plainTest)

	if got := lib.AttrStrings("tags"); !stringSlicesEqual(got, []string{manualTag}) {
		t.Errorf("mylib tags = %v, want [manual]", got)
	}
	if got := plainLib.AttrStrings("tags"); len(got) != 0 {
		t.Errorf("plainlib tags = %v, want none", got)
	}
	if got := bin.AttrStrings("gotags"); !stringSlicesEqual(got, []string{linuxBPFTag}) {
		t.Errorf("mybin gotags = %v, want [linux_bpf]", got)
	}
	if got := bin.AttrStrings("target_compatible_with"); !stringSlicesEqual(got, []string{linuxPlatform}) {
		t.Errorf("mybin target_compatible_with = %v, want [%s]", got, linuxPlatform)
	}
	if got := test.AttrStrings("gotags"); !stringSlicesEqual(got, []string{linuxBPFTag}) {
		t.Errorf("mylib_test gotags = %v, want [linux_bpf]", got)
	}
	if got := test.AttrStrings("target_compatible_with"); !stringSlicesEqual(got, []string{linuxPlatform}) {
		t.Errorf("mylib_test target_compatible_with = %v, want [%s]", got, linuxPlatform)
	}
	if got := plainTest.AttrStrings("gotags"); len(got) != 0 {
		t.Errorf("plainlib_test gotags = %v, want none", got)
	}
	if got := plainTest.AttrStrings("target_compatible_with"); len(got) != 0 {
		t.Errorf("plainlib_test target_compatible_with = %v, want none", got)
	}
}

// TestApplyLinuxBPF_TestSrcsGated covers a go_test whose own srcs are gated even
// though it embeds nothing that requires the tag.
func TestApplyLinuxBPF_TestSrcsGated(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "x_test.go", "//go:build linux_bpf")

	test := rule.NewRule("go_test", "x_test")
	test.SetAttr("srcs", []string{"x_test.go"})

	applyResult(dir, nil, test)

	if got := test.AttrStrings("gotags"); !stringSlicesEqual(got, []string{linuxBPFTag}) {
		t.Errorf("gotags = %v, want [linux_bpf]", got)
	}
	if got := test.AttrStrings("target_compatible_with"); !stringSlicesEqual(got, []string{linuxPlatform}) {
		t.Errorf("target_compatible_with = %v, want [%s]", got, linuxPlatform)
	}
}

// TestApplyLinuxBPF_RetrofitsExisting guards the brownfield case: an existing
// go_test already carrying gotags=["test"] must be updated in place (gotags is
// not mergeable, so the generated rule alone can't do it).
func TestApplyLinuxBPF_RetrofitsExisting(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "bpf.go", "//go:build linux_bpf")

	lib := rule.NewRule("go_library", "mylib")
	lib.SetAttr("srcs", []string{"bpf.go"})
	genTest := rule.NewRule("go_test", "mylib_test")
	genTest.SetAttr("embed", []string{":mylib"})
	genTest.SetAttr("gotags", []string{"test"})

	existingTest := rule.NewRule("go_test", "mylib_test")
	existingTest.SetAttr("gotags", []string{"test"})
	file := &rule.File{Rules: []*rule.Rule{existingTest}}

	applyResult(dir, file, lib, genTest)

	if got := existingTest.AttrStrings("gotags"); !stringSlicesEqual(got, []string{"test", linuxBPFTag}) {
		t.Errorf("existing gotags = %v, want [test linux_bpf]", got)
	}
	if got := existingTest.AttrStrings("target_compatible_with"); !stringSlicesEqual(got, []string{linuxPlatform}) {
		t.Errorf("existing target_compatible_with = %v, want [%s]", got, linuxPlatform)
	}
}

// TestApplyLinuxBPF_Idempotent ensures a second pass adds nothing.
func TestApplyLinuxBPF_Idempotent(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "bpf.go", "//go:build linux_bpf")

	lib := rule.NewRule("go_library", "mylib")
	lib.SetAttr("srcs", []string{"bpf.go"})

	applyResult(dir, nil, lib)
	applyResult(dir, nil, lib)

	if got := lib.AttrStrings("tags"); !stringSlicesEqual(got, []string{manualTag}) {
		t.Errorf("tags = %v, want [manual] (idempotent)", got)
	}
}

// TestApplyLinuxBPF_SkipsDdAgentGoTest verifies flavored tests are left to the
// flavor logic (linux_bpf is not a flavor).
func TestApplyLinuxBPF_SkipsDdAgentGoTest(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "bpf.go", "//go:build linux_bpf")

	lib := rule.NewRule("go_library", "mylib")
	lib.SetAttr("srcs", []string{"bpf.go"})
	flavored := rule.NewRule("dd_agent_go_test", "mylib_test")
	flavored.SetAttr("embed", []string{":mylib"})

	applyResult(dir, nil, lib, flavored)

	if got := flavored.AttrStrings("gotags"); len(got) != 0 {
		t.Errorf("dd_agent_go_test gotags = %v, want none", got)
	}
}

func TestConfigureLinuxBPF_DefaultDisabled(t *testing.T) {
	c := &config.Config{Exts: map[string]interface{}{}}
	newLinuxBPFLang().Configure(c, "some/pkg", nil)
	if linuxBPFEnabled(c) {
		t.Error("expected disabled by default (opt-in)")
	}
}

func TestConfigureLinuxBPF_DirectiveOnOff(t *testing.T) {
	c := &config.Config{Exts: map[string]interface{}{}}
	on := &rule.File{}
	on.Directives = []rule.Directive{{Key: linuxBPFExtName, Value: "on"}}
	newLinuxBPFLang().Configure(c, "some/pkg", on)
	if !linuxBPFEnabled(c) {
		t.Fatal("expected enabled after directive on")
	}

	off := &rule.File{}
	off.Directives = []rule.Directive{{Key: linuxBPFExtName, Value: "off"}}
	newLinuxBPFLang().Configure(c, "some/pkg/child", off)
	if linuxBPFEnabled(c) {
		t.Error("expected disabled after directive off")
	}
}

// TestConfigureLinuxBPF_Inherits verifies the directive is inheritable: a child
// with no directive keeps the parent's value cloned into c.Exts.
func TestConfigureLinuxBPF_Inherits(t *testing.T) {
	l := newLinuxBPFLang()
	c := &config.Config{Exts: map[string]interface{}{}}
	parent := &rule.File{}
	parent.Directives = []rule.Directive{{Key: linuxBPFExtName, Value: "on"}}
	l.Configure(c, "some/pkg", parent)
	l.Configure(c, "some/pkg/child", nil)
	if !linuxBPFEnabled(c) {
		t.Error("expected enabled inherited from parent")
	}
}

func TestKnownDirectivesIncludesLinuxBPF(t *testing.T) {
	dirs := newLinuxBPFLang().KnownDirectives()
	found := false
	for _, d := range dirs {
		if d == linuxBPFExtName {
			found = true
		}
	}
	if !found {
		t.Errorf("%q not in KnownDirectives: %v", linuxBPFExtName, dirs)
	}
}
