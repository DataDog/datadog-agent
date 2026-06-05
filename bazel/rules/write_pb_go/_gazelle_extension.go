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

	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/language/proto"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

const name = "write_pb_go"

var compilerRe = regexp.MustCompile(`:go_([^_]+)`)

type goProtoLibrary struct {
	label         label.Label
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
	// TODO(regis): find an approach that doesn't rely on directory traversal order
	for i, libs := range goProtoLibraries(args) {
		l.goProtoLibraries[i] = append(l.goProtoLibraries[i], libs...)
	}
	return generateResult(args, l.goProtoLibraries)
}

// goProtoLibraries indexes go_proto_library entries by fully-qualified importpath; relative
// importpaths are prefixed with GoPrefix to normalize them.
func goProtoLibraries(args language.GenerateArgs) map[string][]goProtoLibrary {
	protoLibrarySrcs := protoLibrarySrcs(args)
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
		protoLabels := r.AttrStrings("protos")   // plural: `gazelle:proto file`
		if s := r.AttrString("proto"); s != "" { // singular: `gazelle:proto default|package`
			protoLabels = append(protoLabels, s)
		}
		var protoInfos []proto.FileInfo
		for _, s := range protoLabels {
			protoLabel, err := label.Parse(s)
			if err != nil {
				continue
			}
			if infos, ok := protoLibrarySrcs[protoLabel.Name]; ok {
				protoInfos = append(protoInfos, infos...)
			}
		}
		if len(protoInfos) > 0 {
			result[importPath] = append(result[importPath], goProtoLibrary{
				label:         label.New("", args.Rel, r.Name()),
				generatedSrcs: generatedSrcs(r, protoInfos),
			})
		}
	}
	return result
}

// generateResult emits a write_pb_go rule when the current directory holds a go_library whose
// importpath matches an accumulated go_proto_library, or deletes a stale one otherwise.
// Any `gazelle:write_pb_go <go_proto_library> <override>` directive substitutes the former label with the latter.
func generateResult(args language.GenerateArgs, goProtoLibraries map[string][]goProtoLibrary) language.GenerateResult {
	if importPath := goLibraryImportPath(args.File); importPath != "" {
		srcs := map[string][]string{}
		for _, lib := range goProtoLibraries[importPath] {
			srcs[override(args, lib.label.Rel("", args.Rel).String())] = lib.generatedSrcs
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

// protoLibrarySrcs indexes proto_library srcs by rule name.
func protoLibrarySrcs(args language.GenerateArgs) map[string][]proto.FileInfo {
	result := map[string][]proto.FileInfo{}
	for _, r := range args.OtherGen {
		if r.Kind() != "proto_library" {
			continue
		}
		pkg, ok := r.PrivateAttr(proto.PackageKey).(proto.Package)
		if !ok {
			continue
		}
		var protoInfos []proto.FileInfo
		for _, src := range r.AttrStrings("srcs") {
			protoInfos = append(protoInfos, pkg.Files[src])
		}
		result[r.Name()] = protoInfos
	}
	return result
}

// generatedSrcs infers .pb.go filenames from the compiler labels: :go_proto -> .pb.go,
// :go_{name}_* -> _{name}.pb.go; defaults to [.pb.go].
// For serviceless protos, omit dummy _grpc.pb.go backfilled by rules_go
// (https://github.com/bazel-contrib/rules_go/pull/1394) that would only pollute the source tree.
func generatedSrcs(r *rule.Rule, protoInfos []proto.FileInfo) []string {
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
	for _, protoInfo := range protoInfos {
		stem := strings.TrimSuffix(protoInfo.Name, ".proto")
		for _, suffix := range suffixes {
			if protoInfo.HasServices || suffix != "_grpc.pb.go" {
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

// override returns any label overridden by `gazelle:write_pb_go <orig> <overridden>` or orig.
func override(args language.GenerateArgs, label string) string {
	for _, d := range args.File.Directives {
		if d.Key != name {
			continue
		}
		if parts := strings.Fields(d.Value); len(parts) == 2 && parts[0] == label {
			return parts[1]
		}
	}
	return label
}
