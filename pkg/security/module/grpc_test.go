// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package module

import (
	"context"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func installGoGRPCClient(t *testing.T) string {
	// grpcurl is installed by inv -e install-tools
	t.Helper()
	gp := os.Getenv("GOPATH")
	if gp == "" {
		gp = os.Getenv("HOME") + "/go"
	}
	binPath := gp + "/bin/grpcurl"
	tmpDir := t.TempDir()
	err := os.Chmod(tmpDir, 0777)
	require.NoError(t, err)
	binPathDst := tmpDir + "/grpcurl"
	d, err := ioutil.ReadFile(binPath)
	require.NoError(t, err)
	err = ioutil.WriteFile(binPathDst, d, 0755)
	require.NoError(t, err)
	return binPathDst
}

func testGRPCServe(t *testing.T, shouldFailed bool, prefixCmd []string, auth bool, uid int, gid int) {
	dir := t.TempDir()
	err := os.Chmod(dir, 0777)
	require.NoError(t, err)
	err = os.Chmod(dir+"/..", 0777)
	require.NoError(t, err)

	socketPath := dir + "/test.http.sock"
	var grpcOpts []grpc.ServerOption
	if auth {
		grpcOpts = append(grpcOpts, GRPCWithCredOptions(uid, gid))
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

	grpcurl := installGoGRPCClient(t)
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

func lookupUser(t *testing.T, name string) (usrID int, grpID int, usrIDstr string, grpIDstr string) {
	usr, err := user.Lookup(name)
	require.NoError(t, err)

	usrID, err = strconv.Atoi(usr.Uid)
	require.NoError(t, err)

	grpID, err = strconv.Atoi(usr.Gid)
	require.NoError(t, err)

	grp, err := user.LookupGroupId(usr.Gid)
	require.NoError(t, err)

	return usrID, grpID, usr.Username, grp.Name
}

func checkIfSudoExistAndNotInteractive(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
	defer cancel()
	if err := exec.CommandContext(ctx, "sudo", "id").Run(); err != nil {
		t.Skipf("sudo is not installed or interactive %s", err)
	}
}

func TestGRPCServerAuth(t *testing.T) {
	checkIfSudoExistAndNotInteractive(t)

	auth := true
	uid, gid, uidStr, gidStr := lookupUser(t, "nobody")

	t.Run("root always valid", func(t *testing.T) {
		testGRPCServe(t, false, []string{"sudo"}, auth, uid, gid)
	})
	t.Run("nobody:nogroup is valid", func(t *testing.T) {
		testGRPCServe(t, false, []string{"sudo", "-u", uidStr, "-g", gidStr}, auth, uid, gid)
	})
	t.Run("nobody:nogroup no access", func(t *testing.T) {
		testGRPCServe(t, true, []string{"sudo", "-u", uidStr, "-g", gidStr}, auth, 0, 0)
	})

	auth = false
	t.Run("root always valid auth socket disabled", func(t *testing.T) {
		testGRPCServe(t, false, []string{"sudo"}, auth, uid, gid)
	})
	t.Run("nobody:nogroup access auth socket disabled", func(t *testing.T) {
		testGRPCServe(t, false, []string{"sudo", "-u", uidStr, "-g", gidStr}, auth, 0, 0)
	})
}
