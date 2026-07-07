// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package main generates docs/CONFIG.md, a reference of every Pulumi config
// key read by the e2e framework (e.g. "ddinfra:gcp/defaultPrivateKeyPath").
//
// Config keys are declared as string constants scattered across
// common/config/environment.go, resources/*/environment.go, and a handful of
// scenario/entrypoint files (run/main.go, scenarios/aws/gensim-eks/run.go,
// ...), each read through a *sdkconfig.Config value whose namespace
// ("ddinfra", "aws", "gensim", ...) is set once via sdkconfig.New(ctx, <ns>)
// or config.New(ctx, <ns>) — either on a struct field or a local variable.
// This tool parses the source (not compiled code) to recover, for every key,
// which field/variable read it and which namespace that owner belongs to.
//
// Known gap: scenarios/aws/microVMs/config/config.go opens its own "microvm"
// namespace, but its keys are read from a different package
// (scenarios/aws/microVMs/microvms) via package-qualified consts
// (config.DDMicroVMLocalWorkingDirectory). Const and namespace resolution
// here are scoped to a single file, so cross-package references like that
// are not picked up.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const outputPath = "docs/CONFIG.md"

// configKey is one resolved Pulumi config key.
type configKey struct {
	Namespace string // e.g. "ddinfra"
	Value     string // e.g. "gcp/defaultPrivateKeyPath"
	Const     string // e.g. "DDInfraDefaultPrivateKeyPath"; empty for inline literal keys
	Doc       string
	File      string
	Line      int
}

// configCallSite pairs a config key with the struct field or local variable
// it was read through, e.g. {Field: "InfraConfig", ConstIdent: "DDInfraOSDescriptor"}
// or {Field: "cfg", Literal: "episodes"} for an inline string key.
type configCallSite struct {
	Field      string
	ConstIdent string
	Literal    string
	Pos        token.Pos // valid only when Literal != ""
}

func main() {
	check := flag.Bool("check", false, "exit 1 if docs/CONFIG.md is out of date instead of writing it")
	flag.Parse()

	root, err := frameworkRoot()
	if err != nil {
		fatal(err)
	}

	files, err := discoverFiles(root)
	if err != nil {
		fatal(err)
	}

	// Each provider redeclares identifiers like DDInfraDefaultPrivateKeyPath
	// with a different value (aws/, gcp/, az/, ...), so const lookups and
	// call-site matching must stay scoped to a single file's *ast.File —
	// merging them into one global map-by-identifier would let later files
	// silently clobber earlier ones. Only namespaceByField (keyed by struct
	// field name, e.g. "InfraConfig") and the final resolved keys are safe
	// to accumulate across files, since those never collide.
	fset := token.NewFileSet()
	namespaceByField := make(map[string]string)
	var keys []configKey
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			fatal(fmt.Errorf("parsing %s: %w", path, err))
		}

		constValues, constPos := collectConstDecls(fset, file)
		maps.Copy(namespaceByField, resolveNamespacesByField(constValues, file))

		// Dedup stays scoped to this one file (like constPos), but is shared
		// across all of the file's functions below so the same key read from
		// two different functions is still only documented once.
		seen := make(map[string]bool)

		// Local variable names like "cfg" or "rootConfig" are reused across
		// unrelated functions with different meanings (even within one
		// file), so resolution and call-site matching for them is scoped to
		// each function body individually rather than merged into the
		// persistent, cross-file namespaceByField map.
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}

			localNS := resolveLocalVarNamespaces(constValues, fn.Body)
			fnNS := namespaceByField
			if len(localNS) > 0 {
				fnNS = make(map[string]string, len(namespaceByField)+len(localNS))
				maps.Copy(fnNS, namespaceByField)
				maps.Copy(fnNS, localNS)
			}

			callSites := findConfigCallSites(fn.Body)
			keys = append(keys, buildConfigKeys(fset, callSites, constPos, fnNS, root, seen)...)
		}
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Namespace != keys[j].Namespace {
			return keys[i].Namespace < keys[j].Namespace
		}
		if keys[i].Value != keys[j].Value {
			return keys[i].Value < keys[j].Value
		}
		// Namespace+Value ties happen when two consts resolve to the same
		// key (e.g. ddagent:fullImagePath); break on Const for a fully
		// deterministic order regardless of scan order.
		return keys[i].Const < keys[j].Const
	})

	doc := renderMarkdown(keys)

	if *check {
		existing, err := os.ReadFile(filepath.Join(root, outputPath))
		if err != nil {
			fatal(fmt.Errorf("reading %s: %w (run `dda inv e2e.configdoc` to generate it)", outputPath, err))
		}
		if string(existing) != doc {
			fmt.Fprintf(os.Stderr, "%s is out of date; run `dda inv e2e.configdoc` and commit the result\n", outputPath)
			os.Exit(1)
		}
		return
	}

	if err := os.WriteFile(filepath.Join(root, outputPath), []byte(doc), 0o644); err != nil {
		fatal(err)
	}
}

