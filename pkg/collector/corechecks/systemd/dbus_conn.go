// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
//
// This file includes software developed at CoreOS, Inc.
// (http://www.coreos.com/).
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

// +build systemd

package systemd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/coreos/go-systemd/dbus"
	godbus "github.com/godbus/dbus"
)

// NewSystemdConnection establishes a private, direct connection to systemd.
// This can be used for communicating with systemd without a dbus daemon.
// Callers should call Close() when done with the connection.
// Note: method borrowed from `go-systemd/dbus` to provide custom path for systemd private socket
// Source: https://github.com/coreos/go-systemd
func NewSystemdConnection(privateSocket string) (*dbus.Conn, error) {
	return dbus.NewConnection(func() (*godbus.Conn, error) {
		// We skip Hello when talking directly to systemd.
		return dbusAuthConnection(func() (*godbus.Conn, error) {
			return godbus.Dial(fmt.Sprintf("unix:path=%s", privateSocket))
		})
	})
}

// Note: method borrowed from `go-systemd/dbus` to provide custom path for systemd private socket
// Source: https://github.com/coreos/go-systemd
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
