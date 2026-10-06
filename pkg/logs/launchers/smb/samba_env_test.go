// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && smb && !goexperiment.systemcrypto && !goexperiment.boringcrypto && !requirefips

package smb

// This file runs the Samba server of the integration tests in
// samba_integration_test.go, and the log writer that writes to it.

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/client"
	smb2 "github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/auth"
)

const (
	// sambaIntegrationEnv opts in to the tests that need Docker, as for the
	// other integration tests that `dda inv integration-tests` runs.
	sambaIntegrationEnv = "INTEGRATION"

	sambaUser       = "smbtest"
	sambaShare      = "logs"
	sambaImageRepo  = "dd-agent-smb-it-samba"
	sambaContextDir = "testdata/samba"
	sambaLabel      = "com.datadoghq.agent.smb-integration-test"

	// redactedPassword replaces the passwords in everything the tests print.
	redactedPassword = "********"

	// Bounds of the writer's own SMB calls, and how long it retries one
	// write through an outage.
	writerOpTimeout = 10 * time.Second
	writerRetryFor  = 2 * time.Minute
)

// requireSambaIntegration skips the test unless the integration tests were
// asked for and a Docker daemon running Linux containers answers.
func requireSambaIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv(sambaIntegrationEnv) == "" {
		t.Skipf("Samba integration test: set %s=1 to run it against a Samba server in Docker", sambaIntegrationEnv)
	}
	if runtime.GOOS == "windows" {
		t.Skip("Samba integration test: needs a Docker daemon running Linux containers, not available on Windows runners")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("Samba integration test: Docker is unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Os}}").CombinedOutput()
	if err != nil {
		t.Skipf("Samba integration test: Docker is unavailable (docker version: %v): %s", err, strings.TrimSpace(string(out)))
	}
	if serverOS := strings.TrimSpace(string(out)); serverOS != "linux" {
		t.Skipf("Samba integration test: the Docker daemon runs %q containers, the Samba image needs linux", serverOS)
	}
}

// sambaEnv is a Samba server in a Docker container, sharing sambaShare to
// sambaUser with a random password. Every password it ever had is redacted
// from what the tests print.
type sambaEnv struct {
	t         *testing.T
	container string
	host      string // address the clients dial
	port      int

	mu       sync.Mutex
	password string
	secrets  []string
}

// startSamba builds the image of testdata/samba if needed, runs it, sets a
// random password and waits until the SMB client can connect.
func startSamba(t *testing.T) *sambaEnv {
	t.Helper()
	image := buildSambaImage(t)
	host, bind := dockerPublishAddress(t)
	port := freePort(t)
	env := &sambaEnv{
		t:         t,
		container: "dd-agent-smb-it-" + randomHex(t, 4),
		host:      host,
		port:      port,
	}
	env.mustDocker(time.Minute, "", "run", "--detach",
		"--name", env.container,
		"--label", sambaLabel+"=1",
		"--publish", fmt.Sprintf("%s:%d:445", bind, port),
		image)
	t.Cleanup(func() {
		if t.Failed() {
			logs, _ := dockerCmd(30*time.Second, "", "logs", "--tail", "200", env.container)
			t.Logf("Samba container logs:\n%s", env.redact(logs))
		}
		if out, err := dockerCmd(time.Minute, "", "rm", "--force", "--volumes", env.container); err != nil {
			t.Logf("cannot remove the Samba container %s: %v: %s", env.container, err, out)
		}
	})
	env.setPassword(newPassword(t))
	env.waitReady(time.Minute)
	version, _ := dockerCmd(30*time.Second, "", "exec", env.container, "smbd", "--version")
	t.Logf("Samba %s ready at smb://%s:%d/%s (container %s, image %s)", strings.TrimSpace(version), host, port, sambaShare, env.container, image)
	return env
}

// buildSambaImage builds testdata/samba under a tag derived from its content,
// so an unchanged image is built once and then reused from the local cache.
func buildSambaImage(t *testing.T) string {
	t.Helper()
	// The build context is copied: under Bazel, testdata holds symlinks to
	// the source tree, which docker build does not follow.
	buildContext := t.TempDir()
	sum := sha256.New()
	for _, name := range []string{"Dockerfile", "smb.conf"} {
		content, err := os.ReadFile(filepath.Join(sambaContextDir, name))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(buildContext, name), content, 0o644))
		fmt.Fprintf(sum, "%s %d\n", name, len(content))
		sum.Write(content)
	}
	image := sambaImageRepo + ":" + hex.EncodeToString(sum.Sum(nil))[:16]
	if _, err := dockerCmd(30*time.Second, "", "image", "inspect", image); err == nil {
		return image
	}
	t.Logf("building the Samba test image %s", image)
	out, err := dockerCmd(10*time.Minute, "", "build", "--tag", image, buildContext)
	require.NoError(t, err, "docker build of %s failed:\n%s", sambaContextDir, out)
	return image
}

