// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package net

import (
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSocketExistsNewUDSListener(t *testing.T, socketPath string) {
	// Pre-create a socket
	addr, err := net.ResolveUnixAddr("unix", socketPath)
	assert.NoError(t, err)
	_, err = net.Listen("unix", addr.Name)
	assert.NoError(t, err)

	// Create a new socket using UDSListener
	l, err := NewListener(socketPath)
	require.NoError(t, err)

	l.Stop()
}

func testSocketExistsAsRegularFileNewUDSListener(t *testing.T, socketPath string) {
	// Pre-create a file
	f, err := os.OpenFile(socketPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	assert.NoError(t, err)
	defer f.Close()

	// Create a new socket using UDSListener
	_, err = NewListener(socketPath)
	require.Error(t, err)
}

func testWorkingNewUDSListener(t *testing.T, socketPath string) {
	s, err := NewListener(socketPath)
	require.NoError(t, err)
	defer s.Stop()

	assert.NoError(t, err)
	assert.NotNil(t, s)
	time.Sleep(1 * time.Second)
	fi, err := os.Stat(socketPath)
	require.NoError(t, err)
	assert.Equal(t, "Srwx-w----", fi.Mode().String())
}

func TestNewUDSListener(t *testing.T) {
	t.Run("socket_exists_but_is_successfully_removed", func(tt *testing.T) {
		dir := t.TempDir()
		testSocketExistsNewUDSListener(tt, dir+"/net.sock")
	})

	t.Run("non_socket_exists_and_fails_to_be_removed", func(tt *testing.T) {
		dir := t.TempDir()
		testSocketExistsAsRegularFileNewUDSListener(tt, dir+"/net.sock")
	})

	t.Run("working", func(tt *testing.T) {
		dir := t.TempDir()
		testWorkingNewUDSListener(tt, dir+"/net.sock")
	})
}

type fakeHandler struct {
	t       *testing.T
	request string
}

func (f *fakeHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	f.request = req.URL.String()
}

func testHttpServe(t *testing.T, shouldFailed bool, f *fakeHandler, prefixCmd []string, uid int, gid int) error {
	dir, err := os.MkdirTemp("", "testHttpServe-*")
	t.Cleanup(func() { os.RemoveAll(dir) })
	err = os.Chmod(dir, 0777)
	assert.NoError(t, err)

	socketPath := dir + "/test.http.sock"
	conn, err := net.Listen("unix", socketPath)
	assert.NoError(t, err)
	err = os.Chmod(socketPath, 0666)
	assert.NoError(t, err)

	go func() {
		time.Sleep(time.Second)
		cmd := append(prefixCmd, "curl", "-s", "--unix-socket", socketPath, "http://unix/test")
		o, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil && !shouldFailed {
			t.Log(string(o))
		}
		if !shouldFailed {
			assert.NoError(t, err)
		} else {
			assert.Error(t, err)
		}
		conn.Close()
	}()

	return HttpServe(conn, f, uid, gid)
}

func lookupUser(t *testing.T, name string) (usrID int, grpID int, usrIDstr string, grpIDstr string) {
	usr, err := user.Lookup(name)
	assert.NoError(t, err)

	usrID, err = strconv.Atoi(usr.Uid)
	assert.NoError(t, err)

	grpID, err = strconv.Atoi(usr.Gid)
	assert.NoError(t, err)

	grp, err := user.LookupGroupId(usr.Gid)
	assert.NoError(t, err)

	return usrID, grpID, usr.Username, grp.Name
}

func TestHttpServe(t *testing.T) {
	uid, gid, uidStr, gidStr := lookupUser(t, "nobody")

	// root is always valid
	f := &fakeHandler{t: t}
	err := testHttpServe(t, false, f, []string{"sudo"}, uid, gid)
	if !errors.Is(err, net.ErrClosed) && err != http.ErrServerClosed {
		assert.NoError(t, err)
	}
	assert.Equal(t, "/test", f.request)

	// nobody:nogroup is valid
	f = &fakeHandler{t: t}
	err = testHttpServe(t, false, f, []string{"sudo", "-u", uidStr, "-g", gidStr}, uid, gid)
	if !errors.Is(err, net.ErrClosed) && err != http.ErrServerClosed {
		assert.NoError(t, err)
	}
	assert.Equal(t, "/test", f.request)

	// nobody:nogroup no access
	f = &fakeHandler{t: t}
	err = testHttpServe(t, true, f, []string{"sudo", "-u", uidStr, "-g", gidStr}, 0, 0)
	if errors.Is(err, net.ErrClosed) || err == http.ErrServerClosed {
		assert.Error(t, err)
	}
	assert.Equal(t, "", f.request)
}
