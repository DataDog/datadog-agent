// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package daemon

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// clientIDFile is the file the daemon keeps its remote config client ID in.
const clientIDFile = "installer-client-id"

// loadClientID returns the remote config client ID of the host's installer daemon.
//
// Fleet addresses a task to one client ID. A configuration experiment replaces the stable daemon
// with the experiment daemon, and ending it replaces that one with the stable daemon again, each
// within seconds, so a random ID per process would leave a task addressed to an ID that no daemon
// has any more. Every daemon of the host reads the same ID from a file in the root-owned state
// directory instead, which the Agent's account cannot change. An empty return gives the client a
// random ID, as before.
func loadClientID() string {
	id, err := loadOrCreateClientID(paths.ConfigExperimentStatePath)
	if err != nil {
		log.Warnf("Daemon: could not keep a remote config client ID, using a random one: %v", err)
		return ""
	}
	return id
}

// loadOrCreateClientID returns the client ID kept in dir, creating the file with a new ID when it
// is missing or holds anything but an ID. Two daemons that both find it missing end up with the
// same ID: the one that creates it first wins, and the other reads it.
func loadOrCreateClientID(dir string) (string, error) {
	path := filepath.Join(dir, clientIDFile)
	id, err := readClientID(path)
	if err == nil {
		return id, nil
	}
	missing := errors.Is(err, fs.ErrNotExist)
	if !missing {
		log.Warnf("Daemon: replacing the remote config client ID at %s: %v", path, err)
	}

	tmp, err := os.CreateTemp(dir, clientIDFile+".*")
	if err != nil {
		return "", fmt.Errorf("could not create a client ID file in %s: %w", dir, err)
	}
	defer os.Remove(tmp.Name())
	id = client.GenerateClientID()
	_, err = tmp.WriteString(id)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		// The ID is not a secret: any account may read it, none but root may write it.
		err = os.Chmod(tmp.Name(), 0644)
	}
	if err != nil {
		return "", fmt.Errorf("could not write the client ID: %w", err)
	}

	if !missing {
		if err := os.Rename(tmp.Name(), path); err != nil {
			return "", fmt.Errorf("could not replace %s: %w", path, err)
		}
		return id, nil
	}
	// Link rather than rename: it fails if another daemon created the file since the read, and then
	// that daemon's ID is the one to use.
	err = os.Link(tmp.Name(), path)
	if errors.Is(err, fs.ErrExist) {
		return readClientID(path)
	}
	if err != nil {
		return "", fmt.Errorf("could not publish %s: %w", path, err)
	}
	return id, nil
}

// readClientID reads the client ID at path without following a final link or blocking on a
// special file. Anything that is not a regular file holding exactly an ID is an error.
func readClientID(path string) (string, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("not a regular file")
	}
	// Read more than an ID is long, so that a file holding an ID and anything else is not taken for one.
	buf := make([]byte, 64)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}
	id := string(buf[:n])
	if !client.ValidClientID(id) {
		return "", errors.New("not a client ID")
	}
	return id, nil
}