// dockerPublishAddress returns the address the clients dial to reach a
// published port, and the address to publish it on: the loopback for a local
// daemon, or the daemon's host for a remote one (DOCKER_HOST=tcp://...), as
// with Docker-in-Docker CI runners.
func dockerPublishAddress(t *testing.T) (host, bind string) {
	t.Helper()
	if dockerHost := os.Getenv("DOCKER_HOST"); strings.HasPrefix(dockerHost, "tcp://") {
		u, err := url.Parse(dockerHost)
		require.NoError(t, err, "DOCKER_HOST")
		return u.Hostname(), "0.0.0.0"
	}
	return "127.0.0.1", "127.0.0.1"
}

// freePort returns a TCP port that is free on this host. The container keeps
// it across restarts, unlike a port docker picks.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func randomHex(t *testing.T, n int) string {
	t.Helper()
	b := make([]byte, n)
	_, err := rand.Read(b)
	require.NoError(t, err)
	return hex.EncodeToString(b)
}

// newPassword returns a random password, never printed.
func newPassword(t *testing.T) string {
	return "It-" + randomHex(t, 12)
}

// dockerCmd runs the docker CLI. stdin carries secrets, never the arguments,
// which other processes can read.
func dockerCmd(timeout time.Duration, stdin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (e *sambaEnv) mustDocker(timeout time.Duration, stdin string, args ...string) string {
	e.t.Helper()
	out, err := dockerCmd(timeout, stdin, args...)
	require.NoError(e.t, err, "docker %s: %s", args[0], e.redact(out))
	return out
}

// setPassword sets the password of sambaUser on the server. Sessions already
// established keep working; new ones need the new password.
func (e *sambaEnv) setPassword(password string) {
	e.t.Helper()
	e.mu.Lock()
	e.password = password
	e.secrets = append(e.secrets, password)
	e.mu.Unlock()
	e.mustDocker(time.Minute, password+"\n"+password+"\n", "exec", "--interactive", e.container, "smbpasswd", "-s", "-a", sambaUser)
}

// currentPassword returns the password the server accepts now.
func (e *sambaEnv) currentPassword() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.password
}

// redact removes every password the server ever had from s.
func (e *sambaEnv) redact(s string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, secret := range e.secrets {
		s = strings.ReplaceAll(s, secret, redactedPassword)
	}
	return s
}

// secretsIn returns how many of the passwords the server ever had appear in s.
func (e *sambaEnv) secretsIn(s string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, secret := range e.secrets {
		if strings.Contains(s, secret) {
			n++
		}
	}
	return n
}

func (e *sambaEnv) clientConfig(password string) client.Config {
	return client.Config{
		Host:        e.host,
		Share:       sambaShare,
		Username:    sambaUser,
		Password:    password,
		Port:        e.port,
		DialTimeout: 5 * time.Second,
		OpTimeout:   5 * time.Second,
	}
}

// listDir lists dir with the log source's client, as the launcher sees it.
func (e *sambaEnv) listDir(dir string) ([]client.Entry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := client.Dial(ctx, e.clientConfig(e.currentPassword()))
	if err != nil {
		return nil, errors.New(e.redact(err.Error()))
	}
	defer c.Close()
	entries, err := c.ListDir(ctx, dir)
	if err != nil {
		return nil, errors.New(e.redact(err.Error()))
	}
	return entries, nil
}

// fileID returns the FileId the listing of dir reports for name, 0 if absent.
func (e *sambaEnv) fileID(dir, name string) uint64 {
	e.t.Helper()
	entries, err := e.listDir(dir)
	require.NoError(e.t, err)
	for _, entry := range entries {
		if entry.Name == name {
			return entry.FileID
		}
	}
	return 0
}

// waitReady waits until the log source's client can connect with the current
// password and list the share.
func (e *sambaEnv) waitReady(timeout time.Duration) {
	e.t.Helper()
	var err error
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		var c client.Client
		c, err = client.Dial(ctx, e.clientConfig(e.currentPassword()))
		if err == nil {
			_, err = c.ListDir(ctx, "")
			_ = c.Close()
		}
		cancel()
		if err == nil {
			return
		}
	}
	require.FailNow(e.t, "the Samba server is not ready", "last error: %s", e.redact(fmt.Sprint(err)))
}

