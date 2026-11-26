// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Command gen extracts AWS partition endpoint metadata from aws-sdk-go-v2's
// internal/endpoints/awsrulesfn/partitions.go into a generated Go file for
// pkg/util/aws/query.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strconv"
)

type parsedPartition struct {
	ID                 string
	RegionRegex        string
	DNSSuffix          string
	DualStackDNSSuffix string
	SupportsFIPS       bool
	SupportsDualStack  bool
}

func main() {
	inPath := flag.String("in", "", "path to aws-sdk-go-v2 internal/endpoints/awsrulesfn/partitions.go")
	outPath := flag.String("out", "", "path to write generated Go file")
	flag.Parse()

	if *inPath == "" || *outPath == "" {
		fmt.Fprintln(os.Stderr, "both -in and -out flags are required")
		os.Exit(2)
	}

	src, err := os.ReadFile(*inPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", *inPath, err)
		os.Exit(1)
	}

	generated, err := generateFromSource(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate partitions from %s: %v\n", *inPath, err)
		os.Exit(1)
	}

	if err := os.WriteFile(*outPath, generated, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", *outPath, err)
		os.Exit(1)
	}
}

func generateFromSource(src []byte) ([]byte, error) {
	partitions, err := parsePartitions(src)
	if err != nil {
		return nil, err
	}
	return renderPartitions(partitions)
}

func parsePartitions(src []byte) ([]parsedPartition, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "partitions.go", src, 0)
	if err != nil {
		return nil, fmt.Errorf("parse Go source: %w", err)
	}

	var sliceLit *ast.CompositeLit
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR {
			continue
		}
		for _, spec := range genDecl.Specs {
			valSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range valSpec.Names {
				if name.Name == "partitions" && i < len(valSpec.Values) {
					lit, ok := valSpec.Values[i].(*ast.CompositeLit)
					if !ok {
						return nil, errors.New("partitions is not a composite literal")
					}
					sliceLit = lit
					break
				}
			}
		}
	}

	if sliceLit == nil {
		return nil, errors.New("partitions variable not found")
	}

	parsed := make([]parsedPartition, 0, len(sliceLit.Elts))
	hasDefaultAWS := false

	for idx, elt := range sliceLit.Elts {
		lit, ok := elt.(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("partition[%d] is not a composite literal", idx)
		}
		p, err := parsePartitionLiteral(lit)
		if err != nil {
			return nil, fmt.Errorf("partition[%d]: %w", idx, err)
		}
		if p.ID == "aws" {
			hasDefaultAWS = true
		}
		parsed = append(parsed, p)
	}

	if len(parsed) == 0 {
		return nil, errors.New("no partitions parsed from partitions")
	}
	if !hasDefaultAWS {
		return nil, errors.New(`required default partition "aws" not found`)
	}

	return parsed, nil
}

func parsePartitionLiteral(lit *ast.CompositeLit) (parsedPartition, error) {
	var p parsedPartition
	var gotConfig bool

	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyIdent, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch keyIdent.Name {
		case "ID":
			s, err := stringLit(kv.Value)
			if err != nil {
				return p, fmt.Errorf("ID: %w", err)
			}
			p.ID = s
		case "RegionRegex":
			s, err := stringLit(kv.Value)
			if err != nil {
				return p, fmt.Errorf("RegionRegex: %w", err)
			}
			if _, err := regexp.Compile(s); err != nil {
				return p, fmt.Errorf("invalid RegionRegex %q: %w", s, err)
			}
			p.RegionRegex = s
		case "DefaultConfig":
			cfgLit, ok := kv.Value.(*ast.CompositeLit)
			if !ok {
				return p, errors.New("DefaultConfig is not a composite literal")
			}
			if err := parseDefaultConfig(cfgLit, &p); err != nil {
				return p, fmt.Errorf("DefaultConfig: %w", err)
			}
			gotConfig = true
		case "Regions":
			if err := checkNoNonNilRegionOverrides(kv.Value); err != nil {
				return p, fmt.Errorf("Regions: %w", err)
			}
		}
	}

	if p.ID == "" {
		return p, errors.New("missing ID")
	}
	if p.RegionRegex == "" {
		return p, fmt.Errorf("partition %q missing RegionRegex", p.ID)
	}
	if !gotConfig || p.DNSSuffix == "" || p.DualStackDNSSuffix == "" {
		return p, fmt.Errorf("partition %q missing DefaultConfig DNS suffixes", p.ID)
	}
	return p, nil
}

