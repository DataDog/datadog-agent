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
	"strconv"
	"strings"

	"github.com/cihub/seelog"
)

var levelToSyslogSeverity = map[seelog.LogLevel]int{
	// Mapping to RFC 5424 where possible
	seelog.TraceLvl:    7,
	seelog.DebugLvl:    7,
	seelog.InfoLvl:     6,
	seelog.WarnLvl:     4,
	seelog.ErrorLvl:    3,
	seelog.CriticalLvl: 2,
	seelog.Off:         7,
}

// CreateSyslogHeaderFormatter creates a seelog formatter function that formats a message as a syslog header.
func CreateSyslogHeaderFormatter(params string) seelog.FormatterFunc {
	facility := 20
	rfc := false

	ps := strings.Split(params, ",")
	if len(ps) == 2 {
		i, err := strconv.Atoi(ps[0])
		if err == nil && i >= 0 && i <= 23 {
			facility = i
		}

		rfc = (ps[1] == "true")
	} else {
		fmt.Println("badly formatted syslog header parameters - using defaults")
	}

	pid := os.Getpid()
	appName := filepath.Base(os.Args[0])

	if rfc { // RFC 5424
		return func(_ string, level seelog.LogLevel, _ seelog.LogContextInterface) interface{} {
			return fmt.Sprintf("<%d>1 %s %d - -", facility*8+levelToSyslogSeverity[level], appName, pid)
		}
	}

	// otherwise old-school logging
	return func(_ string, level seelog.LogLevel, _ seelog.LogContextInterface) interface{} {
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

// ReceiveMessage process current log message
func (s *Receiver) ReceiveMessage(message string, _ seelog.LogLevel, _ seelog.LogContextInterface) error {
	if !s.enabled {
		return nil
	}

	if s.conn != nil {
		_, err := s.conn.Write([]byte(message))
		if err == nil {
			return nil
		}
	}

	// try to reconnect - close the connection first just in case
	//                    we don't want fd leaks here.
	if s.conn != nil {
		s.conn.Close()
	}
	conn, err := getSyslogConnection(s.uri)
	if err != nil {
		return err
	}

	s.conn = conn
	_, err = s.conn.Write([]byte(message))
	fmt.Printf("Retried: %v\n", message)
	return err
}

// AfterParse parses the receiver configuration
func (s *Receiver) AfterParse(initArgs seelog.CustomReceiverInitArgs) error {
	var conn net.Conn
	var ok bool
	var err error

	s.enabled = true
	uri, ok := initArgs.XmlCustomAttrs["uri"]
	if ok && uri != "" {
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
