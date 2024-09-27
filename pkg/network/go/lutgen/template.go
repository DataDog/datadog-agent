// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package lutgen

import (
	"bytes"
	_ "embed"
	"fmt"
	"go/format"
	"io"
	"text/template"

	"github.com/DataDog/datadog-agent/pkg/network/go/goversion"
)

//go:embed template.go.tpl
var templateContents string

var sourceTemplate = template.Must(template.New("").Parse(templateContents))

type templateArgs struct {
	Package                string
	Imports                []string
	MinGoVersion           goversion.GoVersion
	SupportedArchitectures []string
	LookupFunctions        []lookupFunctionTemplateArgs
}

type lookupFunctionTemplateArgs struct {
	Name               string
	OutputType         string
	OutputZeroValue    string
	RenderedDocComment string
	ArchCases          []archCaseTemplateArgs
}

type archCaseTemplateArgs struct {
	Arch     string
	HasMin   bool
	Min      goversion.GoVersion
	Branches []branchTemplateArgs
}

type branchTemplateArgs struct {
	Version       goversion.GoVersion
	RenderedValue string
}

func (t *templateArgs) Render(writer io.Writer) error {
	// Render the template to Go source, and write to a buffer
	var buf bytes.Buffer
	if err := sourceTemplate.Execute(&buf, t); err != nil {
		return fmt.Errorf("error while executing source template: %w", err)
	}

	// Format the resultant rendered source
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		// Print out the raw source
		return fmt.Errorf("error while formatting generated source: %w\nRaw source:\n=====\n%s\n=====", err, buf.String())
	}

	_, err = writer.Write(formatted)
	if err != nil {
		return err
	}

	return nil
}
