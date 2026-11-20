// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// kubernetes-generator regenerates the private action runner's typed Kubernetes actions.
package main

import (
	"bytes"
	"embed"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
	"unicode"

	"go.yaml.in/yaml/v3"
)

//go:embed config.yaml templates/*.tmpl
var assets embed.FS

type targetConfig struct {
	Version       string   `yaml:"version"`
	CopyrightYear int      `yaml:"copyrightYear"`
	Resources     []string `yaml:"resources"`
}

type action struct {
	Name, Package, Client string
	CopyrightYear         int
	Resource              resource
	SDKMethod             string
	SDKMethodArgs         []string
	RequiredImports       []string
	StandardImports       []string
	NamespaceRequired     bool
}

type resource struct {
	Name, NamePluralized string
	Namespaced           bool
	Fields               []field
}

type field struct {
	Name, FullType, Tag string
	Imports             []string
}

type entrypoint struct {
	Package           string
	CopyrightYear     int
	Actions           []action
	ManualActionNames []string
}

func main() {
	workspace := os.Getenv("BUILD_WORKSPACE_DIRECTORY")
	if workspace == "" {
		workspace = "."
	}
	sdkRoot := flag.String("sdk-root", "", "directory containing k8s.io/api and k8s.io/client-go sources")
	bundle := flag.String("bundle", "", "bundle to generate, e.g. core (default: all configured bundles)")
	source := flag.String("source", filepath.Join(workspace, "pkg/privateactionrunner/bundles/kubernetes"), "handwritten bundle source directory")
	out := flag.String("out", "", "output directory (default: source directory)")
	check := flag.Bool("check", false, "report stale generated files without changing them")
	flag.Parse()
	if *out == "" {
		*out = *source
	}
	if err := run(*sdkRoot, *bundle, *source, *out, *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(sdkRoot, bundle, source, out string, check bool) error {
	if sdkRoot == "" {
		return fmt.Errorf("--sdk-root is required; use bazel run //tools/privateactionrunner/kubernetes-generator")
	}
	raw, err := assets.ReadFile("config.yaml")
	if err != nil {
		return err
	}
	var targets map[string]targetConfig
	if err := yaml.Unmarshal(raw, &targets); err != nil {
		return err
	}
	if bundle != "" {
		cfg, ok := targets[bundle]
		if !ok {
			return fmt.Errorf("unknown bundle %q", bundle)
		}
		targets = map[string]targetConfig{bundle: cfg}
	}
	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	slices.Sort(names)
	// Render every selected bundle before writing, so parse failures leave the checkout intact.
	files := map[string][]byte{}
	for _, name := range names {
		generated, err := generateBundle(sdkRoot, name, filepath.Join(source, name), targets[name])
		if err != nil {
			return fmt.Errorf("generate %s: %w", name, err)
		}
		for filename, content := range generated {
			files[filepath.Join(name, filename)] = content
		}
	}
	var stale []string
	for _, name := range names {
		old, err := filepath.Glob(filepath.Join(out, name, "*.gen.go"))
		if err != nil {
			return err
		}
		for _, filename := range old {
			rel := filepath.Join(name, filepath.Base(filename))
			if _, ok := files[rel]; !ok {
				stale = append(stale, rel)
				if !check {
					if err := os.Remove(filename); err != nil {
						return err
					}
				}
			}
		}
	}
	for filename, content := range files {
		fullPath := filepath.Join(out, filename)
		old, err := os.ReadFile(fullPath)
		if err == nil && bytes.Equal(old, content) {
			continue
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		stale = append(stale, filename)
		if !check {
			if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(fullPath, content, 0o644); err != nil {
				return err
			}
		}
	}
	if check && len(stale) > 0 {
		slices.Sort(stale)
		return fmt.Errorf("stale generated files:\n%s", strings.Join(stale, "\n"))
	}
	return nil
}

func render(name string, data any) ([]byte, error) {
	raw, err := assets.ReadFile("templates/" + name + ".go.tmpl")
	if err != nil {
		return nil, err
	}
	funcs := template.FuncMap{
		"title":   title,
		"list":    func(values ...string) []string { return values },
		"mustHas": func(value string, values []string) bool { return slices.Contains(values, value) },
		"join":    func(sep string, values []string) string { return strings.Join(values, sep) },
	}
	tmpl, err := template.New(name).Funcs(funcs).Parse(string(raw))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	result, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format %s: %w", name, err)
	}
	return result, nil
}

func title(name string) string {
	if name == "" {
		return ""
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func snake(name string) string {
	runes := []rune(name)
	var out strings.Builder
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(runes[i-1]) || (i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
			out.WriteByte('_')
		}
		out.WriteRune(unicode.ToLower(r))
	}
	return out.String()
}
