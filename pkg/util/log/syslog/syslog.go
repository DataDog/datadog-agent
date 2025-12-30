// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package syslog logs to syslog
package syslog

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

var levelToSyslogSeverity = map[types.LogLevel]int{
	// Mapping to RFC 5424 where possible
	types.TraceLvl:    7,
	types.DebugLvl:    7,
	types.InfoLvl:     6,
	types.WarnLvl:     4,
	types.ErrorLvl:    3,
	types.CriticalLvl: 2,
	types.Off:         7,
}

// HeaderFormatter creates a seelog formatter function that formats a message as a syslog header.
func HeaderFormatter(facility int, rfc bool) func(level types.LogLevel) string {
	pid := os.Getpid()
	appName := filepath.Base(os.Args[0])

	if rfc { // RFC 5424
		return func(level types.LogLevel) string {
			return fmt.Sprintf("<%d>1 %s %d - -", facility*8+levelToSyslogSeverity[level], appName, pid)
		}
	}

	// otherwise old-school logging
	return func(level types.LogLevel) string {
		return fmt.Sprintf("<%d>%s[%d]:", facility*8+levelToSyslogSeverity[level], appName, pid)
	}
}

// Receiver implements seelog.CustomReceiver
type Receiver struct {
	enabled bool
	uri     *url.URL
	conn    net.Conn
}

var syslogAddrs = []string{"/dev/log", "/var/run/syslog", "/var/run/log"}

func getSyslogConnection(uri *url.URL) (net.Conn, error) {
	var conn net.Conn
	var err error

	// local
	localNetNames := []string{"unixgram", "unix"}
	if uri == nil {
		for _, netName := range localNetNames {
			for _, addr := range syslogAddrs {
				conn, err = net.Dial(netName, addr)
				if err == nil { // on success
					return conn, nil
				}
			}
		}
	} else {
		switch uri.Scheme {
		case "unix", "unixgram":
			fmt.Printf("Trying to connect to: %s\n", uri.Path)
			for _, netName := range localNetNames {
				conn, err = net.Dial(netName, uri.Path)
				if err == nil {
					break
				}
			}
		case "udp":
			conn, err = net.Dial(uri.Scheme, uri.Host)
		case "tcp":
			conn, err = net.Dial("tcp", uri.Host)
		}
		if err == nil {
			return conn, nil
		}
	}

	return nil, errors.New("Unable to connect to syslog")
}

// Write writes the message to the syslog receiver
func (s *Receiver) Write(message []byte) (int, error) {
	if !s.enabled {
		return 0, nil
	}

	if s.conn != nil {
		return s.conn.Write(message)
	}

	// try to reconnect - close the connection first just in case
	//                    we don't want fd leaks here.
	if s.conn != nil {
		s.conn.Close()
	}
	conn, err := getSyslogConnection(s.uri)
	if err != nil {
		return 0, err
	}

	s.conn = conn
	n, err := s.conn.Write(message)
	fmt.Printf("Retried: %v\n", message)
	return n, err
}

// AfterParse parses the receiver configuration
func (s *Receiver) AfterParse(uri string) error {
	var conn net.Conn
	var err error

	s.enabled = true
	if uri != "" {
		url, err := url.ParseRequestURI(uri)
		if err != nil {
			s.enabled = false
		}

		s.uri = url
	}

	if !s.enabled {
		return errors.New("bad syslog receiver configuration - disabling")
	}

	conn, err = getSyslogConnection(s.uri)
	if err != nil {
		fmt.Printf("%v\n", err)
		return nil
	}
	s.conn = conn

	return nil
}

// Flush is a NOP in current implementation
func (s *Receiver) Flush() {
	// Nothing to do here...
}

// Close is a NOP in current implementation
func (s *Receiver) Close() error {
	return nil
}
