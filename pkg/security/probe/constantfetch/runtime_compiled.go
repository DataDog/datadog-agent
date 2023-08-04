// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && linux_bpf

package constantfetch

import (
	"bytes"
	"debug/elf"
	"fmt"
	"sort"
	"text/template"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

type rcSymbolPair struct {
	ID        string
	Operation string
}

// RuntimeCompilationConstantFetcher is a constant fetcher utilizing runtime compilation
type RuntimeCompilationConstantFetcher struct {
	config       *ebpf.Config
	statsdClient statsd.ClientInterface
	headers      []string
	symbolPairs  []rcSymbolPair
	result       map[string]uint64
}

// NewRuntimeCompilationConstantFetcher creates a RuntimeCompilationConstantFetcher
func NewRuntimeCompilationConstantFetcher(config *ebpf.Config, statsdClient statsd.ClientInterface) *RuntimeCompilationConstantFetcher {
	return &RuntimeCompilationConstantFetcher{
		config:       config,
		statsdClient: statsdClient,
		result:       make(map[string]uint64),
	}
}

func (cf *RuntimeCompilationConstantFetcher) String() string {
	return "runtime-compilation"
}

// AppendSizeofRequest appends a sizeof request
func (cf *RuntimeCompilationConstantFetcher) AppendSizeofRequest(id, typeName, headerName string) {
	cf.result[id] = ErrorSentinel
	if headerName == "" {
		return
	}

	cf.headers = append(cf.headers, headerName)
	cf.symbolPairs = append(cf.symbolPairs, rcSymbolPair{
		ID:        id,
		Operation: fmt.Sprintf("sizeof(%s)", typeName),
	})
}

// AppendOffsetofRequest appends an offset request
func (cf *RuntimeCompilationConstantFetcher) AppendOffsetofRequest(id, typeName, fieldName, headerName string) {
	cf.result[id] = ErrorSentinel
	if headerName == "" {
		return
	}

	cf.headers = append(cf.headers, headerName)
	cf.symbolPairs = append(cf.symbolPairs, rcSymbolPair{
		ID:        id,
		Operation: fmt.Sprintf("offsetof(%s, %s)", typeName, fieldName),
	})
}

const runtimeCompilationTemplate = `
#include <linux/kconfig.h>
#ifdef CONFIG_HAVE_ARCH_COMPILER_H
#include <asm/compiler.h>
#endif
{{ range .headers }}
#include <{{ . }}>
{{ end }}

{{ range .symbols }}
size_t {{.ID}} = {{.Operation}};
{{ end }}
`

func (cf *RuntimeCompilationConstantFetcher) getCCode() (string, error) {
	headers := sortAndDedup(cf.headers)
	tmpl, err := template.New("runtimeCompilationTemplate").Parse(runtimeCompilationTemplate)
	if err != nil {
		return "", err
	}

	var buffer bytes.Buffer
	if err := tmpl.Execute(&buffer, map[string]interface{}{
		"headers": headers,
		"symbols": cf.symbolPairs,
	}); err != nil {
		return "", err
	}

	return buffer.String(), nil
}

// FinishAndGetResults returns the results
func (cf *RuntimeCompilationConstantFetcher) FinishAndGetResults() (map[string]uint64, error) {
	cCode, err := cf.getCCode()
	if err != nil {
		return nil, err
	}

	elfFile, err := runtime.ConstantFetcher.Compile(cf.config, cCode, nil, cf.statsdClient)
	if err != nil {
		return nil, err
	}

	f, err := elf.NewFile(elfFile)
	if err != nil {
		return nil, err
	}

	symbols, err := f.Symbols()
	if err != nil {
		return nil, err
	}
	for _, sym := range symbols {
		if _, present := cf.result[sym.Name]; !present {
			continue
		}

		section := f.Sections[sym.Section]
		buf := make([]byte, sym.Size)
		if _, err := section.ReadAt(buf, int64(sym.Value)); err != nil {
			return nil, fmt.Errorf("unable to read section at %d: %s", int64(sym.Value), err)
		}

		var value uint64
		switch sym.Size {
		case 4:
			value = uint64(f.ByteOrder.Uint32(buf))
		case 8:
			value = f.ByteOrder.Uint64(buf)
		default:
			return nil, fmt.Errorf("unexpected symbol size: `%v`", sym.Size)
		}

		cf.result[sym.Name] = value
	}

	seclog.Infof("runtime compiled constants: %v", cf.result)
	return cf.result, nil
}

func sortAndDedup(in []string) []string {
	// sort and dedup headers
	set := make(map[string]bool)
	for _, value := range in {
		set[value] = true
	}

	out := make([]string, 0, len(in))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