func parseDefaultConfig(lit *ast.CompositeLit, p *parsedPartition) error {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyIdent, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch keyIdent.Name {
		case "DnsSuffix":
			s, err := stringLit(kv.Value)
			if err != nil {
				return fmt.Errorf("DnsSuffix: %w", err)
			}
			p.DNSSuffix = s
		case "DualStackDnsSuffix":
			s, err := stringLit(kv.Value)
			if err != nil {
				return fmt.Errorf("DualStackDnsSuffix: %w", err)
			}
			p.DualStackDNSSuffix = s
		case "SupportsFIPS":
			b, err := boolIdent(kv.Value)
			if err != nil {
				return fmt.Errorf("SupportsFIPS: %w", err)
			}
			p.SupportsFIPS = b
		case "SupportsDualStack":
			b, err := boolIdent(kv.Value)
			if err != nil {
				return fmt.Errorf("SupportsDualStack: %w", err)
			}
			p.SupportsDualStack = b
		}
	}
	return nil
}

// checkNoNonNilRegionOverrides fails loudly if a future aws-sdk-go-v2 update introduces
// per-region DNS/FIPS/DualStack overrides that the simplified partition table would ignore.
func checkNoNonNilRegionOverrides(expr ast.Expr) error {
	mapLit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	for _, entry := range mapLit.Elts {
		kv, ok := entry.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		regionName, _ := stringLit(kv.Key)
		overrideLit, ok := kv.Value.(*ast.CompositeLit)
		if !ok {
			continue
		}
		for _, field := range overrideLit.Elts {
			fkv, ok := field.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			ident, ok := fkv.Value.(*ast.Ident)
			if !ok || ident.Name != "nil" {
				fieldName := ""
				if k, ok := fkv.Key.(*ast.Ident); ok {
					fieldName = k.Name
				}
				return fmt.Errorf("region %q has non-nil override for %s; update pkg/util/aws/query/gen to support per-region overrides", regionName, fieldName)
			}
		}
	}
	return nil
}

func stringLit(expr ast.Expr) (string, error) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", fmt.Errorf("expected string literal, got %T", expr)
	}
	return strconv.Unquote(lit.Value)
}

func boolIdent(expr ast.Expr) (bool, error) {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false, fmt.Errorf("expected bool identifier, got %T", expr)
	}
	switch ident.Name {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("unexpected bool identifier %q", ident.Name)
	}
}

func renderPartitions(partitions []parsedPartition) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(`// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Code generated by bazel run //pkg/util/aws/query:update_partitions; DO NOT EDIT.
// Source: github.com/aws/aws-sdk-go-v2/internal/endpoints/awsrulesfn/partitions.go

package query

var awsPartitions = []partition{
`)

	for _, p := range partitions {
		fmt.Fprintf(&buf, "\t{\n")
		fmt.Fprintf(&buf, "\t\tid:                 %q,\n", p.ID)
		fmt.Fprintf(&buf, "\t\tregionRegexPattern: %q,\n", p.RegionRegex)
		fmt.Fprintf(&buf, "\t\tdnsSuffix:          %q,\n", p.DNSSuffix)
		fmt.Fprintf(&buf, "\t\tdualStackDNSSuffix: %q,\n", p.DualStackDNSSuffix)
		fmt.Fprintf(&buf, "\t\tsupportsFIPS:       %t,\n", p.SupportsFIPS)
		fmt.Fprintf(&buf, "\t\tsupportsDualStack:  %t,\n", p.SupportsDualStack)
		fmt.Fprintf(&buf, "\t},\n")
	}

	buf.WriteString("}\n")

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format generated source: %w", err)
	}
	return formatted, nil
}