// frameworkRoot returns the directory containing this module's go.mod
// (test/e2e-framework), regardless of the working directory this binary is
// invoked from.
func frameworkRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

// discoverFiles returns every file that declares or reads Pulumi config
// keys: common/config/environment.go, one per cloud/local provider under
// resources/, plus any other file discovered by findConfigEntrypoints.
func discoverFiles(root string) ([]string, error) {
	var files []string
	seen := make(map[string]bool)
	add := func(path string) {
		if !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
	}

	if _, err := os.Stat(filepath.Join(root, "common/config/environment.go")); err == nil {
		add(filepath.Join(root, "common/config/environment.go"))
	}
	matches, err := filepath.Glob(filepath.Join(root, "resources/*/environment.go"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	for _, m := range matches {
		add(m)
	}

	extra, err := findConfigEntrypoints(root)
	if err != nil {
		return nil, err
	}
	for _, e := range extra {
		add(e)
	}

	return files, nil
}

// findConfigEntrypoints returns every non-test .go file under root whose
// source opens a Pulumi config namespace via `.New(ctx`, e.g.
// run/main.go's `config.New(ctx, "")` or gensim-eks/run.go's
// `config.New(ctx, "gensim")`. These live outside common/config and
// resources/*/environment.go, so discoverFiles can't find them by path
// convention alone.
func findConfigEntrypoints(root string) ([]string, error) {
	selfDir := filepath.Join(root, "cmd", "configdoc")
	var matches []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if strings.HasPrefix(path, selfDir+string(filepath.Separator)) {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(contents, []byte(".New(ctx")) {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

// collectConstDecls indexes every string-valued const declaration in file by
// identifier, along with its source position and any attached comment (used
// later as the key's documentation). Scoped to a single file: identifiers
// like DDInfraDefaultPrivateKeyPath are redeclared with different values in
// every provider's environment.go, so merging across files would collide.
func collectConstDecls(fset *token.FileSet, file *ast.File) (values map[string]string, positions map[string]configKey) {
	values = make(map[string]string)
	positions = make(map[string]configKey)

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 {
				continue
			}
			lit, ok := vs.Values[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				continue
			}
			name := vs.Names[0].Name
			values[name] = value

			var docParts []string
			if vs.Doc != nil {
				docParts = append(docParts, strings.TrimSpace(vs.Doc.Text()))
			}
			if vs.Comment != nil {
				docParts = append(docParts, strings.TrimSpace(vs.Comment.Text()))
			}

			pos := fset.Position(vs.Pos())
			positions[name] = configKey{
				Const: name,
				Value: value,
				Doc:   strings.TrimSpace(strings.Join(docParts, " ")),
				File:  pos.Filename,
				Line:  pos.Line,
			}
		}
	}
	return values, positions
}

// resolveNamespacesByField finds every `field: sdkconfig.New(ctx, <ns>)`
// assignment in file (in CommonEnvironment and any provider-specific
// environment, e.g. resources/aws's separate "aws" namespace) and returns
// the field name -> namespace value mapping, e.g. {"InfraConfig": "ddinfra"}.
func resolveNamespacesByField(constValues map[string]string, file *ast.File) map[string]string {
	result := make(map[string]string)
	ast.Inspect(file, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		field, ok := kv.Key.(*ast.Ident)
		if !ok {
			return true
		}
		call, ok := kv.Value.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "New" || len(call.Args) != 2 {
			return true
		}
		if ns, ok := resolveStringArg(call.Args[1], constValues); ok {
			result[field.Name] = ns
		}
		return true
	})
	return result
}

// resolveLocalVarNamespaces finds every `ident := config.New(ctx, <ns>)` (or
// sdkconfig.New) short variable declaration in file — the shape used by
// entrypoints like run/main.go's `rootConfig := config.New(ctx, "")` or
// gensim-eks/run.go's `cfg := config.New(ctx, "gensim")` — and returns the
// variable name -> namespace mapping. Deliberately not merged into the
// persistent namespaceByField map: see the caller.
func resolveLocalVarNamespaces(constValues map[string]string, scope ast.Node) map[string]string {
	result := make(map[string]string)
	ast.Inspect(scope, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || assign.Tok != token.DEFINE || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		ident, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "New" || len(call.Args) != 2 {
			return true
		}
		if ns, ok := resolveStringArg(call.Args[1], constValues); ok {
			result[ident.Name] = ns
		}
		return true
	})
	return result
}

// resolveStringArg returns the literal string value of expr and whether it
// resolved, whether expr is a raw string literal or an identifier
// referencing a known const. A resolved empty string (e.g. the root
// namespace `config.New(ctx, "")`) is a valid result, distinct from "not
// resolved".
func resolveStringArg(expr ast.Expr, constValues map[string]string) (string, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			if s, err := strconv.Unquote(e.Value); err == nil {
				return s, true
			}
		}
	case *ast.Ident:
		if v, ok := constValues[e.Name]; ok {
			return v, true
		}
	}
	return "", false
}

