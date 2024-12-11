// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package runtime

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"

	"github.com/DataDog/datadog-agent/pkg/ebpf/compiler"
)

const listGenSource = `
#include <uapi/linux/bpf.h>

#define __BPF_ENUM_FN(x) BPF_FUNC_ ## x
__BPF_FUNC_MAPPER(__BPF_ENUM_FN)
#undef __BPF_ENUM_FN
`

const defineTemplate = `
#ifndef __BPF_CROSS_COMPILE_DYNAMIC__
#define __BPF_CROSS_COMPILE_DYNAMIC__

{{ range $index, $fn := .AllFuncs -}}
#define __E_{{ $fn }} {{ mapexists $fn $.AvailableFuncs }}
{{ end -}}
#endif
`

func includeHelperAvailability(kernelHeaders []string) (string, error) {
	fns, err := getAvailableHelpers(kernelHeaders)
	if err != nil {
		return "", fmt.Errorf("get available helpers: %w", err)
	}

	f, err := os.CreateTemp(os.TempDir(), "bpf_cross_compile_dynamic.*.h")
	if err != nil {
		return "", fmt.Errorf("temp define file: %w", err)
	}
	defer f.Close()

	if err := generateHelperDefines(fns, allHelpers, f); err != nil {
		return "", fmt.Errorf("generate helper defines: %w", err)
	}
	return f.Name(), nil
}

type helperData struct {
	AllFuncs       []string
	AvailableFuncs map[string]struct{}
}

func generateHelperDefines(availableFns []string, allFns []string, out io.Writer) error {
	tmpl, err := template.New("helperexist").Funcs(map[string]any{
		"mapexists": func(k string, m map[string]struct{}) bool {
			_, ok := m[k]
			return ok
		},
	}).Parse(defineTemplate)
	if err != nil {
		return fmt.Errorf("helper define template parse: %w", err)
	}

	data := &helperData{
		AllFuncs:       allFns,
		AvailableFuncs: make(map[string]struct{}),
	}

	for _, f := range availableFns {
		data.AvailableFuncs[f] = struct{}{}
	}
	return tmpl.Execute(out, data)
}

func getAvailableHelpers(kernelHeaders []string) ([]string, error) {
	clangOut := &bytes.Buffer{}
	if err := compiler.Preprocess(bytes.NewBufferString(listGenSource), clangOut, nil, kernelHeaders); err != nil {
		return nil, fmt.Errorf("preprocess helpers: %w", err)
	}

	var lastLine []byte
	scanner := bufio.NewScanner(clangOut)
	for scanner.Scan() {
		lastLine = scanner.Bytes()
	}
	if len(lastLine) == 0 {
		return nil, fmt.Errorf("empty output")
	}

	funcs := strings.Split(string(lastLine), ", ")
	if len(funcs) < 2 || funcs[0] != "BPF_FUNC_unspec" {
		return nil, fmt.Errorf("invalid preprocess output: %s", lastLine)
	}

	// remove BPF_FUNC_unspec
	funcs = funcs[1:]
	funcs[len(funcs)-1] = strings.TrimSuffix(funcs[len(funcs)-1], ",")
	return funcs, nil
}
