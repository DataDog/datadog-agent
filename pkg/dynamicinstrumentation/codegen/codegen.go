// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package codegen is used to generate bpf program source code based on probe definitions
package codegen

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"text/template"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GenerateBPFParamsCode generates the source code associated with the probe and data
// in it's associated process info.
func GenerateBPFParamsCode(procInfo *ditypes.ProcessInfo, probe *ditypes.Probe) error {
	parameterBytes := []byte{}
	out := bytes.NewBuffer(parameterBytes)

	if probe.InstrumentationInfo.InstrumentationOptions.CaptureParameters {
		params := procInfo.TypeMap.Functions[probe.FuncName]
		for i := range params {
			flattenedParams := flattenParameters([]*ditypes.Parameter{params[i]})
			err := generateHeadersText(flattenedParams, out)
			if err != nil {
				return err
			}
			err = generateParametersTextViaLocationExpressions(flattenedParams, out)
			if err != nil {
				return err
			}
		}
	} else {
		log.Info("Not capturing parameters")
	}

	probe.InstrumentationInfo.BPFParametersSourceCode = out.String()
	return nil
}

func resolveHeaderTemplate(param *ditypes.Parameter) (*template.Template, error) {
	switch param.Kind {
	case uint(reflect.String):
		return template.New("string_header_template").Parse(stringHeaderTemplateText)
	case uint(reflect.Slice):
		if param.Location != nil && param.Location.InReg {
			return template.New("slice_reg_header_template").Parse(sliceRegisterHeaderTemplateText)
		}
		return template.New("slice_stack_header_template").Parse(sliceStackHeaderTemplateText)
	default:
		return template.New("header_template").Parse(headerTemplateText)
	}
}

func generateHeadersText(params []*ditypes.Parameter, out io.Writer) error {
	for i := range params {
		err := generateHeaderText(params[i], out)
		if err != nil {
			return err
		}
	}
	return nil
}

func generateHeaderText(param *ditypes.Parameter, out io.Writer) error {
	if reflect.Kind(param.Kind) == reflect.Slice {
		return generateSliceHeader(param, out)
	} else if reflect.Kind(param.Kind) == reflect.String {
		return generateStringHeader(param, out)
	}
	template, err := resolveHeaderTemplate(param)
	if err != nil {
		return err
	}
	err = template.Execute(out, param)
	if err != nil {
		return err
	}
	if len(param.ParameterPieces) != 0 {
		return generateHeadersText(param.ParameterPieces, out)
	}
	return nil
}

func generateParametersTextViaLocationExpressions(params []*ditypes.Parameter, out io.Writer) error {
	for i := range params {
		collectedExpressions := collectLocationExpressions(params[i])
		for _, locationExpression := range collectedExpressions {
			template, err := resolveLocationExpressionTemplate(locationExpression)
			if err != nil {
				return err
			}
			err = template.Execute(out, locationExpression)
			if err != nil {
				return fmt.Errorf("could not execute template for generating location expression: %w", err)
			}
		}
	}
	return nil
}

// collectLocationExpressions goes through the parameter tree (param.ParameterPieces) via
// breadth first traversal, collecting the LocationExpression's from each parameter and appending them
// to a collective slice.
func collectLocationExpressions(param *ditypes.Parameter) []ditypes.LocationExpression {
	collectedExpressions := []ditypes.LocationExpression{}
	queue := []*ditypes.Parameter{param}
	var top *ditypes.Parameter

	for {
		if len(queue) == 0 {
			break
		}
		top = queue[0]
		queue = queue[1:]

		if top == nil {
			continue
		}
		queue = append(queue, top.ParameterPieces...)
		if len(top.LocationExpressions) > 0 {
			expressions := []ditypes.LocationExpression{}
			for i := range top.LocationExpressions {
				expressions = append(expressions, collectSubLocationExpressions(top.LocationExpressions[i])...)
			}
			collectedExpressions = append(expressions, collectedExpressions...)
			top.LocationExpressions = []ditypes.LocationExpression{}
		}
	}
	return collectedExpressions
}

func collectSubLocationExpressions(location ditypes.LocationExpression) []ditypes.LocationExpression {
	collectedExpressions := []ditypes.LocationExpression{}
	queue := []ditypes.LocationExpression{location}
	var top ditypes.LocationExpression

	for len(queue) != 0 {
		top = queue[0]
		queue = queue[1:]
		queue = append(queue, top.IncludedExpressions...)
		if top.Opcode != ditypes.OpPopPointerAddress {
			collectedExpressions = append(collectedExpressions, top)
		}
		top.IncludedExpressions = []ditypes.LocationExpression{}
	}
	return collectedExpressions
}

