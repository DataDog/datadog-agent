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

	flattenedParams := flattenParameters(procInfo.TypeMap.Functions[probe.FuncName])
	parametersText, err := generateParameterText(flattenedParams)
	if err != nil {
		return err
	}

	prog.PopulatedParameterText = parametersText

	buf := new(bytes.Buffer)
	err = programTemplate.Execute(buf, prog)
	if err != nil {
		return err
	}

	log.Println(buf.String())
	probe.InstrumentationInfo.BPFSourceCode = buf.String()

	return nil
}

func generateParameterText(paramPieces []ditypes.Parameter) (string, error) {
	var executedTemplateBytes []byte
	for i := range paramPieces {
		buf := new(bytes.Buffer)

		template, err := resolveParameterTemplate(paramPieces[i])
		if err != nil {
			return "", err
		}
		paramPieces[i].Type = cleanupTypeName(paramPieces[i].Type)
		err = template.Execute(buf, paramPieces[i])
		if err != nil {
			return "", fmt.Errorf("could not execute template for generating read of parameter: %w", err)
		}
		executedTemplateBytes = append(executedTemplateBytes, buf.Bytes()...)
	}
	return string(executedTemplateBytes), nil
}

func resolveParameterTemplate(param ditypes.Parameter) (*template.Template, error) {
	if param.Location.InReg {
		return resolveRegisterParameterTemplate(param)
	} else {
		return resolveStackParameterTemplate(param)
	}
}

func resolveRegisterParameterTemplate(param ditypes.Parameter) (*template.Template, error) {
	needsDereference := param.Location.NeedsDereference
	stringType := param.Kind == uint(reflect.String)
	sliceType := param.Kind == uint(reflect.Slice)

	if needsDereference && stringType {
		// Register String Pointer
		return template.New("string_register_pointer_template").Parse(stringPointerRegisterTemplateText)
	} else if needsDereference && sliceType {
		// Register Slice Pointer
		return template.New("slice_register_pointer_template").Parse(slicePointerRegisterTemplateText)
	} else if needsDereference {
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

func resolveStackParameterTemplate(param ditypes.Parameter) (*template.Template, error) {
	needsDereference := param.Location.NeedsDereference
	stringType := param.Kind == uint(reflect.String)
	sliceType := param.Kind == uint(reflect.Slice)

	if needsDereference && stringType {
		// Stack String Pointer
		return template.New("string_stack_pointer_template").Parse(stringPointerStackTemplateText)
	} else if needsDereference && sliceType {
		// Stack Slice Pointer
		return template.New("slice_stack_pointer_template").Parse(slicePointerStackTemplateText)
	} else if needsDereference {
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
