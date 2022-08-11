// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package mapper

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const compilationStepTimeout = 15 * time.Second

var sockToPidProg = `
#define SEC(NAME) __attribute__((section(NAME), used))
#define BUF_SIZE_MAP_NS 256

struct bpf_map_def {
    unsigned int type;
    unsigned int key_size;
    unsigned int value_size;
    unsigned int max_entries;
    unsigned int map_flags;
    unsigned int pinning;
    char namespace[BUF_SIZE_MAP_NS];
};

struct bpf_map_def SEC("maps/sock_to_pid") sock_to_pid = {
	.type = 1,
	.key_size = sizeof(unsigned long),
	.value_size = sizeof(int),
	.pinning = 0,
	.namespace = "",
};
`

var (
	datadogAgentEmbeddedPath = "/opt/datadog-agent/embedded"
	clangBinPath             = filepath.Join(datadogAgentEmbeddedPath, "bin/clang-bpf")
	llcBinPath               = filepath.Join(datadogAgentEmbeddedPath, "bin/llc-bpf")
)

func startHTTPServer(t *testing.T) func() {
	return testutil.HTTPServer(t, "127.0.0.1:443", testutil.Options{
		EnableTLS: false,
	})
}

func TestPidMapper(t *testing.T) {
	serverDone := startHTTPServer(t)
	defer serverDone()

	cfg := config.New()
	cfg.EnableRuntimeCompiler = true
	cfg.MaxTrackedConnections = 1024

	sockPidMap, err := initializeSockToPidMap(t, cfg)
	require.NoError(t, err)

	pidMapper, err := NewPidMapper(cfg, sockPidMap)
	require.NoError(t, err)
	defer pidMapper.Stop()

	inodes, err := getAllTCPInodes("/proc/net/tcp")
	require.NoError(t, err)

	cmap, ok, err := pidMapper.ebpfProgram.GetMap(inodePidMap)
	require.NoError(t, err)
	assert.True(t, ok)
	var pid uint32
	for _, inode := range inodes {
		err = cmap.Lookup(inode, &pid)
		require.NoError(t, err)

		err = validateInodePidMapping(inode, pid)
		require.NoError(t, err)
		t.Logf("Validated: %d -> %d\n", inode, pid)
	}
}

func validateInodePidMapping(validateInode uint64, pid uint32) error {
	procRoot := util.HostProc()
	fdpath := filepath.Join(procRoot, fmt.Sprintf("%d", pid), "fd")

	d, err := os.Open(fdpath)
	if err != nil {
		return err
	}

	fnames, err := d.Readdirnames(-1)
	if err != nil {
		return err
	}

	for _, fname := range fnames {
		inodePath := filepath.Join(fdpath, fname)
		inode, err := os.Readlink(inodePath)
		if err != nil {
			return err
		}

		if !strings.HasPrefix(inode, "socket:[") {
			continue
		}

		inodeNum, err := strconv.ParseUint(inode[len("socket[:"):len(inode)-1], 10, 64)
		if err != nil {
			return err
		}

		if validateInode == inodeNum {
			return nil
		}
	}

	return fmt.Errorf("Could not find inode: %d, for pid: %d", validateInode, pid)
}

func compileTestProg(r io.Reader, outputdir string) error {
	clangCtx, clangCancel := context.WithTimeout(context.Background(), compilationStepTimeout)
	defer clangCancel()

	compile := exec.CommandContext(clangCtx, clangBinPath, "-O2", "-target", "bpf", "-x", "c", "-c", "-o", "/tmp/testprog.o", "-")

	var clangOut, clangErr bytes.Buffer
	compile.Stdin = r
	compile.Stdout = &clangOut
	compile.Stderr = &clangErr

	err := compile.Run()
	if err != nil {
		return err
	}

	return nil
}

func initializeSockToPidMap(t *testing.T, cfg *config.Config) (*ebpf.Map, error) {
	r := strings.NewReader(sockToPidProg)
	err := compileTestProg(r, "/tmp/testprog.o")
	require.NoError(t, err)

	bc, err := bytecode.GetReader("/tmp", "testprog.o")
	require.NoError(t, err)

	mgr := &manager.Manager{
		Maps: []*manager.Map{
			{Name: string(probes.SockToPidMap)},
		},
	}

	err = mgr.InitWithOptions(bc, manager.Options{
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
		MapSpecEditors: map[string]manager.MapSpecEditor{
			string(probes.SockToPidMap): {Type: ebpf.Hash, MaxEntries: uint32(cfg.MaxTrackedConnections), EditorFlag: manager.EditMaxEntries},
		},
	})

	if err != nil {
		return nil, err
	}

	m, ok, err := mgr.GetMap(string(probes.SockToPidMap))
	if !ok {
		fmt.Errorf("could not get map %s", string(probes.SockToPidMap))
	}
	if err != nil {
		return nil, err
	}

	return m, nil
}

func getAllTCPInodes(nettcp string) ([]uint64, error) {
	var inodes []uint64
	f, err := os.Open(nettcp)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		if fields[0] == "sl" {
			continue
		}
		inode, err := strconv.ParseUint(fields[9], 10, 64)
		if err != nil {
			continue
		}
		if inode == 0 {
			continue
		}

		inodes = append(inodes, inode)
	}

	return inodes, nil
}
