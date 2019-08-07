// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build systemd

package systemd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/coreos/go-systemd/dbus"
	godbus "github.com/godbus/dbus"
)

// New establishes a connection to any available bus and authenticates.
// Callers should call Close() when done with the connection.
func NewSystemdAnyConnection(privateSocket string) (*dbus.Conn, error) {
	conn, err := NewSystemdConnection(privateSocket)
	if err != nil {
		log.Debugf("Fail to connect using private socket: %v", err)
		return dbus.NewSystemConnection()
	}
	return conn, err
}

// NewSystemdConnection establishes a private, direct connection to systemd.
// This can be used for communicating with systemd without a dbus daemon.
// Callers should call Close() when done with the connection.
// Note: method borrowed from `go-systemd/dbus` to provide custom path for systemd private socket
func NewSystemdConnection(privateSocket string) (*dbus.Conn, error) {
	return dbus.NewConnection(func() (*godbus.Conn, error) {
		// We skip Hello when talking directly to systemd.
		return dbusAuthConnection(func() (*godbus.Conn, error) {
			return godbus.Dial(fmt.Sprintf("unix:path=%s", privateSocket))
		})
	})
}

// Note: method borrowed from `go-systemd/dbus` to provide custom path for systemd private socket
func dbusAuthConnection(createBus func() (*godbus.Conn, error)) (*godbus.Conn, error) {
	conn, err := createBus()
	if err != nil {
		return nil, err
	}

	// Only use EXTERNAL method, and hardcode the uid (not username)
	// to avoid a username lookup (which requires a dynamically linked
	// libc)
	methods := []godbus.Auth{godbus.AuthExternal(strconv.Itoa(os.Getuid()))}

	err = conn.Auth(methods)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}
