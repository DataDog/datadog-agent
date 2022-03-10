// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && linux_bpf
// +build linux,linux_bpf

package constantfetch

import (
	"bytes"
	"debug/elf"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-go/statsd"
	"golang.org/x/sys/unix"
)

type rcSymbolPair struct {
	Id        string
	Operation string
}

type RuntimeCompilationConstantFetcher struct {
	config       *ebpf.Config
	statsdClient *statsd.Client
	headers      []string
	symbolPairs  []rcSymbolPair
	result       map[string]uint64
}

func NewRuntimeCompilationConstantFetcher(config *ebpf.Config, statsdClient *statsd.Client) *RuntimeCompilationConstantFetcher {
	return &RuntimeCompilationConstantFetcher{
		config:       config,
		statsdClient: statsdClient,
		result:       make(map[string]uint64),
	}
}

func (cf *RuntimeCompilationConstantFetcher) AppendSizeofRequest(id, typeName, headerName string) {
	if headerName != "" {
		cf.headers = append(cf.headers, headerName)
	}

	cf.symbolPairs = append(cf.symbolPairs, rcSymbolPair{
		Id:        id,
		Operation: fmt.Sprintf("sizeof(%s)", typeName),
	})
	cf.result[id] = ErrorSentinel
}

func (cf *RuntimeCompilationConstantFetcher) AppendOffsetofRequest(id, typeName, fieldName, headerName string) {
	if headerName != "" {
		cf.headers = append(cf.headers, headerName)
	}

	cf.symbolPairs = append(cf.symbolPairs, rcSymbolPair{
		Id:        id,
		Operation: fmt.Sprintf("offsetof(%s, %s)", typeName, fieldName),
	})
	cf.result[id] = ErrorSentinel
}

const runtimeCompilationTemplate = `
#include <linux/kconfig.h>
#include <asm/compiler.h>
{{ range .headers }}
#include <{{ . }}>
{{ end }}

{{ range .symbols }}
size_t {{.Id}} = {{.Operation}};
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

func (cf *RuntimeCompilationConstantFetcher) compileConstantFetcher(config *ebpf.Config, cCode string) (io.ReaderAt, error) {
	provider := &constantFetcherRCProvider{
		cCode: cCode,
	}
	runtimeCompiler := runtime.NewRuntimeCompiler()
	reader, err := runtimeCompiler.CompileObjectFile(config, additionalFlags, "constant_fetcher.c", provider)

	if cf.statsdClient != nil {
		telemetry := runtimeCompiler.GetRCTelemetry()
		if err := telemetry.SendMetrics(cf.statsdClient); err != nil {
			log.Errorf("failed to send telemetry for runtime compilation of constants: %v", err)
		}
	}

	return reader, err
}

func (cf *RuntimeCompilationConstantFetcher) FinishAndGetResults() (map[string]uint64, error) {
	cCode, err := cf.getCCode()
	if err != nil {
		return nil, err
	}

	elfFile, err := cf.compileConstantFetcher(cf.config, cCode)
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
		section.ReadAt(buf, int64(sym.Value))

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

	log.Infof("runtime compiled constants: %v", cf.result)
	return cf.result, nil
}

var additionalFlags = []string{
	"-D__KERNEL__",
	"-fno-stack-protector",
	"-fno-color-diagnostics",
	"-fno-unwind-tables",
	"-fno-asynchronous-unwind-tables",
	"-fno-jump-tables",
}

type constantFetcherRCProvider struct {
	cCode string
}

func (p *constantFetcherRCProvider) GetInputReader(config *ebpf.Config, tm *runtime.RuntimeCompilationTelemetry) (io.Reader, error) {
	return strings.NewReader(p.cCode), nil
}

func (a *constantFetcherRCProvider) GetOutputFilePath(config *ebpf.Config, uname *unix.Utsname, flagHash string, tm *runtime.RuntimeCompilationTelemetry) (string, error) {
	cCodeHash, err := runtime.Sha256hex([]byte(a.cCode))
	if err != nil {
		return "", err
	}

	unameHash, err := runtime.UnameHash(uname)
	if err != nil {
		return "", err
	}

	return filepath.Join(config.RuntimeCompilerOutputDir, fmt.Sprintf("constant_fetcher-%s-%s-%s.o", unameHash, cCodeHash, flagHash)), nil
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
