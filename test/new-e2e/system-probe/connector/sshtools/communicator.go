// Copyright (C) 2017 ScyllaDB

package sshtools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// Logger is the minimal interface Communicator needs for logging. Note that
// log.Logger from the standard library implements this interface, and it is
// easy to implement by custom loggers, if they don't do so already anyway.
type Logger interface {
	Println(v ...interface{})
}

// Communicator allows for executing commands on a remote host over SSH, it is
// not thread safe. New communicator is not connected by default, however,
// calling Start or Upload on not connected communicator would try to establish
// SSH connection before executing.
type Communicator struct {
	host   string
	config Config
	dial   DialContextFunc
	logger Logger

	// OnDial is a listener that may be set to track openning SSH connection to
	// the remote host. It is called for both successful and failed trials.
	OnDial func(host string, err error)
	// OnConnClose is a listener that may be set to track closing of SSH
	// connection.
	OnConnClose func(host string)

	client        *ssh.Client
	keepaliveDone chan struct{}
}

// NewCommunicator creates a *sshtools.Communicator
func NewCommunicator(host string, config Config, dial DialContextFunc, logger Logger) *Communicator {
	return &Communicator{
		host:   host,
		config: config,
		dial:   dial,
		logger: logger,
	}
}

// Connect must be called to connect the communicator to remote host. It can
// be called multiple times, in that case the current SSH connection is closed
// and a new connection is established.
func (c *Communicator) Connect(ctx context.Context) (err error) {
	c.logger.Println("Connecting to remote host", "host", c.host)

	defer func() {
		if c.OnDial != nil {
			c.OnDial(c.host, err)
		}
	}()

	c.reset()

	client, err := c.dial(ctx, "tcp", net.JoinHostPort(c.host, fmt.Sprint(c.config.Port)), &c.config.ClientConfig)
	if err != nil {
		return fmt.Errorf("ssh: dial failed: %w", err)
	}
	c.client = client

	c.logger.Println("Connected!", "host", c.host)

	if c.config.KeepaliveEnabled() {
		c.logger.Println("Starting ssh KeepAlives", "host", c.host)
		c.keepaliveDone = make(chan struct{})
		go StartKeepalive(client, c.config.ServerAliveInterval, c.config.ServerAliveCountMax, c.keepaliveDone)
	}

	return nil
}

// Disconnect closes the current SSH connection.
func (c *Communicator) Disconnect() {
	c.reset()
}

func (c *Communicator) reset() {
	if c.keepaliveDone != nil {
		close(c.keepaliveDone)
	}
	c.keepaliveDone = nil

	if c.client != nil {
		c.client.Close()
		if c.OnConnClose != nil {
			c.OnConnClose(c.host)
		}
	}
	c.client = nil
}

// Start starts the specified command but does not wait for it to complete.
// Each command is executed in a new SSH session. If context is canceled
// the session is immediately closed and error is returned.
//
// The cmd Wait method will return the exit code and release associated
// resources once the command exits.
func (c *Communicator) Start(ctx context.Context, cmd *Cmd) error {
	session, err := c.newSession(ctx)
	if err != nil {
		return err
	}

	// Setup command
	cmd.init(ctx, session)

	// Setup session
	session.Stdin = cmd.Stdin
	session.Stdout = cmd.Stdout
	session.Stderr = cmd.Stderr

	if c.config.Pty {
		// request a PTY
		termModes := ssh.TerminalModes{
			ssh.ECHO:          0,     // do not echo
			ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
			ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
		}

		if err := session.RequestPty("xterm", 80, 40, termModes); err != nil {
			return err
		}
	}

	for _, kv := range cmd.Env {
		if key, val, ok := strings.Cut(kv, "="); ok {
			if err := session.Setenv(key, val); err != nil {
				return fmt.Errorf("set env `%s`: %w", key, err)
			}
		}
	}

	c.logger.Println("Starting remote command",
		"host", c.host,
		"cmd", cmd.Command,
	)
	err = session.Start(strings.TrimSpace(cmd.Command) + "\n")
	if err != nil {
		return err
	}

	// Start a goroutine to wait for the session to end
	go func() {
		defer session.Close()

		err := session.Wait()
		exitStatus := 0
		if err != nil {
			if exitErr, ok := err.(*ssh.ExitError); ok {
				exitStatus = exitErr.ExitStatus()
			} else {
				exitStatus = -1
			}
		}

		if err != nil {
			c.logger.Println("Remote command exited with error",
				"host", c.host,
				"cmd", cmd.Command,
				"status", exitStatus,
				"error", err,
			)
		} else {
			c.logger.Println("Remote command exited",
				"host", c.host,
				"cmd", cmd.Command,
				"status", exitStatus,
			)
		}

		cmd.setExitStatus(exitStatus, err)
	}()

	return nil
}

func (c *Communicator) newSession(ctx context.Context) (session *ssh.Session, err error) {
	c.logger.Println("Opening new ssh session", "host", c.host)
	if c.client == nil {
		err = errors.New("ssh client is not connected")
	} else {
		session, err = c.client.NewSession()
	}

	if err != nil {
		c.logger.Println("ssh session open error", "host", c.host, "error", err)
		if err := c.Connect(ctx); err != nil {
			return nil, err
		}

		return c.client.NewSession()
	}

	return session, nil
}

