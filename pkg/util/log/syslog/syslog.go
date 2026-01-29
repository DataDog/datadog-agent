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

// HeaderFormatter creates a function that formats a syslog header for the given log level.
func HeaderFormatter(facility int, rfc bool) func(types.LogLevel) string {
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

// Receiver writes log messages to syslog
type Receiver struct {
	uri  *url.URL
	conn net.Conn
}

// NewReceiver creates a new syslog receiver with the given URI.
// The URI should be in the format "udp://host:port", "tcp://host:port", or "unix:///path/to/socket".
func NewReceiver(uri string) (*Receiver, error) {
	parsedURI, err := url.ParseRequestURI(uri)
	if err != nil {
		return nil, fmt.Errorf("bad syslog receiver configuration: %w", err)
	}

	conn, err := getSyslogConnection(parsedURI)
	if err != nil {
		// Connection failure is not fatal - we'll retry on write
		fmt.Printf("%v\n", err)
	}

	return &Receiver{
		uri:  parsedURI,
		conn: conn,
	}, nil
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

// Flush is a NOP in current implementation
func (s *Receiver) Flush() {
	// Nothing to do here...
}

// Close is a NOP in current implementation
func (s *Receiver) Close() error {
	return nil
}
