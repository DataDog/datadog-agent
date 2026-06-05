// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package uploader

import (
	"fmt"
	"slices"
	"strings"

	jsonv2 "github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"

	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
)

// PackageScope adapts a symdb.Package to the v2 JSON MarshalerTo interface so
// that callers can stream the package's JSON representation directly to a
// writer via jsonv2.MarshalWrite, without first materialising the full
// uploader.Scope tree for the whole package.
//
// Each child (function or type) is still converted to an uploader.Scope and
// passed through default reflection-based marshalling, but only one child is
// live at a time.
type PackageScope struct {
	pkg          symdb.Package
	agentVersion string
}

// NewPackageScope returns a marshaler that emits the JSON representation of
// pkg as a "package" scope.
func NewPackageScope(pkg symdb.Package, agentVersion string) PackageScope {
	return PackageScope{pkg: pkg, agentVersion: agentVersion}
}

// MarshalJSONTo implements the v2 JSON MarshalerTo interface.
func (p PackageScope) MarshalJSONTo(enc *jsontext.Encoder) (retErr error) {
	defer func() {
		switch r := recover().(type) {
		case nil: // no panic
		case error:
			retErr = r
		default:
			retErr = fmt.Errorf("panic: %v", r)
		}
	}()
	writeToken := func(token jsontext.Token) {
		if err := enc.WriteToken(token); err != nil {
			panic(err)
		}
	}
	writeTokens := func(tokens ...jsontext.Token) {
		for _, token := range tokens {
			writeToken(token)
		}
	}
	writeStringField := func(key, value string) {
		writeTokens(jsontext.String(key), jsontext.String(value))
	}
	writeIntField := func(key string, value int64) {
		writeTokens(jsontext.String(key), jsontext.Int(value))
	}

	writeToken(jsontext.BeginObject)
	writeStringField("scope_type", string(ScopeTypePackage))
	writeStringField("name", p.pkg.Name)
	writeIntField("start_line", 0)
	writeIntField("end_line", 0)
	writeTokens(jsontext.String("has_injectible_lines"), jsontext.Bool(false))
	if p.agentVersion != "" {
		writeTokens(jsontext.String("language_specifics"), jsontext.BeginObject)
		writeStringField("agent_version", p.agentVersion)
		writeToken(jsontext.EndObject)
	}
	if total := len(p.pkg.Functions) + len(p.pkg.Types); total > 0 {
		writeTokens(jsontext.String("scopes"), jsontext.BeginArray)

		// childRef refers to either a function or a type (funcIdx == -1).
		type childRef struct {
			name    string // sort key
			funcIdx int    // valid if typeName == ""
		}
		refs := make([]childRef, 0, len(p.pkg.Functions)+len(p.pkg.Types))
		for i, fn := range p.pkg.Functions {
			refs = append(refs, childRef{name: fn.Name, funcIdx: i})
		}
		for name := range p.pkg.Types {
			refs = append(refs, childRef{name: name, funcIdx: -1})
		}
		slices.SortFunc(refs, func(a, b childRef) int {
			return strings.Compare(a.name, b.name)
		})
		for _, ref := range refs {
			var child Scope
			if ref.funcIdx == -1 {
				child = convertTypeToScope(*p.pkg.Types[ref.name])
			} else {
				const isMethod = false
				child = convertFunctionToScope(p.pkg.Functions[ref.funcIdx], isMethod)
			}
			if err := jsonv2.MarshalEncode(enc, &child); err != nil {
				return err
			}
		}
		writeToken(jsontext.EndArray)
	}
	writeToken(jsontext.EndObject)
	return nil
}