// Upload creates a file with a given path and permissions and content on
// a remote host. If context is canceled the upload is interrupted, file is not
// saved and error is returned.
func (c *Communicator) Upload(ctx context.Context, path string, perm os.FileMode, src io.Reader) error {
	// The target directory and file for talking the SCP protocol
	targetDir := filepath.Dir(path)
	targetFile := filepath.Base(path)

	// On windows, filepath.Dir uses backslash separators (ie. "\tmp").
	// This does not work when the target host is unix.  Switch to forward slash
	// which works for unix and windows
	targetDir = filepath.ToSlash(targetDir)

	// Skip copying if we can get the file size directly from common io.Readers
	size := int64(0)

	switch s := src.(type) {
	case interface {
		Stat() (os.FileInfo, error)
	}:
		fi, err := s.Stat()
		if err == nil {
			size = fi.Size()
		}
	case interface {
		Len() int
	}:
		size = int64(s.Len())
	}

	c.logger.Println("Uploading file",
		"host", c.host,
		"path", path,
		"perm", perm.Perm(),
	)

	scpFunc := func(w io.Writer, stdoutR *bufio.Reader) error {
		return scpUploadFile(w, src, stdoutR, targetFile, perm, size)
	}
	err := c.scpSession(ctx, "scp -vt "+targetDir, scpFunc)

	if err != nil {
		c.logger.Println("Uploading file ended with error",
			"host", c.host,
			"path", path,
			"perm", perm.Perm(),
			"error", err,
		)
	} else {
		c.logger.Println("Uploading file ended",
			"host", c.host,
			"path", path,
			"perm", perm.Perm(),
		)
	}

	return err
}

func (c *Communicator) scpSession(ctx context.Context, scpCommand string, f func(io.Writer, *bufio.Reader) error) error {
	session, err := c.newSession(ctx)
	if err != nil {
		return err
	}
	defer session.Close()

	// Get a pipe to stdin so that we can send data down
	stdinW, err := session.StdinPipe()
	if err != nil {
		return err
	}

	// We only want to close once, so we nil w after we close it,
	// and only close in the defer if it hasn't been closed already.
	defer func() {
		if stdinW != nil {
			stdinW.Close()
		}
	}()

	// Get a pipe to stdout so that we can get responses back
	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	stdoutR := bufio.NewReader(stdoutPipe)

	// Start the sink mode on the other side
	if err := session.Start(scpCommand); err != nil {
		return err
	}

	// Call our callback that executes in the context of SCP. We ignore
	// EOF errors if they occur because it usually means that SCP prematurely
	// ended on the other side.
	if err := f(stdinW, stdoutR); err != nil && err != io.EOF {
		return err
	}

	// Close the stdin, which sends an EOF, and then set w to nil so that
	// our defer func doesn't close it again since that is unsafe with
	// the Go SSH package.
	stdinW.Close()
	stdinW = nil

	// Wait for the SCP connection to close, meaning it has consumed all
	// our data and has completed. Or has errored.
	exitCh := make(chan struct{})
	go func() {
		// Ignore result if context was cancelled
		if ctx.Err() != nil {
			return
		}
		err = session.Wait()
		close(exitCh)
	}()

	select {
	case <-ctx.Done():
		err = ctx.Err()
	case <-exitCh:
		// continue
	}

	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			// Otherwise, we have an ExitErorr, meaning we can just read
			// the exit status
			c.logger.Println("scp error", "host", c.host, "error", exitErr)

			// If we exited with status 127, it means SCP isn't available.
			// Return a more descriptive error for that.
			if exitErr.ExitStatus() == 127 {
				return errors.New("SCP failed to start, this usually means that SCP is not properly installed on the remote system")
			}
		}

		return err
	}

	return nil
}

// checkSCPStatus checks that a prior command sent to SCP completed
// successfully. If it did not complete successfully, an error will
// be returned.
func checkSCPStatus(r *bufio.Reader) error {
	code, err := r.ReadByte()
	if err != nil {
		return err
	}

	if code != 0 {
		// Treat any non-zero (really 1 and 2) as fatal errors
		message, _, err := r.ReadLine()
		if err != nil {
			return fmt.Errorf("error reading error message: %w", err)
		}

		return errors.New(string(message))
	}

	return nil
}

func scpUploadFile(dst io.Writer, src io.Reader, stdout *bufio.Reader, file string, perm os.FileMode, size int64) error {
	if size == 0 {
		// Create a temporary file where we can copy the contents of the src
		// so that we can determine the length, since SCP is length-prefixed.
		tf, err := os.CreateTemp("", "scylla-manager-upload")
		if err != nil {
			return fmt.Errorf("error creating temporary file for upload: %w", err)
		}
		defer os.Remove(tf.Name()) // nolint: errcheck
		defer tf.Close()

		if _, err := io.Copy(tf, src); err != nil {
			return err
		}

		// Sync the file so that the contents are definitely on disk, then
		// read the length of it.
		if err := tf.Sync(); err != nil {
			return fmt.Errorf("error creating temporary file for upload: %w", err)
		}

		// Seek the file to the beginning so we can re-read all of it
		if _, err := tf.Seek(0, 0); err != nil {
			return fmt.Errorf("error creating temporary file for upload: %w", err)
		}

		fi, err := tf.Stat()
		if err != nil {
			return fmt.Errorf("error creating temporary file for upload: %w", err)
		}

		src = tf
		size = fi.Size()
	}

	// Start the protocol
	mode := fmt.Sprintf("C%04o", uint32(perm.Perm()))

	fmt.Fprintln(dst, mode, size, file)
	if err := checkSCPStatus(stdout); err != nil {
		return err
	}

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}

	fmt.Fprint(dst, "\x00")
	if err := checkSCPStatus(stdout); err != nil {
		return err
	}

	return nil
}
