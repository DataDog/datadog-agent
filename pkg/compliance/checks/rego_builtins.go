// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"errors"
	"strconv"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/types"
)

var regoBuiltins = []func(*rego.Rego){
	octalLiteralFunc,
	findingBuilder("passed_process_finding", true, processResourceTermsExtractor),
	findingBuilder("failed_process_finding", false, processResourceTermsExtractor),
}

var octalLiteralFunc = rego.Function1(
	&rego.Function{
		Name: "parseOctal",
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

type resourceTerms struct {
	ID   *ast.Term
	Type *ast.Term
	Data *ast.Term
}

type termsExtractor func(*ast.Term) resourceTerms

func processResourceTermsExtractor(process *ast.Term) resourceTerms {
	return resourceTerms{
		ID:   ast.StringTerm("$hostname_daemon"),
		Type: ast.StringTerm("docker_daemon"),
		Data: ast.ObjectTerm(),
	}
}

func findingBuilder(name string, status bool, extractor termsExtractor) func(*rego.Rego) {
	return rego.Function1(
		&rego.Function{
			Name: name,
			Decl: types.NewFunction(types.Args(types.A), types.A),
		},
		func(_ rego.BuiltinContext, a *ast.Term) (*ast.Term, error) {
			terms := make([][2]*ast.Term, 0)
			resourceTerms := extractor(a)

			terms = append(terms, [2]*ast.Term{
				ast.StringTerm("resource_id"),
				resourceTerms.ID,
			})

			terms = append(terms, [2]*ast.Term{
				ast.StringTerm("resource_type"),
				resourceTerms.Type,
			})

			terms = append(terms, [2]*ast.Term{
				ast.StringTerm("data"),
				resourceTerms.Data,
			})

			terms = append(terms, [2]*ast.Term{
				ast.StringTerm("status"),
				ast.BooleanTerm(status),
			})

			return ast.ObjectTerm(terms...), nil
		},
	)
}
