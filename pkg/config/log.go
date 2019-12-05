// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package config

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"
)

// LoggerName specifies the name of an instantiated logger.
type LoggerName string

const logDateFormat = "2006-01-02 15:04:05 MST" // see time.Format for format syntax

var syslogTLSConfig *tls.Config

// BuildCommonFormat returns the log common format seelog string
func BuildCommonFormat(loggerName LoggerName) string {
	return fmt.Sprintf("%%Date(%s) | %s | %%LEVEL | (%%ShortFilePath:%%Line in %%FuncShort) | %%Msg%%n", logDateFormat, loggerName)
}

func createQuoteMsgFormatter(params string) seelog.FormatterFunc {
	return func(message string, level seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		return strconv.Quote(message)
	}
}

// BuildJSONFormat returns the log JSON format seelog string
func BuildJSONFormat(loggerName LoggerName) string {
	seelog.RegisterCustomFormatter("QuoteMsg", createQuoteMsgFormatter)
	return fmt.Sprintf("{&quot;agent&quot;:&quot;%s&quot;,&quot;time&quot;:&quot;%%Date(%s)&quot;,&quot;level&quot;:&quot;%%LEVEL&quot;,&quot;file&quot;:&quot;%%ShortFilePath&quot;,&quot;line&quot;:&quot;%%Line&quot;,&quot;func&quot;:&quot;%%FuncShort&quot;,&quot;msg&quot;:%%QuoteMsg}%%n", strings.ToLower(string(loggerName)), logDateFormat)
}

