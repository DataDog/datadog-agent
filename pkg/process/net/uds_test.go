// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package net

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/require"
)

func testSocketExistsNewUDSListener(t *testing.T, socketPath string) {
	// Pre-create a socket
	addr, err := net.ResolveUnixAddr("unix", socketPath)
	require.NoError(t, err)
	_, err = net.Listen("unix", addr.Name)
	require.NoError(t, err)

	// Create a new socket using UDSListener
	l, err := NewListener(socketPath)
	require.NoError(t, err)

	l.Stop()
}

func testSocketExistsAsRegularFileNewUDSListener(t *testing.T, socketPath string) {
	// Pre-create a file
	f, err := os.OpenFile(socketPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	require.NoError(t, err)
	defer f.Close()

	// Create a new socket using UDSListener
	_, err = NewListener(socketPath)
	require.Error(t, err)
}

func testWorkingNewUDSListener(t *testing.T, socketPath string) {
	s, err := NewListener(socketPath)
	require.NoError(t, err)
	defer s.Stop()

	require.NoError(t, err)
	require.NotNil(t, s)
	time.Sleep(1 * time.Second)
	fi, err := os.Stat(socketPath)
	require.NoError(t, err)
	require.Equal(t, "Srwx-w----", fi.Mode().String())
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
	w.WriteHeader(200)
}

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

func almostACopyOfHttpServe(l net.Listener, handler http.Handler, authSocket bool, sig string) error {
	srv := &http.Server{Handler: handler}
	if authSocket {
		srv.ConnContext = func(ctx context.Context, c net.Conn) context.Context {
			var unixConn *net.UnixConn
			var ok bool
			if unixConn, ok = c.(*net.UnixConn); !ok {
				return ctx
			}
			valid, err := IsUnixNetConnValid(unixConn, sig)
			if err != nil || !valid {
				if err != nil {
					log.Errorf("unix socket %s -> %s closing connection, error %s", unixConn.LocalAddr(), unixConn.RemoteAddr(), err)
				} else if !valid {
					log.Errorf("unix socket %s -> %s closing connection, rejected. Client accessing this socket require to be a signed binary", unixConn.LocalAddr(), unixConn.RemoteAddr())
				}
				// reject the connection
				newCtx, cancelCtx := context.WithCancel(ctx)
				ctx = newCtx
				cancelCtx()
				c.Close()
			}
			return ctx
		}
	}
	return srv.Serve(l)
}

func testHttpServe(t *testing.T, shouldFailed bool, f *fakeHandler, prefixCmd []string, auth bool, sig string) (err error) {
	dir := t.TempDir()
	err = os.Chmod(dir, 0777)
	require.NoError(t, err)
	err = os.Chmod(dir+"/..", 0777)
	require.NoError(t, err)

	socketPath := dir + "/test.http.sock"
	conn, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	err = os.Chmod(socketPath, 0666)
	require.NoError(t, err)

	go func() {
		time.Sleep(500 * time.Millisecond)
		cmd := append(prefixCmd, "curl", "-s", "--unix-socket", socketPath, "http://unix/test")
		o, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		conn.Close() // closing the server
		if !shouldFailed {
			if err != nil {
				t.Log(cmd, string(o))
			}
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}()

	return almostACopyOfHttpServe(conn, f, auth, sig)
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
		t.Skipf("sudo is not installed or interactive")
	}
}

func TestHttpServeAuth(t *testing.T) {
	checkIfHostProc(t)
	checkIfSudoExistAndNotInteractive(t)

	curl, err := exec.LookPath("curl")
	require.NoError(t, err)
	curlSig := sigFile(curl)
	uidStr, gidStr := lookupUser(t, "nobody")

	auth := true
	t.Run("root is valid", func(t *testing.T) {
		f := &fakeHandler{t: t}
		err := testHttpServe(t, false, f, []string{"sudo"}, auth, curlSig)
		if !errors.Is(err, net.ErrClosed) && err != http.ErrServerClosed {
			require.NoError(t, err)
		}
		require.Equal(t, "/test", f.request)
	})
	t.Run("nobody:nogroup is valid", func(t *testing.T) {
		f := &fakeHandler{t: t}
		err := testHttpServe(t, false, f, []string{"sudo", "-u", uidStr, "-g", gidStr}, auth, curlSig)
		if !errors.Is(err, net.ErrClosed) && err != http.ErrServerClosed {
			require.NoError(t, err)
		}
		require.Equal(t, "/test", f.request)
	})
	t.Run("root no access", func(t *testing.T) {
		f := &fakeHandler{t: t}
		err := testHttpServe(t, true, f, []string{"sudo"}, auth, "bad sig")
		if errors.Is(err, net.ErrClosed) || err == http.ErrServerClosed {
			require.Error(t, err)
		}
		require.Equal(t, "", f.request)
	})
	t.Run("nobody:nogroup no access", func(t *testing.T) {
		f := &fakeHandler{t: t}
		err := testHttpServe(t, true, f, []string{"sudo", "-u", uidStr, "-g", gidStr}, auth, "bad sig")
		if errors.Is(err, net.ErrClosed) || err == http.ErrServerClosed {
			require.Error(t, err)
		}
		require.Equal(t, "", f.request)
	})
}

func TestHttpServeNoAuth(t *testing.T) {
	checkIfSudoExistAndNotInteractive(t)

	uidStr, gidStr := lookupUser(t, "nobody")

	auth := false
	t.Run("root is valid (auth disabled)", func(t *testing.T) {
		f := &fakeHandler{t: t}
		err := testHttpServe(t, false, f, []string{"sudo"}, auth, "always access")
		if !errors.Is(err, net.ErrClosed) && err != http.ErrServerClosed {
			require.NoError(t, err)
		}
		require.Equal(t, "/test", f.request)
	})
	t.Run("nobody:nogroup is valid (auth disabled)", func(t *testing.T) {
		f := &fakeHandler{t: t}
		err := testHttpServe(t, false, f, []string{"sudo", "-u", uidStr, "-g", gidStr}, auth, "always access")
		if !errors.Is(err, net.ErrClosed) && err != http.ErrServerClosed {
			require.NoError(t, err)
		}
		require.Equal(t, "/test", f.request)
	})
}
