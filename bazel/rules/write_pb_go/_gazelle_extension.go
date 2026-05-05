// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package write_pb_go

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/language/proto"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

const name = "write_pb_go"

var compilerRe = regexp.MustCompile(`:go_([^_]+)`)

type goProtoLibrary struct {
	relDir        string
	relTarget     string
	generatedSrcs []string
}

type lang struct {
	language.BaseLang
	goProtoLibraries map[string][]goProtoLibrary
}

//goland:noinspection GoUnusedExportedFunction
func NewLanguage() language.Language {
	return &lang{goProtoLibraries: make(map[string][]goProtoLibrary)}
}

func (*lang) Name() string {
	return name
}

func (*lang) Kinds() map[string]rule.KindInfo {
	return map[string]rule.KindInfo{
		name: {
			MergeableAttrs: map[string]bool{"srcs": true},
			NonEmptyAttrs:  map[string]bool{"srcs": true},
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
	// GenerateRules is called once per directory; accumulate across visits so that proto dirs
	// (visited first) are available when Go dirs are processed.
	for i, libs := range goProtoLibraries(args, protoLibrarySrcs(args)) {
		l.goProtoLibraries[i] = append(l.goProtoLibraries[i], libs...)
	}
	return generateResult(args, l.goProtoLibraries)
}

// protoLibrarySrcs indexes proto_library srcs by rule name; OtherGen takes precedence over
// File.Rules so Gazelle-generated rules are preferred over potentially stale committed ones.
func protoLibrarySrcs(args language.GenerateArgs) map[string][]string {
	result := map[string][]string{}
	for _, r := range args.OtherGen {
		if r.Kind() == "proto_library" {
			result[r.Name()] = r.AttrStrings("srcs")
		}
	}
	if args.File != nil {
		for _, r := range args.File.Rules {
			if r.Kind() == "proto_library" {
				if _, ok := result[r.Name()]; !ok {
					result[r.Name()] = r.AttrStrings("srcs")
				}
			}
		}
	}
	return result
}

// goProtoLibraries indexes go_proto_library entries by fully-qualified importpath; relative
// importpaths are prefixed with GoPrefix to normalize them.
func goProtoLibraries(args language.GenerateArgs, protoLibrarySrcs map[string][]string) map[string][]goProtoLibrary {
	result := map[string][]goProtoLibrary{}
	for _, r := range args.OtherGen {
		if r.Kind() != "go_proto_library" {
			continue
		}
		importPath := r.AttrString("importpath")
		if importPath == "" {
			continue
		}
		if pc := proto.GetProtoConfig(args.Config); pc != nil && !strings.HasPrefix(importPath, pc.GoPrefix) {
			importPath = fmt.Sprintf("%s/%s", pc.GoPrefix, importPath)
		}
		if protos, ok := protoLibrarySrcs[r.AttrString("proto")[strings.LastIndex(r.AttrString("proto"), ":")+1:]]; ok {
			result[importPath] = append(result[importPath], goProtoLibrary{
				relDir:        args.Rel,
				relTarget:     fmt.Sprintf(":%s", r.Name()),
				generatedSrcs: generatedSrcs(r, protos),
			})
		}
	}
	return result
}

// generateResult emits a write_pb_go rule when the current directory holds a go_library whose
// importpath matches an accumulated go_proto_library, or deletes a stale one otherwise.
func generateResult(args language.GenerateArgs, goProtoLibraries map[string][]goProtoLibrary) language.GenerateResult {
	if importPath := goLibraryImportPath(args.File); importPath != "" {
		srcs := map[string][]string{}
		for _, lib := range goProtoLibraries[importPath] {
			var target string
			if lib.relDir == args.Rel {
				target = lib.relTarget // canonical case: .proto and .go coexist
			} else {
				target = fmt.Sprintf("//%s%s", lib.relDir, lib.relTarget) // foreign case
			}
			srcs[target] = lib.generatedSrcs
		}
		if len(srcs) > 0 {
			r := rule.NewRule(name, name)
			r.SetAttr("srcs", srcs)
			return language.GenerateResult{
				Gen:     []*rule.Rule{r},
				Imports: []interface{}{nil},
			}
		}
	}
	if args.File != nil {
		for _, r := range args.File.Rules {
			if r.Kind() == name {
				return language.GenerateResult{Empty: []*rule.Rule{rule.NewRule(name, name)}}
			}
		}
	}
	return language.GenerateResult{}
}

// generatedSrcs infers .pb.go filenames from the compiler labels: :go_proto -> .pb.go,
// :go_{name}_* -> _{name}.pb.go; defaults to [.pb.go].
func generatedSrcs(r *rule.Rule, protos []string) []string {
	var suffixes []string
	for _, compiler := range r.AttrStrings("compilers") {
		m := compilerRe.FindStringSubmatch(compiler)
		if m == nil {
			continue
		}
		if m[1] == "proto" {
			suffixes = append(suffixes, ".pb.go")
		} else {
			suffixes = append(suffixes, fmt.Sprintf("_%s.pb.go", m[1]))
		}
	}
	if len(suffixes) == 0 {
		suffixes = []string{".pb.go"}
	}
	var result []string
	for _, proto := range protos {
		if stem, ok := strings.CutSuffix(proto, ".proto"); ok {
			for _, suffix := range suffixes {
				result = append(result, stem+suffix)
			}
		}
	}
	sort.Strings(result)
	return result
}

// goLibraryImportPath returns the importpath of the go_library in f, or "".
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
