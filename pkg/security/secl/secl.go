// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secl

import (
	"fmt"
	"strings"

	"github.com/alecthomas/participle/lexer"
)

// SprintExprAt returns a string sed to highlight the precise location of an error
func SprintExprAt(expr string, pos lexer.Position) string {
	column := pos.Column
	if column > 0 {
		column--
	}

	str := fmt.Sprintf("%s\n", expr)
	str += strings.Repeat(" ", column)
	str += "^"
	return str
}