// restart restarts the container, which drops every SMB connection; the
// share's files and the account survive.
func (e *sambaEnv) restart() {
	e.t.Helper()
	e.mustDocker(2*time.Minute, "", "restart", "--time", "2", e.container)
	e.waitReady(time.Minute)
}

// pause freezes the server's processes: connections stay open but nothing
// answers, as with a network partition.
func (e *sambaEnv) pause() {
	e.t.Helper()
	e.mustDocker(time.Minute, "", "pause", e.container)
}

func (e *sambaEnv) unpause() {
	e.t.Helper()
	e.mustDocker(time.Minute, "", "unpause", e.container)
}

// smbWriter is the log writer of the tests: an application writing its logs
// to the share over its own SMB session, as another machine would. It holds
// its files open between writes, like a logging library, and writes each
// file at the offset it tracks (it is the only writer of its files), so a
// write retried after an outage never duplicates a line.
//
// Its methods are called from one goroutine at a time; errors are returned,
// not reported, so a writer goroutine can hand them to the test.
type smbWriter struct {
	env   *sambaEnv
	sess  *smb2.Session
	share *smb2.Share
	open  map[string]*writerFile // open files, by their current path
	dials int
}

type writerFile struct {
	f    *smb2.File
	size int64 // offset of the next write
}

func newSMBWriter(env *sambaEnv) *smbWriter {
	return &smbWriter{env: env, open: make(map[string]*writerFile)}
}

// connect returns the writer's share, dialing with the server's current
// password and reopening its files if the previous session was dropped.
func (w *smbWriter) connect(ctx context.Context) (*smb2.Share, error) {
	if w.share != nil {
		return w.share, nil
	}
	password := w.env.currentPassword()
	d := &smb2.Dialer{
		Credentials:           auth.NTLMCredential{User: sambaUser, Password: password},
		TransportDialer:       smb2.TCPDialer{Port: w.env.port, Dialer: &net.Dialer{Timeout: 5 * time.Second}},
		RequireMessageSigning: true,
		DisableAAPLExtension:  true,
	}
	sess, err := d.Dial(ctx, w.env.host)
	if err != nil {
		return nil, err
	}
	share, err := sess.Mount(ctx, sambaShare)
	if err != nil {
		_ = sess.Abort()
		return nil, err
	}
	w.dials++
	for p, wf := range w.open {
		f, err := share.OpenFile(ctx, p, os.O_RDWR, 0)
		if err != nil {
			_ = sess.Abort()
			return nil, fmt.Errorf("reopen %s: %w", p, err)
		}
		wf.f = f
	}
	w.sess, w.share = sess, share
	return share, nil
}

// drop forgets a broken session. Its handles died with it.
func (w *smbWriter) drop() {
	if w.sess != nil {
		_ = w.sess.Abort()
	}
	w.sess, w.share = nil, nil
}

// do runs op, reconnecting and retrying it after network and session errors
// for up to writerRetryFor. Other errors are returned at once.
func (w *smbWriter) do(what string, op func(ctx context.Context, share *smb2.Share) error) error {
	deadline := time.Now().Add(writerRetryFor)
	backoff := 100 * time.Millisecond
	for {
		ctx, cancel := context.WithTimeout(context.Background(), writerOpTimeout)
		share, err := w.connect(ctx)
		if err == nil {
			err = op(ctx, share)
		}
		cancel()
		if err == nil {
			return nil
		}
		retryable := client.Classify(err) == client.ErrTransient || errors.Is(err, os.ErrClosed)
		if !retryable && w.share != nil {
			return fmt.Errorf("writer: %s: %s", what, w.env.redact(err.Error()))
		}
		// A failed dial (any error, including a password the server no
		// longer accepts) or a broken session: start over.
		w.drop()
		if time.Now().After(deadline) {
			return fmt.Errorf("writer: %s: still failing after %s: %s", what, writerRetryFor, w.env.redact(err.Error()))
		}
		time.Sleep(backoff)
		backoff = min(2*backoff, 2*time.Second)
	}
}

// mkdirAll creates dir on the share.
func (w *smbWriter) mkdirAll(dir string) error {
	return w.do("mkdir "+dir, func(ctx context.Context, share *smb2.Share) error {
		return share.MkdirAll(ctx, dir, 0o755)
	})
}

