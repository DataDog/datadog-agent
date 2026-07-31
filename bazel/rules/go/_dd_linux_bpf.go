// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// This file extends the dd_agent_go_test Gazelle extension with linux_bpf
// handling. Packages whose Go sources are gated behind //go:build linux_bpf
// can't compile in the default configuration, so:
//
//   - their go_library gets tags=["manual"], keeping it out of //... wildcard
//     builds (it would otherwise build empty and mislead consumers);
//   - the go_binary / go_test that compile those sources get the linux_bpf
//     gotag so rules_go's build transition turns the tag on, plus
//     target_compatible_with = ["@platforms//os:linux"] so //... builds skip
//     them off-linux instead of failing (they're not manual, unlike the lib).
//
// The behaviour is opt-in per subtree via "# gazelle:dd_linux_bpf on" and
// inheritable, mirroring dd_agent_go_test.

package dd_agent_go_test

import (
	"go/build/constraint"
	"path/filepath"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

const (
	linuxBPFExtName = "dd_linux_bpf"
	linuxBPFTag     = "linux_bpf"
	manualTag       = "manual"
	linuxPlatform   = "@platforms//os:linux"
)

type ddLinuxBPFConfig struct {
	enabled bool
}

// configureLinuxBPF reads the # gazelle:dd_linux_bpf directive. Like
// dd_agent_go_test it is opt-in (default off) and inheritable: the value seeds
// from the parent config Gazelle cloned into c.Exts before descending, so a
// whole subtree is toggled from one BUILD file.
func configureLinuxBPF(c *config.Config, f *rule.File) {
	cfg := ddLinuxBPFConfig{enabled: false}
	if prev, ok := c.Exts[linuxBPFExtName].(ddLinuxBPFConfig); ok {
		cfg = prev
	}
	if f != nil {
		for _, d := range f.Directives {
			if d.Key == linuxBPFExtName {
				cfg.enabled = d.Value != "off"
			}
		}
	}
	c.Exts[linuxBPFExtName] = cfg
}

func linuxBPFEnabled(c *config.Config) bool {
	cfg, ok := c.Exts[linuxBPFExtName].(ddLinuxBPFConfig)
	return ok && cfg.enabled
}

// requiresLinuxBPF reports whether a //go:build expression compiles only when
// linux_bpf is set: satisfiable with the tag, never satisfiable without it.
// This treats "//go:build linux_bpf" (and e.g. "linux_bpf && linux") as
// requiring the tag while ignoring "//go:build !linux_bpf". "test" is always
// assumed true in both checks, since these expressions only ever gate
// _test.go files. Platform/arch tokens are free variables and go1.N tokens
// resolve against the toolchain, both handled by canSatisfy (shared with the
// flavor logic).
func requiresLinuxBPF(e constraint.Expr) bool {
	return canSatisfy(e, map[string]bool{linuxBPFTag: true, "test": true}) &&
		!canSatisfy(e, map[string]bool{"test": true})
}

// srcsRequireLinuxBPF reports whether any of srcs carries a //go:build header
// that requires linux_bpf. Files with no build constraint (or that we can't
// read) don't force the tag on their own.
func srcsRequireLinuxBPF(srcs []string, pkgDir string) bool {
	for _, s := range srcs {
		path := s
		if !filepath.IsAbs(path) {
			path = filepath.Join(pkgDir, s)
		}
		e, hasConstraint, err := readBuildConstraint(path)
		if err != nil || !hasConstraint {
			continue
		}
		if requiresLinuxBPF(e) {
			return true
		}
	}
	return false
}

// applyLinuxBPF post-processes the generated rules of a linux_bpf-gated package.
// It runs after test replacement, so dd_agent_go_test rules (kind != go_test)
// are intentionally left alone: linux_bpf is not a flavor, and applicableFlavors
// already drops linux_bpf-only tests from the flavored path.
//
// Because tags/gotags/target_compatible_with are not mergeable for the Go kinds,
// Gazelle's merger keeps an existing occurrence of such an attr verbatim (see
// rule.MergeList). Mutating only the freshly generated rule would therefore
// never retrofit a package that already has e.g. gotags=["test"]. So we add the
// value to both the generated rule and, when present, the matching existing rule
// pulled from args.File; addStringToListIfMissing keeps this idempotent.
func (l *lang) applyLinuxBPF(result language.GenerateResult, args language.GenerateArgs) language.GenerateResult {
	libRequires := make(map[string]bool)
	for _, r := range result.Gen {
		if r.Kind() == "go_library" {
			libRequires[r.Name()] = srcsRequireLinuxBPF(r.AttrStrings("srcs"), args.Dir)
		}
	}

	embedsRequiringLib := func(r *rule.Rule) bool {
		for _, e := range r.AttrStrings("embed") {
			if libRequires[strings.TrimPrefix(e, ":")] {
				return true
			}
		}
		return false
	}

	addValue := func(gen *rule.Rule, attr, value string) {
		addStringToListIfMissing(gen, attr, value)
		if ex, ok := findRule(args.File, gen.Kind(), gen.Name()); ok {
			addStringToListIfMissing(ex, attr, value)
		}
	}

	for _, r := range result.Gen {
		switch r.Kind() {
		case "go_library":
			if libRequires[r.Name()] {
				addValue(r, "tags", manualTag)
			}
		case "go_binary":
			if embedsRequiringLib(r) {
				addValue(r, "gotags", linuxBPFTag)
				addValue(r, "target_compatible_with", linuxPlatform)
			}
		case "go_test":
			if embedsRequiringLib(r) || srcsRequireLinuxBPF(r.AttrStrings("srcs"), args.Dir) {
				addValue(r, "gotags", linuxBPFTag)
				addValue(r, "target_compatible_with", linuxPlatform)
			}
		}
	}
	return result
}
