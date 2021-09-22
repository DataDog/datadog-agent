// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"errors"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/types"
)

const (
	ResourceIDFindingField     = "resource_id"
	ResourceTypeFindingField   = "resource_type"
	ResourceStatusFindingField = "status"
	ResourceDataFindingField   = "data"
)

var regoBuiltins = []func(*rego.Rego){
	octalLiteralFunc,
	findingBuilder("passed_process_finding", true, processResourceTermsExtractor),
	findingBuilder("failed_process_finding", false, processResourceTermsExtractor),
	rawFinding,
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

type resourceTerms struct {
	ID   *ast.Term
	Type *ast.Term
	Data *ast.Term
}

type termsExtractor func(*ast.Term) (resourceTerms, error)

func processResourceTermsExtractor(process *ast.Term) (resourceTerms, error) {
	dataTerms := [][2]*ast.Term{
		{
			ast.StringTerm(compliance.ProcessFieldName),
			process.Get(ast.StringTerm("name")),
		},
		{
			ast.StringTerm(compliance.ProcessFieldExe),
			process.Get(ast.StringTerm("exe")),
		},
		{
			ast.StringTerm(compliance.ProcessFieldCmdLine),
			process.Get(ast.StringTerm("cmdLine")),
		},
	}

	return resourceTerms{
		ID:   ast.StringTerm("$hostname_daemon"),
		Type: ast.StringTerm("docker_daemon"),
		Data: ast.ObjectTerm(dataTerms...),
	}, nil
}

var rawFinding = rego.Function4(
	&rego.Function{
		Name: "finding",
		Decl: types.NewFunction(types.Args(types.B, types.S, types.S, types.A), types.A),
	},
	func(_ rego.BuiltinContext, status, resType, resID, data *ast.Term) (*ast.Term, error) {
		terms := [][2]*ast.Term{
			{
				ast.StringTerm(ResourceIDFindingField),
				resID,
			},
			{
				ast.StringTerm(ResourceTypeFindingField),
				resType,
			},
			{
				ast.StringTerm(ResourceDataFindingField),
				data,
			},
			{
				ast.StringTerm(ResourceStatusFindingField),
				status,
			},
		}

		return ast.ObjectTerm(terms...), nil
	},
)

func findingBuilder(name string, status bool, extractor termsExtractor) func(*rego.Rego) {
	return rego.Function1(
		&rego.Function{
			Name: name,
			Decl: types.NewFunction(types.Args(types.A), types.A),
		},
		func(_ rego.BuiltinContext, a *ast.Term) (*ast.Term, error) {
			resourceTerms, err := extractor(a)
			if err != nil {
				return nil, err
			}

			terms := [][2]*ast.Term{
				{
					ast.StringTerm(ResourceIDFindingField),
					resourceTerms.ID,
				},

				{
					ast.StringTerm(ResourceTypeFindingField),
					resourceTerms.Type,
				},

				{
					ast.StringTerm(ResourceDataFindingField),
					resourceTerms.Data,
				},

				{
					ast.StringTerm(ResourceStatusFindingField),
					ast.BooleanTerm(status),
				},
			}

			return ast.ObjectTerm(terms...), nil
		},
	)
}
