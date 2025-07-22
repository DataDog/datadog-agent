// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package syslog implements a syslog handler for the datadog agent.
package syslog

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log/formatters"
	"github.com/DataDog/datadog-agent/pkg/util/log/handlers"
	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

// Receiver implements seelog.CustomReceiver
type Receiver struct {
	uri             *url.URL
	conn            net.Conn
	syslogTLSConfig *tls.Config
	useTLS          bool

	loggerName    string
	writeHeader   func(*bytes.Buffer, slog.Level)
	formatMessage func(*bytes.Buffer, slog.Record)
}

// NewReceiver creates a new syslog receiver
func NewReceiver(loggerName string, jsonFormat bool, syslogURI string, syslogPem string, syslogKey string, syslogRFC bool, syslogTLSVerify bool) (*Receiver, error) {
	s := &Receiver{
		loggerName: loggerName,
	}

	if jsonFormat {
		s.formatMessage = s.formatJSONMessage
	} else {
		s.formatMessage = s.formatTextMessage
	}

	if syslogURI != "" { // non-blank uri enables syslog
		syslogTLSKeyPair, err := getSyslogTLSKeyPair(syslogPem, syslogKey)
		if err != nil {
			return nil, err
		}
		if syslogTLSKeyPair != nil {
			s.useTLS = true
			s.syslogTLSConfig = &tls.Config{
				Certificates:       []tls.Certificate{*syslogTLSKeyPair},
				InsecureSkipVerify: syslogTLSVerify, // TODO: this is clearly incorrect...
			}
		}
	}

	if syslogURI != "" {
		url, err := url.ParseRequestURI(syslogURI)
		if err != nil {
			return nil, errors.New("bad syslog receiver configuration - disabling")
		}

		s.uri = url
	}

	conn, err := getSyslogConnection(s.syslogTLSConfig, s.uri, s.useTLS)
	if err != nil {
		fmt.Printf("%v\n", err)
		return s, nil
	}
	s.conn = conn

	s.writeHeader = getHeaderFormatter(20, syslogRFC)

	return s, nil
}

// Handler returns a slog.Handler that writes to the syslog connection
func (s *Receiver) Handler() slog.Handler {
	return handlers.NewFormatHandler(s.format, s)
}

// format formats the log record into a string
func (s *Receiver) format(_ context.Context, record slog.Record) string {
	var buff bytes.Buffer

	s.writeHeader(&buff, record.Level)
	_ = buff.WriteByte(' ')
	s.formatMessage(&buff, record)

	return buff.String()
}

func (s *Receiver) formatTextMessage(buff *bytes.Buffer, record slog.Record) {
	frames := runtime.CallersFrames([]uintptr{record.PC})
	frame, _ := frames.Next()

	level := formatters.LevelToString(record.Level)
	shortFilePath := formatters.ShortFilePath(frame)
	extraTextContext := formatters.ExtraTextContext(record)

	funcShort := frame.Function[strings.LastIndexByte(frame.Function, '.')+1:]

	fmt.Fprintf(buff, `%s | %s | (%s:%d in %s) | %s%s
`, s.loggerName, level, shortFilePath, frame.Line, funcShort, extraTextContext, record.Message)
}

func (s *Receiver) formatJSONMessage(buff *bytes.Buffer, record slog.Record) {
	frames := runtime.CallersFrames([]uintptr{record.PC})
	frame, _ := frames.Next()

	level := formatters.LevelToString(record.Level)
	shortFilePath := formatters.ShortFilePath(frame)
	extraJSONContext := formatters.ExtraJSONContext(record)

	fmt.Fprintf(buff, `{"agent":"%s","level":"%s","relfile":"%s","line":"%d","msg":"%s", %s}
`, strings.ToLower(s.loggerName), level, shortFilePath, frame.Line, record.Message, extraJSONContext)
}

// Write writes the message to the syslog connection
func (s *Receiver) Write(message []byte) (int, error) {
	if s.conn != nil {
		n, err := s.conn.Write([]byte(message))
		if err == nil {
			return n, nil
		}
	}

	// try to reconnect - close the connection first just in case
	//                    we don't want fd leaks here.
	if s.conn != nil {
		s.conn.Close()
	}
	conn, err := getSyslogConnection(s.syslogTLSConfig, s.uri, s.useTLS)
	if err != nil {
		return 0, err
	}

	s.conn = conn
	n, err := s.conn.Write([]byte(message))
	fmt.Printf("Retried: %v\n", message)
	return n, err
}

// Close closes the syslog connection
func (s *Receiver) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

func getSyslogTLSKeyPair(cert string, key string) (*tls.Certificate, error) {
	var syslogTLSKeyPair *tls.Certificate
	if cert != "" && key != "" {
		if cert == "" || key == "" {
			return nil, fmt.Errorf("Both a PEM certificate and key must be specified to enable TLS")
		}

		keypair, err := tls.LoadX509KeyPair(cert, key)
		if err != nil {
			return nil, err
		}

		syslogTLSKeyPair = &keypair
	}

	return syslogTLSKeyPair, nil
}

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

func getHeaderFormatter(facility int, syslogRFC bool) func(*bytes.Buffer, slog.Level) {
	defaultFacility := 20

	if facility < 0 || facility > 23 {
		facility = defaultFacility
	}

	pid := os.Getpid()
	appName := filepath.Base(os.Args[0])

	if syslogRFC { // RFC 5424
		return func(buff *bytes.Buffer, level slog.Level) {
			fmt.Fprintf(buff, "<%d>1 %s %d - -", facility*8+levelToSyslogSeverity[types.LogLevel(level)], appName, pid)
		}
	}

	// otherwise old-school logging
	return func(buff *bytes.Buffer, level slog.Level) {
		fmt.Fprintf(buff, "<%d>%s[%d]:", facility*8+levelToSyslogSeverity[types.LogLevel(level)], appName, pid)
	}
}

func getSyslogConnection(syslogTLSConfig *tls.Config, uri *url.URL, secure bool) (net.Conn, error) {
	var conn net.Conn
	var err error

	// local
	localNetNames := []string{"unixgram", "unix"}
	if uri == nil {
		addrs := []string{"/dev/log", "/var/run/syslog", "/var/run/log"}
		for _, netName := range localNetNames {
			for _, addr := range addrs {
				conn, err = net.Dial(netName, addr)
				if err == nil { // on success
					return conn, nil
				}
			}
		}
	} else {
		switch uri.Scheme {
		case "unix", "unixgram":
			fmt.Printf("Trying to connect to: %s", uri.Path)
			for _, netName := range localNetNames {
				conn, err = net.Dial(netName, uri.Path)
				if err == nil {
					break
				}
			}
		case "udp":
			conn, err = net.Dial(uri.Scheme, uri.Host)
		case "tcp":
			if secure {
				conn, err = tls.Dial("tcp", uri.Host, syslogTLSConfig)
			} else {
				conn, err = net.Dial("tcp", uri.Host)
			}
		}
		if err == nil {
			return conn, nil
		}
	}

	return nil, errors.New("Unable to connect to syslog")
}
