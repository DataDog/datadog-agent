// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"errors"
	"fmt"
	"strconv"

	_ "embed"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/types"
)

//go:embed rego_helpers/datadog.rego
var helpers string

var regoBuiltins = []func(*rego.Rego){
	octalLiteralFunc,
	regoFileRegexp,
	regoFileJSON,
	regoFileYaml,
}

var octalLiteralFunc = rego.Function1(
	&rego.Function{
		Name: "parse_octal",
		Decl: types.NewFunction(types.Args(types.S), types.N),
	},
	func(_ rego.BuiltinContext, a *ast.Term) (*ast.Term, error) {
		str, ok := a.Value.(ast.String)
		if !ok {
			return nil, errors.New("failed to parse octal literal")
		}

		value, err := strconv.ParseInt(string(str), 8, 0)
		if err != nil {
			return nil, err
		}

		return ast.IntNumberTerm(int(value)), err
	},
)

var regoFileRegexp = createFileGetter("regexp", regexpGetter)
var regoFileJSON = createFileGetter("jq", jsonGetter)
var regoFileYaml = createFileGetter("yaml", yamlGetter)

func createFileGetter(suffix string, get getter) func(*rego.Rego) {
	return rego.Function2(
		&rego.Function{
			Name: fmt.Sprintf("file_%v", suffix),
			Decl: types.NewFunction(types.Args(types.S, types.S), types.A),
		},
		func(_ rego.BuiltinContext, a *ast.Term, b *ast.Term) (*ast.Term, error) {
			path, ok := a.Value.(ast.String)
			if !ok {
				return nil, errors.New("failed to parse path as string")
			}
			regexp, ok := b.Value.(ast.String)
			if !ok {
				return nil, errors.New("failed to parse regexp as string")
			}

			result, err := queryValueFromFile(string(path), string(regexp), get)
			return ast.StringTerm(result), err
		},
	)
}