// create creates (or empties) the file p and keeps it open.
func (w *smbWriter) create(p string) error {
	if wf, ok := w.open[p]; ok {
		_ = wf.f.Close(context.Background())
		delete(w.open, p)
	}
	return w.do("create "+p, func(ctx context.Context, share *smb2.Share) error {
		f, err := share.OpenFile(ctx, p, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		w.open[p] = &writerFile{f: f}
		return nil
	})
}

// write appends data to the open file p, in one SMB WRITE.
func (w *smbWriter) write(p string, data []byte) error {
	wf, ok := w.open[p]
	if !ok {
		return fmt.Errorf("writer: write %s: not open", p)
	}
	return w.do("write "+p, func(ctx context.Context, _ *smb2.Share) error {
		n, err := wf.f.WriteAt(ctx, data, wf.size)
		if err != nil {
			return err
		}
		if n != len(data) {
			return fmt.Errorf("short write: %d of %d bytes", n, len(data))
		}
		wf.size += int64(n)
		return nil
	})
}

// closeFile closes the open file p.
func (w *smbWriter) closeFile(p string) error {
	wf, ok := w.open[p]
	if !ok {
		return nil
	}
	delete(w.open, p)
	ctx, cancel := context.WithTimeout(context.Background(), writerOpTimeout)
	defer cancel()
	_ = wf.f.Close(ctx)
	return nil
}

// rename renames oldPath to newPath, replacing newPath. With keepOpen, the
// writer keeps its handle on the file under its new name, as a writer that
// is not told about the rotation does; otherwise it closes the file first,
// as Log4j2's rollover does.
func (w *smbWriter) rename(oldPath, newPath string, keepOpen bool) error {
	if !keepOpen {
		if err := w.closeFile(oldPath); err != nil {
			return err
		}
	}
	err := w.do("rename "+oldPath, func(ctx context.Context, share *smb2.Share) error {
		err := share.Rename(ctx, oldPath, newPath)
		if err != nil && errors.Is(err, os.ErrNotExist) {
			// A retry of a rename the server already made.
			if _, statErr := share.Stat(ctx, newPath); statErr == nil {
				return nil
			}
		}
		return err
	})
	if err == nil {
		if wf, ok := w.open[oldPath]; ok {
			delete(w.open, oldPath)
			w.open[newPath] = wf
		}
	}
	return err
}

// truncate empties the open file p, as copytruncate does.
func (w *smbWriter) truncate(p string) error {
	wf, ok := w.open[p]
	if !ok {
		return fmt.Errorf("writer: truncate %s: not open", p)
	}
	return w.do("truncate "+p, func(ctx context.Context, _ *smb2.Share) error {
		if err := wf.f.Truncate(ctx, 0); err != nil {
			return err
		}
		wf.size = 0
		return nil
	})
}

// remove deletes p, closing it first if it is open.
func (w *smbWriter) remove(p string) error {
	if err := w.closeFile(p); err != nil {
		return err
	}
	return w.do("remove "+p, func(ctx context.Context, share *smb2.Share) error {
		err := share.Remove(ctx, p)
		if err != nil && errors.Is(err, os.ErrNotExist) {
			return nil // a retry of a removal the server already made
		}
		return err
	})
}

// readFile returns the content of p.
func (w *smbWriter) readFile(p string) ([]byte, error) {
	var data []byte
	err := w.do("read "+p, func(ctx context.Context, share *smb2.Share) error {
		var err error
		data, err = share.ReadFile(ctx, p)
		return err
	})
	return data, err
}

// copyFile copies src to dst, as copytruncate does.
func (w *smbWriter) copyFile(src, dst string) error {
	data, err := w.readFile(src)
	if err != nil {
		return err
	}
	return w.do("copy "+src, func(ctx context.Context, share *smb2.Share) error {
		return share.WriteFile(ctx, dst, data, 0o644)
	})
}

// gzipFile writes src compressed to dst, as compress-on-rotate does. It does
// not remove src.
func (w *smbWriter) gzipFile(src, dst string) error {
	data, err := w.readFile(src)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return w.do("gzip "+src, func(ctx context.Context, share *smb2.Share) error {
		return share.WriteFile(ctx, dst, buf.Bytes(), 0o644)
	})
}

// close closes every file and logs off.
func (w *smbWriter) close() {
	for p := range w.open {
		_ = w.closeFile(p)
	}
	if w.sess != nil {
		_ = w.sess.Close()
	}
	w.sess, w.share = nil, nil
}

// String implements fmt.Stringer, so printing a writer never prints a password.
func (w *smbWriter) String() string {
	return "smbWriter{" + w.env.host + ":" + strconv.Itoa(w.env.port) + "}"
}
