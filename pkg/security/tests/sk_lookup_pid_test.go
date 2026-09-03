// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/nettest"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
)

func getSkStoragePidMap(t *testing.T, testModule *testModule) *ebpf.Map {
	p, ok := testModule.probe.PlatformProbe.(*probe.EBPFProbe)
	if !ok {
		t.Errorf("probe type isn't EBPF")
		return nil
	}

	m, _, err := p.Manager.GetMap("sk_storage_pid")
	if err != nil {
		t.Errorf("failed to get map sk_storage_pid: %v", err)
		return nil
	}
	return m
}

func skipIfSkLookupPidResolutionNotSupported(t *testing.T, testModule *testModule) {
	p, ok := testModule.probe.PlatformProbe.(*probe.EBPFProbe)
	if !ok {
		t.Skip("skipping non eBPF probe")
		return
	}

	if !p.IsSkLookupPidResolutionSupported() {
		t.Skip("bpf_sk_lookup based pid resolution is not supported on this host")
	}
}

// receiveFD reads a single file descriptor sent over the unix socket sock using SCM_RIGHTS.
func receiveFD(sock int) (int, error) {
	buf := make([]byte, 1)
	oob := make([]byte, syscall.CmsgSpace(4)) // room for a single int fd
	_, oobn, _, _, err := syscall.Recvmsg(sock, buf, oob, 0)
	if err != nil {
		return -1, err
	}

	scms, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return -1, err
	}
	if len(scms) == 0 {
		return -1, errors.New("no socket control message received")
	}

	fds, err := syscall.ParseUnixRights(&scms[0])
	if err != nil {
		return -1, err
	}
	if len(fds) == 0 {
		return -1, errors.New("no file descriptor received")
	}
	return fds[0], nil
}

// createSocketInChild spawns syscall_tester to create a socket of the given domain/type in a
// separate process, and receives that socket's fd back over a unix socketpair.
func createSocketInChild(t *testing.T, syscallTester, domain, typ string) (int, int) {
	t.Helper()

	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("socketpair error: %v", err)
	}
	parentEnd, childEnd := fds[0], fds[1]
	defer syscall.Close(parentEnd)

	childFile := os.NewFile(uintptr(childEnd), "sock-fd-channel")

	cmd := exec.Command(syscallTester, "create_socket_send_fd", domain, typ)
	cmd.ExtraFiles = []*os.File{childFile} // inherited as fd 3 in the child
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		childFile.Close()
		t.Fatalf("failed to start syscall_tester: %v", err)
	}
	// the child holds its own dup of childEnd, release ours
	childFile.Close()

	childPid := cmd.Process.Pid

	recvFd, err := receiveFD(parentEnd)
	if err != nil {
		_ = cmd.Wait()
		t.Fatalf("failed to receive socket fd from child: %v", err)
	}

	if err := cmd.Wait(); err != nil {
		syscall.Close(recvFd)
		t.Fatalf("syscall_tester create_socket_send_fd failed: %v", err)
	}

	return recvFd, childPid
}

func assertSkStoragePidEntry(t *testing.T, m *ebpf.Map, fd int, expectedPid uint32) {
	var pid uint32
	if err := m.Lookup(uint32(fd), &pid); err != nil {
		t.Errorf("couldn't find sk_storage_pid entry for fd %d: %v", fd, err)
		return
	}
	assert.Equal(t, expectedPid, pid, "wrong pid")
}

// TestSkLookupPidResolution checks that the cgroup socket hooks records the owning pid in the
// sk_storage_pid map.
func TestSkLookupPidResolution(t *testing.T) {
	SkipIfNotAvailable(t)

	checkNetworkCompatibility(t)

	if testEnvironment == DockerEnvironment {
		t.Skip("skipping in docker: sk_storage_pid holds root pid namespace pids, which differ from the container pid namespace")
	}

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	test, err := newTestModule(t, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	skipIfSkLookupPidResolutionNotSupported(t, test)

	m := getSkStoragePidMap(t, test)
	if m == nil {
		t.Fatal("failed to get map sk_storage_pid")
	}

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	socketTests := []struct {
		name      string
		domain    string
		typ       string
		needsIPv6 bool
	}{
		{"ipv4_udp", "ipv4", "udp", false},
		{"ipv4_tcp", "ipv4", "tcp", false},
		{"ipv6_udp", "ipv6", "udp", true},
		{"ipv6_tcp", "ipv6", "tcp", true},
	}

	for _, st := range socketTests {
		t.Run(st.name, func(t *testing.T) {
			if st.needsIPv6 && !nettest.SupportsIPv6() {
				t.Skip("IPv6 is not supported")
			}

			recvFd, childPid := createSocketInChild(t, syscallTester, st.domain, st.typ)
			defer syscall.Close(recvFd)

			assertSkStoragePidEntry(t, m, recvFd, uint32(childPid))
		})
	}
}
