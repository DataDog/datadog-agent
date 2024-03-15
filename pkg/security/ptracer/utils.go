// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds the start command of CWS injector
package ptracer

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/security/common/containerutils"
)

// Funcs mainly copied from github.com/DataDog/datadog-agent/pkg/security/utils/cgroup.go
// in order to reduce the binary size of cws-instrumentation

type controlGroup struct {
	// id unique hierarchy ID
	id int

	// controllers are the list of cgroup controllers bound to the hierarchy
	controllers []string

	// path is the pathname of the control group to which the process
	// belongs. It is relative to the mountpoint of the hierarchy.
	path string
}

func getProcControlGroupsFromFile(path string) ([]controlGroup, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cgroups []controlGroup
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		t := scanner.Text()
		parts := strings.Split(t, ":")
		var ID int
		ID, err = strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		c := controlGroup{
			id:          ID,
			controllers: strings.Split(parts[1], ","),
			path:        parts[2],
		}
		cgroups = append(cgroups, c)
	}
	return cgroups, nil

}

func getCurrentProcContainerID() (string, error) {
	cgroups, err := getProcControlGroupsFromFile("/proc/self/cgroup")
	if err != nil {
		return "", err
	}

	for _, cgroup := range cgroups {
		cid := containerutils.FindContainerID(cgroup.path)
		if cid != "" {
			return cid, nil
		}
	}
	return "", nil
}

func getNSID() uint64 {
	var stat syscall.Stat_t
	if err := syscall.Stat("/proc/self/ns/pid", &stat); err != nil {
		return rand.Uint64()
	}
	return stat.Ino
}

// simpleHTTPRequest used to avoid importing the crypto golang package
func simpleHTTPRequest(uri string) ([]byte, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	addr := u.Host
	if u.Port() == "" {
		addr += ":80"
	}

	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}

	client, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	path := u.Path
	if path == "" {
		path = "/"
	}

	req := fmt.Sprintf("GET %s?%s HTTP/1.1\nHost: %s\nConnection: close\n\n", path, u.RawQuery, u.Hostname())

	_, err = client.Write([]byte(req))
	if err != nil {
		return nil, err
	}

	var body []byte
	buf := make([]byte, 256)

	for {
		n, err := client.Read(buf)
		if err != nil {
			if err != io.EOF {
				return nil, err
			}
			break
		}
		body = append(body, buf[:n]...)
	}

	offset := bytes.Index(body, []byte{'\r', '\n', '\r', '\n'})
	if offset < 0 {

		return nil, errors.New("unable to parse http response")
	}

	return body[offset+2:], nil
}
