// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"embed"
	"io"

	statusdef "github.com/DataDog/datadog-agent/comp/core/status/def"

	pkghtmltemplate "github.com/DataDog/datadog-agent/pkg/template/html"
	pkgtexttemplate "github.com/DataDog/datadog-agent/pkg/template/text"
)

// HTMLFmap return a map of utility functions for HTML templating
func HTMLFmap() pkghtmltemplate.FuncMap {
	return statusdef.HTMLFmap()
}

// TextFmap map of utility functions for text templating
func TextFmap() pkgtexttemplate.FuncMap {
	return statusdef.TextFmap()
}

// RenderHTML reads, parse and execute template from embed.FS
func RenderHTML(templateFS embed.FS, template string, buffer io.Writer, data any) error {
	return statusdef.RenderHTML(templateFS, template, buffer, data)
}

// RenderText reads, parse and execute template from embed.FS
func RenderText(templateFS embed.FS, template string, buffer io.Writer, data any) error {
	return statusdef.RenderText(templateFS, template, buffer, data)
}

// PrintDashes repeats the pattern (dash) for the length of s
func PrintDashes(s string, dash string) string {
	return statusdef.PrintDashes(s, dash)
}
