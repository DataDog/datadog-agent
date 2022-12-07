// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/ebpf/compiler"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var listGenSource = `
#include <uapi/linux/bpf.h>

#define __BPF_ENUM_FN(x) bpf_ ## x
__BPF_FUNC_MAPPER(__BPF_ENUM_FN)
#undef __BPF_ENUM_FN
`

func getAvailableHelpers(cfg *Config, client statsd.ClientInterface) ([]string, error) {
	opts := kernel.KernelHeaderOptions{
		DownloadEnabled: cfg.EnableKernelHeaderDownload,
		Dirs:            cfg.KernelHeadersDirs,
		DownloadDir:     cfg.KernelHeadersDownloadDir,
		AptConfigDir:    cfg.AptConfigDir,
		YumReposDir:     cfg.YumReposDir,
		ZypperReposDir:  cfg.ZypperReposDir,
	}
	kernelHeaders := kernel.GetKernelHeaders(opts, client)
	clangOut := &bytes.Buffer{}
	if err := compiler.Preprocess(bytes.NewBufferString(listGenSource), clangOut, nil, kernelHeaders); err != nil {
		return nil, err
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
	// remove bpf_unspec
	funcs = funcs[1:]
	funcs[len(funcs)-1] = strings.TrimSuffix(funcs[len(funcs)-1], ",")
	return funcs, nil
}
