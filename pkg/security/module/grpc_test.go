// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package module

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"os/user"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func sigFile(fpath string) string {
	f, err := os.Open(fpath)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func installGoGRPCClient(t *testing.T) (string, string) {
	// grpcurl is installed by inv -e install-tools
	t.Helper()
	gp := os.Getenv("GOPATH")
	if gp == "" {
		gp = os.Getenv("HOME") + "/go"
	}
	binPath := gp + "/bin/grpcurl"

	// if failed it's probably because test is executed via go test -exec sudo but grpcurl is unavailable in root gopath
	_, err := exec.LookPath(binPath)
	if err != nil {
		exec.Command("go", "install", "github.com/fullstorydev/grpcurl/cmd/grpcurl@latest").Run()
	}

	tmpDir, err := os.MkdirTemp("", "testgrpcurl-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})
	err = os.Chmod(tmpDir, 0777)
	require.NoError(t, err)
	binPathDst := tmpDir + "/grpcurl"
	d, err := ioutil.ReadFile(binPath)
	require.NoError(t, err)
	err = ioutil.WriteFile(binPathDst, d, 0755)
	require.NoError(t, err)
	return binPathDst, sigFile(binPathDst)
}

func testGRPCServe(t *testing.T, shouldFailed bool, grpcurl string, prefixCmd []string, auth bool, sig string) {
	dir := t.TempDir()
	err := os.Chmod(dir, 0777)
	require.NoError(t, err)
	err = os.Chmod(dir+"/..", 0777)
	require.NoError(t, err)

	socketPath := dir + "/test.http.sock"
	var grpcOpts []grpc.ServerOption
	if auth {
		grpcOpts = append(grpcOpts, GRPCWithCredOptions(sig))
	}
	server := NewGRPCServer(socketPath, grpcOpts...)
	reflection.Register(server.server) // enable services reflection for grpcurl list
	err = server.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})
	err = os.Chmod(socketPath, 0777)
	require.NoError(t, err)

	// wait grpc server to start
	require.Eventually(t, func() bool {
		d := net.Dialer{Timeout: 200 * time.Millisecond}
		c, err := d.Dial("unix", socketPath)
		if err == nil {
			c.Close()
		}
		return err == nil
	}, 5*time.Second, 200*time.Millisecond, "couldn't start GRPC server")

	cmd := append(prefixCmd, grpcurl, "-plaintext", "-unix", socketPath, "list")
	o, errClient := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
	if !shouldFailed {
		if errClient != nil {
			t.Log(cmd, string(o))
		}
		require.NoError(t, errClient)
	} else {
		require.Error(t, errClient)
	}
}

func lookupUser(t *testing.T, name string) (usrIDstr string, grpIDstr string) {
	usr, err := user.Lookup(name)
	require.NoError(t, err)

	grp, err := user.LookupGroupId(usr.Gid)
	require.NoError(t, err)

	return usr.Username, grp.Name
}

func checkIfHostProc(t *testing.T) {
	if os.Getenv("HOST_PROC") == "" && os.Geteuid() == 0 {
		t.Skipf("this test need to be run as root with HOST_PROC as we need to scan sudo -u nobody -g nogroup /proc/pid/exe content")
	}
}

func checkIfSudoExistAndNotInteractive(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
	defer cancel()
	if err := exec.CommandContext(ctx, "sudo", "id").Run(); err != nil {
		t.Skipf("sudo is not installed or interactive %s", err)
	}
}

func TestGRPCServerAuth(t *testing.T) {
	checkIfHostProc(t)
	checkIfSudoExistAndNotInteractive(t)

	grpcurl, sigGrpcurl := installGoGRPCClient(t)

	auth := true
	uidStr, gidStr := lookupUser(t, "nobody")

	t.Run("root is valid", func(t *testing.T) {
		testGRPCServe(t, false, grpcurl, []string{}, auth, sigGrpcurl)
	})
	t.Run("root no access", func(t *testing.T) {
		testGRPCServe(t, true, grpcurl, []string{}, auth, "bad sig")
	})
	t.Run("sudo is valid", func(t *testing.T) {
		testGRPCServe(t, false, grpcurl, []string{"sudo"}, auth, sigGrpcurl)
	})
	t.Run("sudo no access", func(t *testing.T) {
		testGRPCServe(t, true, grpcurl, []string{"sudo"}, auth, "bad sig")
	})
	t.Run("nobody:nogroup is valid", func(t *testing.T) {
		testGRPCServe(t, false, grpcurl, []string{"sudo", "-u", uidStr, "-g", gidStr}, auth, sigGrpcurl)
	})
	t.Run("nobody:nogroup no access", func(t *testing.T) {
		testGRPCServe(t, true, grpcurl, []string{"sudo", "-u", uidStr, "-g", gidStr}, auth, "bad sig")
	})
}

func TestGRPCServerNoAuth(t *testing.T) {
	checkIfSudoExistAndNotInteractive(t)

	grpcurl, _ := installGoGRPCClient(t)
	uidStr, gidStr := lookupUser(t, "nobody")

	auth := false
	t.Run("root always valid auth socket disabled", func(t *testing.T) {
		testGRPCServe(t, false, grpcurl, []string{"sudo"}, auth, "bad sig must be ok")
	})
	t.Run("nobody:nogroup access auth socket disabled", func(t *testing.T) {
		testGRPCServe(t, false, grpcurl, []string{"sudo", "-u", uidStr, "-g", gidStr}, auth, "bad sig must be ok")
	})
}
