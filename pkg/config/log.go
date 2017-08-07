// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	log "github.com/cihub/seelog"
)

const logFileMaxSize = 10 * 1024 * 1024         // 10MB
const logDateFormat = "2006-01-02 15:04:05 MST" // see time.Format for format syntax

// SetupLogger sets up the default logger
func SetupLogger(logLevel, logFile string, syslog bool) error {
	configTemplate := `<seelog minlevel="%s">
    <outputs formatid="common">
        <console />`
	if logFile != "" {
		configTemplate += `<rollingfile type="size" filename="%s" maxsize="%d" maxrolls="1" />`
	}
	if syslog {
		configTemplate += `<custom name="syslog" formatid="syslog" />`
	}
	configTemplate += `</outputs>
    <formats>
        <format id="common" format="%%Date(%s) | %%LEVEL | (%%RelFile:%%Line) | %%Msg%%n"/>`
	if syslog {
		configTemplate += `<format id="syslog" format="%%CustomSyslogHeader(20) %%Msg%%n" />`
	}

	configTemplate += `</formats>
</seelog>`
	config := fmt.Sprintf(configTemplate, strings.ToLower(logLevel), logFile, logFileMaxSize, logDateFormat)

	logger, err := log.LoggerFromConfigAsString(config)
	if err != nil {
		return err
	}
	log.ReplaceLogger(logger)
	return nil
}

// ErrorLogWriter is a Writer that logs all written messages with the global seelog logger
// at an error level
type ErrorLogWriter struct{}

func (s *ErrorLogWriter) Write(p []byte) (n int, err error) {
	log.Error(string(p))
	return len(p), nil
}

var levelToSyslogSeverity = map[log.LogLevel]int{
	// Mapping to RFC 5424 where possible
	log.TraceLvl:    7,
	log.DebugLvl:    7,
	log.InfoLvl:     6,
	log.WarnLvl:     4,
	log.ErrorLvl:    3,
	log.CriticalLvl: 2,
	log.Off:         7,
}

func createSyslogHeaderFormatter(params string) log.FormatterFunc {
	facility := 20
	i, err := strconv.Atoi(params)
	if err == nil && i >= 0 && i <= 23 {
		facility = i
	}

	return func(message string, level log.LogLevel, context log.LogContextInterface) interface{} {
		pid := os.Getpid()
		appName := filepath.Base(os.Args[0])
		hostName, _ := os.Hostname()

		return fmt.Sprintf("<%d>1 %s %s %s %d - -", facility*8+levelToSyslogSeverity[level],
			time.Now().Format("2006-01-02T15:04:05Z07:00"),
			hostName, appName, pid)
	}
}

// SyslogReceiver implements seelog.CustomReceiver
type SyslogReceiver struct {
	conn net.Conn
}

func getSyslogConnection() (net.Conn, error) {
	var conn net.Conn
	var err error

	netNames := []string{"unixgram", "unix"}
	addrs := []string{"/dev/log", "/var/run/syslog", "/var/run/log"}
	for _, netName := range netNames {
		for _, addr := range addrs {
			conn, err = net.Dial(netName, addr)
			if err == nil { // on success
				return conn, nil
			}
		}
	}

	return nil, errors.New("Unable to connect to syslog")
}

// NewSyslogReceiver instantiates SyslogReceiver
func NewSyslogReceiver() *SyslogReceiver {
	// Detect syslog daemon; code derived from Go's own syslog package.
	conn, err := getSyslogConnection()
	if err != nil {
		fmt.Printf("%v\n", err)
		return nil
	}

	return &SyslogReceiver{
		conn: conn,
	}
}

// ReceiveMessage process current log message
func (s *SyslogReceiver) ReceiveMessage(message string, level log.LogLevel, context log.LogContextInterface) error {
	// Implement levels
	if s.conn != nil {
		_, err := s.conn.Write([]byte(message))
		if err == nil {
			fmt.Printf("syslogg'd: %v\n", message)
			return nil
		}
	}

	// try to reconnect
	conn, err := getSyslogConnection()
	if err != nil {
		return err
	}

	s.conn = conn
	_, err = s.conn.Write([]byte(message))
	fmt.Printf("Retried: %v\n", message)
	return err
}

// AfterParse is a NOP in current implementation
func (s *SyslogReceiver) AfterParse(initArgs log.CustomReceiverInitArgs) error {
	return nil
}

// Flush is a NOP in current implementation
func (s *SyslogReceiver) Flush() {
	// Anything to do here?

}

// Close is a NOP in current implementation
func (s *SyslogReceiver) Close() error {
	return nil
}

func init() {
	log.RegisterCustomFormatter("CustomSyslogHeader", createSyslogHeaderFormatter)
	receiver := NewSyslogReceiver()
	if receiver != nil {
		log.RegisterReceiver("syslog", receiver)
		fmt.Print("Registered receiver.")
	}
}