// findConfigCallSites scans every function body in file for expressions that
// read a Pulumi config value, and returns one configCallSite per (field,
// key) pair it recognizes. It must handle two call shapes seen in this
// codebase:
//
//   - Direct:  e.InfraConfig.Get(DDInfraOSDescriptor)
//     e.AgentConfig.RequireSecret(DDAgentAPIKeyParamName)
//     cfg.Get("episodes")
//     (method one of Get, Require, RequireSecret, Try, TryBool, TryInt, TryObject;
//     receiver is a SelectorExpr like `e.InfraConfig` or a plain local
//     variable like `cfg` — the field/owner is its .Sel.Name or .Name;
//     the sole argument is either an *ast.Ident naming a const or a raw
//     string literal)
//
//   - Wrapper: e.GetBoolWithDefault(e.InfraConfig, DDInfraOSImageIDUseLatest, false)
//     (method name ends in "WithDefault"; first argument is the field
//     selector, second argument is the const identifier or string literal)
//
// Anything that doesn't match either shape (e.g. .Getenv, calls on a
// variable that isn't a `<recv>.<Field>` selector or plain identifier)
// should be skipped, not guessed at.
func findConfigCallSites(scope ast.Node) []configCallSite {
	directMethods := map[string]bool{
		"Get": true, "Require": true, "RequireSecret": true,
		"Try": true, "TryBool": true, "TryInt": true, "TryObject": true,
	}

	var sites []configCallSite
	ast.Inspect(scope, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			// Direct reads take exactly one argument (the key). Reject
			// method-name matches with more args so a package-level
			// `config.Get(ctx, "key")` free function (2 args) doesn't get
			// misread as owner=pulumiConfig, key=ctx.
			if directMethods[sel.Sel.Name] && len(call.Args) == 1 {
				if field := fieldName(sel.X); field != "" {
					if site, ok := callSiteFromArg(field, call.Args[0]); ok {
						sites = append(sites, site)
					}
				}
				return true
			}
			if strings.HasSuffix(sel.Sel.Name, "WithDefault") && len(call.Args) >= 2 {
				if field := fieldName(call.Args[0]); field != "" {
					if site, ok := callSiteFromArg(field, call.Args[1]); ok {
						sites = append(sites, site)
					}
				}
			}
		}
		return true
	})
	return sites
}

