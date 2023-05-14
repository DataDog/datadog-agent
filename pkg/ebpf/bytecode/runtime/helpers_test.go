// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package runtime

import (
	"bytes"
	"strings"
	"testing"

	"github.com/DataDog/gopsutil/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestGetAvailableHelpers(t *testing.T) {
	kv, err := kernel.HostVersion()
	require.NoError(t, err)
	_, family, _, err := host.PlatformInformation()
	require.NoError(t, err)
	if kv < kernel.VersionCode(4, 10, 0) && family != "rhel" {
		t.Skip("__BPF_FUNC_MAPPER macro not available on vanilla kernels < 4.10.0")
	}

	cfg := ebpf.NewConfig()
	opts := kernel.KernelHeaderOptions{
		DownloadEnabled: cfg.EnableKernelHeaderDownload,
		Dirs:            cfg.KernelHeadersDirs,
		DownloadDir:     cfg.KernelHeadersDownloadDir,
		AptConfigDir:    cfg.AptConfigDir,
		YumReposDir:     cfg.YumReposDir,
		ZypperReposDir:  cfg.ZypperReposDir,
	}
	kernelHeaders := kernel.GetKernelHeaders(opts, nil)
	fns, err := getAvailableHelpers(kernelHeaders)
	require.NoError(t, err)
	assert.NotEmpty(t, fns, "number of available helpers")

	for _, f := range fns {
		assert.NotEqual(t, "BPF_FUNC_unspec", f)
		assert.False(t, strings.HasSuffix(f, ","))
		t.Log(f)
	}
}

func TestGenerateHelperDefines(t *testing.T) {
	allfns := []string{"BPF_FUNC_map_lookup_elem", "BPF_FUNC_map_update_elem", "BPF_FUNC_map_delete_elem"}
	availfns := []string{"BPF_FUNC_map_lookup_elem", "BPF_FUNC_map_update_elem"}
	buf := &bytes.Buffer{}
	err := generateHelperDefines(availfns, allfns, buf)
	require.NoError(t, err)

	const exp = `
#ifndef __BPF_CROSS_COMPILE_DYNAMIC__
#define __BPF_CROSS_COMPILE_DYNAMIC__

#define __E_BPF_FUNC_map_lookup_elem true
#define __E_BPF_FUNC_map_update_elem true
#define __E_BPF_FUNC_map_delete_elem false
#endif
`
	assert.Equal(t, exp, buf.String())
}
