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
	"strings"
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
		params := procInfo.TypeMap.Functions[probe.FuncName] //applyCaptureDepth(procInfo.TypeMap.Functions[probe.FuncName], probe.InstrumentationInfo.InstrumentationOptions.MaxReferenceDepth)
		if params != nil {
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
	} else { //nolint:revive // TODO
		tmplt, err := resolveHeaderTemplate(param)
		if err != nil {
			return err
		}
		err = tmplt.Execute(out, param)
		if err != nil {
			return err
		}
		if len(param.ParameterPieces) != 0 {
			return generateHeadersText(param.ParameterPieces, out)
		}
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
// depth first traversal, collecting the LocationExpression's from each parameter and appending them
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
		for i := range top.ParameterPieces {
			queue = append(queue, top.ParameterPieces[i])
		}
		if len(top.LocationExpressions) > 0 {
			collectedExpressions = append(top.LocationExpressions, collectedExpressions...)
		}
	}
	return collectedExpressions
}

func resolveLocationExpressionTemplate(locationExpression ditypes.LocationExpression) (*template.Template, error) {
	if locationExpression.Opcode == ditypes.OpReadUserRegister {
		return template.New("read_register_location_expression").Parse(readRegisterTemplateText)
	}
	if locationExpression.Opcode == ditypes.OpReadUserStack {
		return template.New("read_stack_location_expression").Parse(readStackTemplateText)
	}
	if locationExpression.Opcode == ditypes.OpReadUserRegisterToOutput {
		return template.New("read_register_to_output_location_expression").Parse(readRegisterValueToOutputTemplateText)
	}
	if locationExpression.Opcode == ditypes.OpReadUserStackToOutput {
		return template.New("read_stack_to_output_location_expression").Parse(readStackValueToOutputTemplateText)
	}
	if locationExpression.Opcode == ditypes.OpDereference {
		return template.New("dereference_location_expression").Parse(dereferenceTemplateText)
	}
	if locationExpression.Opcode == ditypes.OpDereferenceToOutput {
		return template.New("dereference_to_output_location_expression").Parse(dereferenceToOutputTemplateText)
	}
	if locationExpression.Opcode == ditypes.OpDereferenceLarge {
		return template.New("dereference_large_location_expression").Parse(dereferenceLargeTemplateText)
	}
	if locationExpression.Opcode == ditypes.OpDereferenceLargeToOutput {
		return template.New("dereference_large_to_output_location_expression").Parse(dereferenceLargeToOutputTemplateText)
	}
	if locationExpression.Opcode == ditypes.OpDereferenceDynamic {
		return template.New("dereference_dynamic_location_expression").Parse(dereferenceDynamicTemplateText)
	}
	if locationExpression.Opcode == ditypes.OpDereferenceDynamicToOutput {
		return template.New("dereference_dynamic_to_output_location_expression").Parse(dereferenceDynamicToOutputTemplateText)
	}
	if locationExpression.Opcode == ditypes.OpReadStringToOutput {
		return template.New("read_string_to_output").Parse(readStringToOutputTemplateText)
	}
	if locationExpression.Opcode == ditypes.OpApplyOffset {
		return template.New("apply_offset_location_expression").Parse(applyOffsetTemplateText)
	}
	if locationExpression.Opcode == ditypes.OpPop {
		return template.New("pop_location_expression").Parse(popTemplateText)
	}
	if locationExpression.Opcode == ditypes.OpCopy {
		return template.New("copy_location_expression").Parse(copyTemplateText)
	}
	if locationExpression.Opcode == ditypes.OpLabel {
		return template.New("label").Parse(labelTemplateText)
	}
	if locationExpression.Opcode == ditypes.OpSetGlobalLimit {
		return template.New("set_limit_entry").Parse(setLimitEntryText)
	}
	if locationExpression.Opcode == ditypes.OpJumpIfGreaterThanLimit {
		return template.New("jump_if_greater_than_limit").Parse(jumpIfGreaterThanLimitText)
	}
	if locationExpression.Opcode == ditypes.OpPrintStatement {
		return template.New("print_statement").Parse(printStatementText)
	}
	if locationExpression.Opcode == ditypes.OpComment {
		return template.New("comment").Parse(commentText)
	}
	return nil, errors.New("invalid location expression opcode")
}

func cleanupTypeName(s string) string {
	return strings.TrimPrefix(s, "*")
}

func generateSliceHeader(slice *ditypes.Parameter, out io.Writer) error {
	if slice == nil {
		return errors.New("nil slice parameter when generating header code")
	}
	if len(slice.ParameterPieces) != 3 {
		return fmt.Errorf("invalid slice parameter when generating header code: %d fields", len(slice.ParameterPieces))
	}

	// Slices are defined with an "array" pointer as piece 0, which is a pointer to the actual
	// type, which is defined as piece 0 under that.
	if len(slice.ParameterPieces) != 3 &&
		len(slice.ParameterPieces[0].ParameterPieces) != 1 {
		return errors.New("malformed slice type")
	}

	typeHeaderBytes := []byte{}
	typeHeaderBuf := bytes.NewBuffer(typeHeaderBytes)
	lenHeaderBytes := []byte{}
	lenHeaderBuf := bytes.NewBuffer(lenHeaderBytes)
	lenHeaderBuf.Write([]byte("// Capture length of slice:"))
	err := generateHeaderText(slice.ParameterPieces[0].ParameterPieces[0], typeHeaderBuf)
	if err != nil {
		return err
	}
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
		return err
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
	err = generateParametersTextViaLocationExpressions([]*ditypes.Parameter{stringParam.ParameterPieces[1]}, out)
	if err != nil {
		return err
	}
	if stringParam.ParameterPieces[1] != nil {
		stringParam.ParameterPieces[1].LocationExpressions = []ditypes.LocationExpression{}
	}
	return nil
}

type sliceHeaderWrapper struct {
	Parameter           *ditypes.Parameter
	SliceTypeHeaderText string
}
