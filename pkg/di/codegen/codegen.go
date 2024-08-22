// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package codegen

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"text/template"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/di/ditypes"
)

type BPFProgram struct {
	ProgramText string

	// Used for bpf code generation
	Probe                  *ditypes.Probe
	PopulatedParameterText string
}

// GenerateBPFProgram generates the source code associated with the probe and data
// in it's associated proccess info.
func GenerateBPFProgram(procInfo *ditypes.ProcessInfo, probe *ditypes.Probe) error {
	prog := &BPFProgram{
		Probe: probe,
	}

	programTemplate, err := template.New("program_template").Parse(programTemplateText)
	if err != nil {
		return err
	}

	if probe.InstrumentationInfo.InstrumentationOptions.CaptureParameters {
		params := applyCaptureDepth(procInfo.TypeMap.Functions[probe.FuncName], probe.InstrumentationInfo.InstrumentationOptions.MaxReferenceDepth)
		applyFieldCountLimit(params)
		parametersText := ""
		for i := range params {
			flattenedParams := flattenParameters([]ditypes.Parameter{params[i]})

			headersText, err := generateHeadersText(flattenedParams)
			if err != nil {
				return err
			}
			parametersText += headersText

			valueText, err := generateParametersText(flattenedParams)
			if err != nil {
				return err
			}
			parametersText += valueText
		}

		prog.PopulatedParameterText = parametersText
	} else {
		log.Info("Not capturing parameters")
	}

	buf := new(bytes.Buffer)
	err = programTemplate.Execute(buf, prog)
	if err != nil {
		return err
	}

	log.Debug(buf.String())
	probe.InstrumentationInfo.BPFSourceCode = buf.String()

	return nil
}

func resolveHeaderTemplate(param *ditypes.Parameter) (*template.Template, error) {
	switch param.Kind {
	case uint(reflect.String):
		if param.Location.InReg {
			return template.New("string_reg_header_template").Parse(stringRegisterHeaderTemplateText)
		}
		return template.New("string_stack_header_template").Parse(stringStackHeaderTemplateText)
	case uint(reflect.Slice):
		if param.Location.InReg {
			return template.New("slice_reg_header_template").Parse(sliceRegisterHeaderTemplateText)
		}
		return template.New("slice_stack_header_template").Parse(sliceStackHeaderTemplateText)
	default:
		return template.New("header_template").Parse(headerTemplateText)
	}
}

func generateHeadersText(params []ditypes.Parameter) (string, error) {
	fullHeaderText := ""
	for i := range params {
		text, err := generateHeaderText(params[i])
		if err != nil {
			return "", err
		}
		fullHeaderText += text
	}
	return fullHeaderText, nil
}

func generateHeaderText(param ditypes.Parameter) (string, error) {
	if reflect.Kind(param.Kind) == reflect.Slice {
		return generateSliceHeader(&param)
	}

	tmplt, err := resolveHeaderTemplate(&param)
	if err != nil {
		return "", err
	}
	buf := new(bytes.Buffer)
	err = tmplt.Execute(buf, param)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func generateParametersText(params []ditypes.Parameter) (string, error) {
	parametersText := ""
	for i := range params {
		parameterText, err := generateParameterText(&params[i])
		if err != nil {
			return "", err
		}
		parametersText += parameterText
	}
	return parametersText, nil
}

func generateParameterText(param *ditypes.Parameter) (string, error) {

	if param.Kind == uint(reflect.Array) ||
		param.Kind == uint(reflect.Struct) ||
		param.Kind == uint(reflect.Pointer) {
		// - Arrays/structs don't have actual values, we just want to generate
		// a header for them for the sake of event parsing.
		// - Pointers do have actual values, but they're captured when the
		// underlying value is also captured.
		return "", nil
	}

	buf := new(bytes.Buffer)
	template, err := resolveParameterTemplate(param)
	if err != nil {
		return "", err
	}
	param.Type = cleanupTypeName(param.Type)
	err = template.Execute(buf, param)
	if err != nil {
		return "", fmt.Errorf("could not execute template for generating read of parameter: %w", err)
	}
	return buf.String(), nil
}

func resolveParameterTemplate(param *ditypes.Parameter) (*template.Template, error) {
	if param.Type == "main.triggerVerifierErrorForTesting" {
		return template.New("trigger_verifier_error_template").Parse(forcedVerifierErrorTemplate)
	}
	notSupported := param.NotCaptureReason == ditypes.Unsupported
	cutForFieldLimit := param.NotCaptureReason == ditypes.FieldLimitReached

	if notSupported {
		return template.New("unsupported_type_template").Parse(unsupportedTypeTemplateText)
	} else if cutForFieldLimit {
		return template.New("cut_field_limit_template").Parse(cutForFieldLimitTemplateText)
	}

	if param.Location.InReg {
		return resolveRegisterParameterTemplate(param)
	} else {
		return resolveStackParameterTemplate(param)
	}
}

func resolveRegisterParameterTemplate(param *ditypes.Parameter) (*template.Template, error) {
	needsDereference := param.Location.NeedsDereference
	stringType := param.Kind == uint(reflect.String)
	sliceType := param.Kind == uint(reflect.Slice)

	if needsDereference {
		// Register Pointer
		return template.New("pointer_register_template").Parse(pointerRegisterTemplateText)
	} else if stringType {
		// Register String
		return template.New("string_register_template").Parse(stringRegisterTemplateText)
	} else if sliceType {
		// Register Slice
		return template.New("slice_register_template").Parse(sliceRegisterTemplateText)
	} else if !needsDereference {
		// Register Normal Value
		return template.New("register_template").Parse(normalValueRegisterTemplateText)
	}
	return nil, errors.New("no template created: invalid or unsupported type")
}

func resolveStackParameterTemplate(param *ditypes.Parameter) (*template.Template, error) {
	needsDereference := param.Location.NeedsDereference
	stringType := param.Kind == uint(reflect.String)
	sliceType := param.Kind == uint(reflect.Slice)

	if needsDereference {
		// Stack Pointer
		return template.New("pointer_stack_template").Parse(pointerStackTemplateText)
	} else if stringType {
		// Stack String
		return template.New("string_stack_template").Parse(stringStackTemplateText)
	} else if sliceType {
		// Stack Slice
		return template.New("slice_stack_template").Parse(sliceStackTemplateText)
	} else if !needsDereference {
		// Stack Normal Value
		return template.New("stack_template").Parse(normalValueStackTemplateText)
	}
	return nil, errors.New("no template created: invalid or unsupported type")
}

func cleanupTypeName(s string) string {
	return strings.TrimPrefix(s, "*")
}

func generateSliceHeader(slice *ditypes.Parameter) (string, error) {
	if slice == nil {
		return "", errors.New("nil slice parameter when generating header code")
	}
	if len(slice.ParameterPieces) != 1 {
		return "", errors.New("invalid slice parameter when generating header code")
	}
	headerText, err := generateHeaderText(slice.ParameterPieces[0])
	if err != nil {
		return "", err
	}
	w := sliceHeaderWrapper{
		Parameter:           slice,
		SliceTypeHeaderText: headerText,
	}

	sliceTemplate, err := resolveHeaderTemplate(slice)
	if err != nil {
		return "", err
	}
	buf := new(bytes.Buffer)

	err = sliceTemplate.Execute(buf, w)
	if err != nil {
		return "", fmt.Errorf("could not execute template for generating slice header: %w", err)
	}
	return buf.String(), nil
}

type sliceHeaderWrapper struct {
	Parameter           *ditypes.Parameter
	SliceTypeHeaderText string
}