// callSiteFromArg builds a configCallSite for owner reading key arg, which
// is either an *ast.Ident naming a const or a raw string literal.
func callSiteFromArg(owner string, arg ast.Expr) (configCallSite, bool) {
	switch a := arg.(type) {
	case *ast.Ident:
		return configCallSite{Field: owner, ConstIdent: a.Name}, true
	case *ast.BasicLit:
		if a.Kind == token.STRING {
			if s, err := strconv.Unquote(a.Value); err == nil {
				return configCallSite{Field: owner, Literal: s, Pos: a.Pos()}, true
			}
		}
	}
	return configCallSite{}, false
}

// fieldName returns the field or variable name of expr: the field of a
// `<recv>.<Field>` selector (e.g. "InfraConfig" for `e.InfraConfig`), or the
// name of a plain identifier (e.g. "cfg" for a local variable). Returns ""
// if expr isn't one of those shapes.
func fieldName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		return e.Sel.Name
	case *ast.Ident:
		return e.Name
	}
	return ""
}

// buildConfigKeys joins call sites, const declarations, and namespace
// resolution into the final sorted list of documented keys. seen is shared
// across every call for a single file, so a key read from two different
// owners or functions is still deduplicated by (owner, key), not just key.
func buildConfigKeys(fset *token.FileSet, callSites []configCallSite, positions map[string]configKey, namespaceByField map[string]string, root string, seen map[string]bool) []configKey {
	var keys []configKey
	for _, cs := range callSites {
		dedupKey := cs.Field + "|" + cs.ConstIdent
		if cs.ConstIdent == "" {
			dedupKey = cs.Field + "|literal:" + cs.Literal
		}
		if seen[dedupKey] {
			continue
		}
		var base configKey
		if cs.ConstIdent != "" {
			// An *ast.Ident argument that isn't a known const in this file is
			// not a real config key reference — e.g. `paramName` in the
			// generic `func (e *CommonEnvironment) GetBoolWithDefault(config
			// *sdkconfig.Config, paramName string, ...)` helper definitions,
			// where "config" and "paramName" are just parameter names, not an
			// owner/key pair. Skip silently rather than warn.
			b, ok := positions[cs.ConstIdent]
			if !ok {
				continue
			}
			base = b
		} else {
			pos := fset.Position(cs.Pos)
			base = configKey{Value: cs.Literal, File: pos.Filename, Line: pos.Line}
		}

		ns, ok := namespaceByField[cs.Field]
		if !ok {
			fmt.Fprintf(os.Stderr, "configdoc: warning: no namespace resolved for field %q; skipping\n", cs.Field)
			continue
		}

		seen[dedupKey] = true
		base.Namespace = ns
		if rel, err := filepath.Rel(root, base.File); err == nil {
			base.File = rel
		}
		keys = append(keys, base)
	}
	return keys
}

func renderMarkdown(keys []configKey) string {
	var b bytes.Buffer
	b.WriteString("<!-- Generated by `dda inv e2e.configdoc`. Do not edit by hand. -->\n\n")
	b.WriteString("# Pulumi config key reference\n\n")
	b.WriteString("Every Pulumi config key this scanner can resolve, grouped by namespace ")
	b.WriteString("(see the \"Known gap\" note in cmd/configdoc/main.go for what it can't see). ")
	b.WriteString("Set one with `pulumi config set <namespace>:<key> <value>` or `-c <namespace>:<key>=<value>`.\n")

	var currentNamespace string
	firstSection := true
	for _, k := range keys {
		if firstSection || k.Namespace != currentNamespace {
			firstSection = false
			currentNamespace = k.Namespace
			heading := currentNamespace
			if heading == "" {
				heading = "(root)"
			}
			fmt.Fprintf(&b, "\n## %s\n\n", heading)
			b.WriteString("| Key | Const | Defined in | Doc |\n")
			b.WriteString("|---|---|---|---|\n")
		}
		doc := strings.ReplaceAll(k.Doc, "\n", " ")
		constCol := ""
		if k.Const != "" {
			constCol = fmt.Sprintf("`%s`", k.Const)
		}
		key := currentNamespace + ":" + k.Value
		if currentNamespace == "" {
			key = k.Value
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s:%d | %s |\n", key, constCol, k.File, k.Line, doc)
	}
	return b.String()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "configdoc:", err)
	os.Exit(1)
}