func getSyslogTLSKeyPair() (*tls.Certificate, error) {
	var syslogTLSKeyPair *tls.Certificate
	if Datadog.IsSet("syslog_pem") && Datadog.IsSet("syslog_key") {
		cert := Datadog.GetString("syslog_pem")
		key := Datadog.GetString("syslog_key")

		if cert == "" && key == "" {
			return nil, nil
		} else if cert == "" || key == "" {
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

// SetupLogger sets up a logger with the specified logger name and log level
// if a non empty logFile is provided, it will also log to the file
// a non empty syslogURI will enable syslog, and format them following RFC 5424 if specified
// you can also specify to log to the console and in JSON format
func SetupLogger(loggerName LoggerName, logLevel, logFile, syslogURI string, syslogRFC, logToConsole, jsonFormat bool) error {
	var syslog bool
	var useTLS bool

	if syslogURI != "" { // non-blank uri enables syslog
		syslog = true

		syslogTLSKeyPair, err := getSyslogTLSKeyPair()
		if err != nil {
			return err
		}

		if syslogTLSKeyPair != nil {
			useTLS = true
			syslogTLSConfig = &tls.Config{
				Certificates:       []tls.Certificate{*syslogTLSKeyPair},
				InsecureSkipVerify: Datadog.GetBool("syslog_tls_verify"),
			}
		}
	}

	seelogLogLevel := strings.ToLower(logLevel)
	if seelogLogLevel == "warning" { // Common gotcha when used to agent5
		seelogLogLevel = "warn"
	}

	configTemplate := fmt.Sprintf(`<seelog minlevel="%s">`, seelogLogLevel)

	formatID := "common"
	if jsonFormat {
		formatID = "json"
	}

	configTemplate += fmt.Sprintf(`<outputs formatid="%s">`, formatID)

	if logToConsole {
		configTemplate += `<console />`
	}
	if logFile != "" {
		configTemplate += fmt.Sprintf(`<rollingfile type="size" filename="%s" maxsize="%d" maxrolls="%d" />`, logFile, Datadog.GetSizeInBytes("log_file_max_size"), Datadog.GetInt("log_file_max_rolls"))
	}
	if syslog {
		var syslogTemplate string
		if syslogURI != "" {
			syslogTemplate = fmt.Sprintf(
				`<custom name="syslog" formatid="syslog-%s" data-uri="%s" data-tls="%v" />`,
				formatID,
				syslogURI,
				useTLS,
			)
		} else {
			syslogTemplate = fmt.Sprintf(`<custom name="syslog" formatid="syslog-%s" />`, formatID)
		}
		configTemplate += syslogTemplate
	}

	configTemplate += fmt.Sprintf(`</outputs>
	<formats>
		<format id="json" format="%s"/>
		<format id="common" format="%s"/>
		<format id="syslog-json" format="%%CustomSyslogHeader(20,`+strconv.FormatBool(syslogRFC)+`){&quot;agent&quot;:&quot;%s&quot;,&quot;level&quot;:&quot;%%LEVEL&quot;,&quot;relfile&quot;:&quot;%%ShortFilePath&quot;,&quot;line&quot;:&quot;%%Line&quot;,&quot;msg&quot;:&quot;%%Msg&quot;}%%n"/>
		<format id="syslog-common" format="%%CustomSyslogHeader(20,`+strconv.FormatBool(syslogRFC)+`) %s | %%LEVEL | (%%ShortFilePath:%%Line in %%FuncShort) | %%Msg%%n" />
	</formats>
</seelog>`,
		BuildJSONFormat(loggerName),
		BuildCommonFormat(loggerName),
		strings.ToLower(string(loggerName)),
		loggerName,
	)

	logger, err := seelog.LoggerFromConfigAsString(configTemplate)
	if err != nil {
		return err
	}
	seelog.ReplaceLogger(logger)

	log.SetupDatadogLogger(logger, seelogLogLevel)
	log.AddStrippedKeys(Datadog.GetStringSlice("flare_stripped_keys"))
	return nil
}

// ErrorLogWriter is a Writer that logs all written messages with the global seelog logger
// at an error level
type ErrorLogWriter struct {
	AdditionalDepth int
}

func (s *ErrorLogWriter) Write(p []byte) (n int, err error) {
	log.ErrorStackDepth(s.AdditionalDepth, strings.TrimSpace(string(p)))
	return len(p), nil
}

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

func createSyslogHeaderFormatter(params string) seelog.FormatterFunc {
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
		return func(message string, level seelog.LogLevel, context seelog.LogContextInterface) interface{} {
			return fmt.Sprintf("<%d>1 %s %d - -", facility*8+levelToSyslogSeverity[level], appName, pid)
		}
	}

	// otherwise old-school logging
	return func(message string, level seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		return fmt.Sprintf("<%d>%s[%d]:", facility*8+levelToSyslogSeverity[level], appName, pid)
	}
}

// SyslogReceiver implements seelog.CustomReceiver
type SyslogReceiver struct {
	enabled bool
	uri     *url.URL
	tls     bool
	conn    net.Conn
}

func getSyslogConnection(uri *url.URL, secure bool) (net.Conn, error) {
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
			fmt.Printf("Trying to connecto to: %s", uri.Path)
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

// ReceiveMessage process current log message
func (s *SyslogReceiver) ReceiveMessage(message string, level seelog.LogLevel, context seelog.LogContextInterface) error {
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
	conn, err := getSyslogConnection(s.uri, s.tls)
	if err != nil {
		return err
	}

	s.conn = conn
	_, err = s.conn.Write([]byte(message))
	fmt.Printf("Retried: %v\n", message)
	return err
}

// AfterParse parses the receiver configuration
func (s *SyslogReceiver) AfterParse(initArgs seelog.CustomReceiverInitArgs) error {
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

	tls, ok := initArgs.XmlCustomAttrs["tls"]
	if ok {
		// if certificate specified it should already be in pool
		if tls == "true" {
			s.tls = true
		}
	}

	if !s.enabled {
		return errors.New("bad syslog receiver configuration - disabling")
	}

	conn, err = getSyslogConnection(s.uri, s.tls)
	if err != nil {
		fmt.Printf("%v\n", err)
		return nil
	}
	s.conn = conn

	return nil
}

// Flush is a NOP in current implementation
func (s *SyslogReceiver) Flush() {
	// Nothing to do here...
}

// Close is a NOP in current implementation
func (s *SyslogReceiver) Close() error {
	return nil
}

func parseShortFilePath(params string) seelog.FormatterFunc {
	return func(message string, level seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		return extractShortPathFromFullPath(context.FullPath())
	}
}

func extractShortPathFromFullPath(fullPath string) string {
	// We want to trim the part containing the path of the project
	// ie DataDog/datadog-agent/ or DataDog/datadog-process-agent/
	slices := strings.Split(fullPath, "-agent/")
	return slices[len(slices)-1]
}

func init() {
	seelog.RegisterCustomFormatter("CustomSyslogHeader", createSyslogHeaderFormatter)
	seelog.RegisterCustomFormatter("ShortFilePath", parseShortFilePath)
	seelog.RegisterReceiver("syslog", &SyslogReceiver{})
}
