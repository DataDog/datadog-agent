// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package write_pb_go

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/language/proto"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

const name = "write_pb_go"

type protoEntry struct {
	relDir     string
	protoFiles []string
}

type lang struct {
	language.BaseLang
	importToProto map[string]protoEntry
}

//goland:noinspection GoUnusedExportedFunction
func NewLanguage() language.Language {
	return &lang{importToProto: make(map[string]protoEntry)}
}

func (*lang) Name() string {
	return name
}

func (*lang) Kinds() map[string]rule.KindInfo {
	return map[string]rule.KindInfo{
		name: {
			NonEmptyAttrs:  map[string]bool{"srcs": true},
			MergeableAttrs: map[string]bool{"srcs": true},
		},
	}
}

func (*lang) KnownDirectives() []string {
	return []string{name}
}

func (*lang) Loads() []rule.LoadInfo {
	return []rule.LoadInfo{{
		Name:    fmt.Sprintf("//bazel/rules/%s:defs.bzl", name),
		Symbols: []string{name},
	}}
}

func (l *lang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
	if args.File != nil {
		for _, d := range args.File.Directives {
			if d.Key == name && d.Value == "off" {
				return language.GenerateResult{}
			}
		}
	}

	// Accumulate go_package -> proto entry from proto_library rules in this
	// directory. The proto extension sets PackageKey on every proto_library
	// it generates; directories are visited alphabetically, so this map is
	// always populated before the corresponding Go package is visited.
	for _, r := range args.OtherGen {
		if r.Kind() != "proto_library" {
			continue
		}
		pkg, ok := r.PrivateAttr(proto.PackageKey).(proto.Package)
		if !ok {
			continue
		}
		goPackage, ok := pkg.Options["go_package"]
		if !ok {
			continue
		}
		goPackage, _, _ = strings.Cut(goPackage, ";")
		var protoFiles []string
		for f := range pkg.Files {
			protoFiles = append(protoFiles, f)
		}
		sort.Strings(protoFiles)
		l.importToProto[goPackage] = protoEntry{relDir: args.Rel, protoFiles: protoFiles}
	}

	srcs := map[string][]string{}
	hasGo := false
	for _, f := range args.RegularFiles {
		if strings.HasSuffix(f, ".go") {
			hasGo = true
		} else if stem, ok := strings.CutSuffix(f, ".proto"); ok {
			// Canonical: proto and Go files coexist in the current directory.
			label := fmt.Sprintf(":%s_go_proto", path.Base(args.Rel))
			srcs[label] = append(srcs[label], fmt.Sprintf("%s.pb.go", stem))
		}
	}
	if !hasGo {
		return language.GenerateResult{}
	}

	// Foreign: proto files live in a different directory; discover via go_package.
	// go_package may be a relative path (e.g. "pkg/proto/pbgo/foo") or a fully
	// qualified importpath; match either against the go_library importpath.
	if importPath := goLibraryImportPath(args.File); importPath != "" {
		for goPackage, entry := range l.importToProto {
			if entry.relDir == args.Rel {
				continue
			}
			if importPath != goPackage && !strings.HasSuffix(importPath, fmt.Sprintf("/%s", goPackage)) {
				continue
			}
			label := fmt.Sprintf("//%s:%s_go_proto", entry.relDir, path.Base(entry.relDir))
			for _, f := range entry.protoFiles {
				if stem, ok := strings.CutSuffix(f, ".proto"); ok {
					srcs[label] = append(srcs[label], fmt.Sprintf("%s.pb.go", stem))
				}
			}
			break
		}
	}

	if len(srcs) == 0 {
		return language.GenerateResult{}
	}

	r := rule.NewRule(name, name)
	r.SetAttr("srcs", srcs)
	return language.GenerateResult{
		Gen:     []*rule.Rule{r},
		Imports: []interface{}{nil},
	}
}

func goLibraryImportPath(f *rule.File) string {
	if f == nil {
		return ""
	}
	for _, r := range f.Rules {
		if r.Kind() == "go_library" {
			return r.AttrString("importpath")
		}
	}
	return ""
}
