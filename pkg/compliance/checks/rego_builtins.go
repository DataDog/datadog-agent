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

const (
	ResourceIDFindingField     = "resource_id"
	ResourceTypeFindingField   = "resource_type"
	ResourceStatusFindingField = "status"
	ResourceDataFindingField   = "data"
)

const helpers = `
package datadog

docker_container_finding(status, c) = f {
	resid := sprintf("%s_%s", [input.context.hostname, cast_string(c.id)])
	f := finding(status, "docker_container", resid, {
		"container.id": c.id,
		"container.image": c.image,
		"container.name": c.name
	})
}

passed_docker_container_finding(c) = f {
	f := docker_container_finding(true, c)
}

failing_docker_container_finding(c) = f {
	f := docker_container_finding(false, c)
}

process_finding(status, p) = f {
	resid := sprintf("%s_daemon", [input.context.hostname])
	f := finding(status, "docker_daemon", resid, {
		"process.name": p.name,
		"process.exe": p.exe,
		"process.cmdLine": p.cmdLine
	})
}

passed_process_finding(p) = f {
	f := process_finding(true, p)
}

failing_process_finding(p) = f {
	f := process_finding(false, p)
}

file_finding(status, file) = fg {
	resid := sprintf("%s_daemon", [input.context.hostname])
	fg := finding(status, "docker_daemon", resid, {
		"file.group": file.group,
		"file.path": file.path,
		"file.permissions": file.permissions,
		"file.user": file.user,
	})
}

passed_file_finding(file) = fg {
	fg := file_finding(true, file)
}

failing_file_finding(file) = fg {
	fg := file_finding(false, file)
}
`

var regoBuiltins = []func(*rego.Rego){
	octalLiteralFunc,
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
