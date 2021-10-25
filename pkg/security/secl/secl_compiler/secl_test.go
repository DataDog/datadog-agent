// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secl

import (
	"testing"

	"github.com/alecthomas/participle/lexer"

	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
)

func TestExprAt(t *testing.T) {
	rule, err := ast.ParseRule(`process.name != "/usr/bin/vipw" && open.pathname == "/etc/passwd" && (open.mode == O_TRUNC || open.mode == O_CREAT || open.mode == O_WRONLY)`)
	if err != nil {
		t.Error(err)
	}

	t.Log(SprintExprAt(rule.Expr, lexer.Position{Column: 22}))
}