func resolveLocationExpressionTemplate(locationExpression ditypes.LocationExpression) (*template.Template, error) {
	switch locationExpression.Opcode {
	case ditypes.OpReadUserRegister:
		return template.New("read_register_location_expression").Parse(readRegisterTemplateText)
	case ditypes.OpReadUserStack:
		return template.New("read_stack_location_expression").Parse(readStackTemplateText)
	case ditypes.OpReadUserRegisterToOutput:
		return template.New("read_register_to_output_location_expression").Parse(readRegisterValueToOutputTemplateText)
	case ditypes.OpReadUserStackToOutput:
		return template.New("read_stack_to_output_location_expression").Parse(readStackValueToOutputTemplateText)
	case ditypes.OpDereference:
		return template.New("dereference_location_expression").Parse(dereferenceTemplateText)
	case ditypes.OpDereferenceToOutput:
		return template.New("dereference_to_output_location_expression").Parse(dereferenceToOutputTemplateText)
	case ditypes.OpDereferenceLarge:
		return template.New("dereference_large_location_expression").Parse(dereferenceLargeTemplateText)
	case ditypes.OpDereferenceLargeToOutput:
		return template.New("dereference_large_to_output_location_expression").Parse(dereferenceLargeToOutputTemplateText)
	case ditypes.OpDereferenceDynamic:
		return template.New("dereference_dynamic_location_expression").Parse(dereferenceDynamicTemplateText)
	case ditypes.OpDereferenceDynamicToOutput:
		return template.New("dereference_dynamic_to_output_location_expression").Parse(dereferenceDynamicToOutputTemplateText)
	case ditypes.OpReadStringToOutput:
		return template.New("read_string_to_output").Parse(readStringToOutputTemplateText)
	case ditypes.OpApplyOffset:
		return template.New("apply_offset_location_expression").Parse(applyOffsetTemplateText)
	case ditypes.OpPop:
		return template.New("pop_location_expression").Parse(popTemplateText)
	case ditypes.OpCopy:
		return template.New("copy_location_expression").Parse(copyTemplateText)
	case ditypes.OpLabel:
		return template.New("label").Parse(labelTemplateText)
	case ditypes.OpSetGlobalLimit:
		return template.New("set_limit_entry").Parse(setLimitEntryText)
	case ditypes.OpJumpIfGreaterThanLimit:
		return template.New("jump_if_greater_than_limit").Parse(jumpIfGreaterThanLimitText)
	case ditypes.OpPrintStatement:
		return template.New("print_statement").Parse(printStatementText)
	case ditypes.OpComment:
		return template.New("comment").Parse(commentText)
	default:
		return nil, errors.New("invalid location expression opcode")
	}
}

func generateSliceHeader(slice *ditypes.Parameter, out io.Writer) error {
	// Slices are defined with an "array" pointer as piece 0, which is a pointer to the actual
	// type, which is defined as piece 0 under that.

	// Validate entire data structure is valid and not nil before accessing
	if slice == nil ||
		len(slice.ParameterPieces) != 3 ||
		slice.ParameterPieces[0] == nil ||
		slice.ParameterPieces[1] == nil ||
		len(slice.ParameterPieces[0].ParameterPieces) != 1 ||
		slice.ParameterPieces[0].ParameterPieces[0] == nil {
		return errors.New("malformed slice type")
	}

	typeHeaderBytes := []byte{}
	typeHeaderBuf := bytes.NewBuffer(typeHeaderBytes)
	lenHeaderBytes := []byte{}
	lenHeaderBuf := bytes.NewBuffer(lenHeaderBytes)
	lenHeaderBuf.Write([]byte("// Capture length of slice:"))
	err := generateHeaderText(slice.ParameterPieces[0].ParameterPieces[0], typeHeaderBuf)
	if err != nil {
		return fmt.Errorf("could not generate header text for underlying slice element type: %w", err)
	}
	if slice == nil || len(slice.ParameterPieces) == 0 || slice.ParameterPieces[1] == nil {
		return fmt.Errorf("could not read slice length parameter")
	}
	excludePopPointerAddressExpressions(&slice.ParameterPieces[1].LocationExpressions)
	err = generateParametersTextViaLocationExpressions([]*ditypes.Parameter{slice.ParameterPieces[1]}, lenHeaderBuf)
	if err != nil {
		return err
	}
	slice.ParameterPieces[1].LocationExpressions = []ditypes.LocationExpression{}
	w := sliceHeaderWrapper{
		Parameter:           slice,
		SliceTypeHeaderText: lenHeaderBuf.String() + typeHeaderBuf.String(),
	}

	sliceTemplate, err := resolveHeaderTemplate(slice)
	if err != nil {
		return fmt.Errorf("could not resolve header for slice type: %w", err)
	}

	err = sliceTemplate.Execute(out, w)
	if err != nil {
		return fmt.Errorf("could not execute template for generating slice header: %w", err)
	}

	return nil
}

func generateStringHeader(stringParam *ditypes.Parameter, out io.Writer) error {
	if stringParam == nil {
		return errors.New("nil string parameter when generating header code")
	}
	if len(stringParam.ParameterPieces) != 2 {
		return fmt.Errorf("invalid string parameter when generating header code (pieces len %d)", len(stringParam.ParameterPieces))
	}
	stringHeaderTemplate, err := resolveHeaderTemplate(stringParam)
	if err != nil {
		return err
	}
	err = stringHeaderTemplate.Execute(out, stringParam)
	if err != nil {
		return fmt.Errorf("could not execute template for generating string header: %w", err)
	}
	if stringParam == nil || len(stringParam.ParameterPieces) == 0 || stringParam.ParameterPieces[1] == nil {
		return fmt.Errorf("could not read string length parameter")
	}
	excludePopPointerAddressExpressions(&stringParam.ParameterPieces[1].LocationExpressions)
	err = generateParametersTextViaLocationExpressions([]*ditypes.Parameter{stringParam.ParameterPieces[1]}, out)
	if err != nil {
		return err
	}
	if stringParam.ParameterPieces[1] != nil {
		stringParam.ParameterPieces[1].LocationExpressions = []ditypes.LocationExpression{}
	}
	return nil
}

func excludePopPointerAddressExpressions(expressions *[]ditypes.LocationExpression) {
	if expressions == nil {
		return
	}
	filteredExpressions := []ditypes.LocationExpression{}
	for i := range *expressions {
		if (*expressions)[i].Opcode != ditypes.OpPopPointerAddress {
			filteredExpressions = append(filteredExpressions, (*expressions)[i])
		}
	}
	*expressions = filteredExpressions
}

type sliceHeaderWrapper struct {
	Parameter           *ditypes.Parameter
	SliceTypeHeaderText string
}
