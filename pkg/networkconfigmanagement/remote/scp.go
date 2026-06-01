// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remote

import (
	"bufio"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/ssh"
)

// readSCPResponse reads an SCP response from the reader. If the first byte is
// 0, the command succeeded and nothing more is read. Otherwise, the remaining
// bytes are read and returned as the error message.
func readSCPResponse(reader *bufio.Reader) error {
	b, err := reader.ReadByte()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return io.ErrUnexpectedEOF
		}
		return err
	}
	if b > 0 {
		msg, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			if len(msg) > 0 {
				err = nil
			} else {
				err = io.ErrUnexpectedEOF
			}
		}
		if err != nil {
			return err
		}
		return errors.New(string(msg))
	}
	return nil
}

func executeSCP(session *ssh.Session, cmdStr string, filename string, data string) (output string, err error) {
	// Get I/O handles
	stdin, err := session.StdinPipe()
	if err != nil {
		return "", err
	}
	rawStdout, err := session.StdoutPipe()
	if err != nil {
		return "", err
	}
	stdout := bufio.NewReader(rawStdout)
	stderr, err := session.StderrPipe()
	if err != nil {
		return "", err
	}
	defer func() {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			// If we stopped because the other side closed, instead of because
			// we got an error message, check if they wrote anything to stderr
			// and return that if so.
			errBytes, _ := io.ReadAll(stderr)
			if len(errBytes) > 0 {
				err = errors.New(string(errBytes))
			}
		}
	}()
	// Start the SCP command
	if err := session.Start(cmdStr); err != nil {
		return "", err
	}
	// Write the scp control line
	if _, err := fmt.Fprintln(stdin, "C0600", len(data), filename); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return "", err
	}
	if err := readSCPResponse(stdout); err != nil {
		return "", err
	}
	// Write the data
	if _, err = fmt.Fprint(stdin, data+"\x00"); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return "", err
	}
	if err := readSCPResponse(stdout); err != nil {
		return "", err
	}
	stdin.Close()
	if err := session.Wait(); err != nil {
		return "", err
	}
	// Read any additional output that the command provides
	result, err := io.ReadAll(stdout)
	return string(result), err
}
