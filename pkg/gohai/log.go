// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package main

import (
	"bytes"
	"fmt"
	"os"

	log "github.com/cihub/seelog"
)

const baseStdErrLogConfig = `<seelog minlevel="loglevel">
	<outputs formatid="common">
		<custom name="stderr" />
	</outputs>
	<formats>
		<format id="common" format="%Msg%n"/>
	</formats>
</seelog>`

// StdErrReceiver is a dummy receiver used to log to stderr instead of stdout.
// See seelog.CustomReceiver.
type StdErrReceiver struct{}

// ReceiveMessage is called when the custom receiver gets seelog message from
// a parent dispatcher.
// See seelog.CustomReceiver.
func (sr *StdErrReceiver) ReceiveMessage(message string, _ log.LogLevel, _ log.LogContextInterface) error {
	fmt.Fprint(os.Stderr, message)
	return nil
}

// AfterParse is called immediately after your custom receiver is instantiated by
// the xml config parser.
// See seelog.CustomReceiver.
func (sr *StdErrReceiver) AfterParse(_ log.CustomReceiverInitArgs) error {
	return nil
}

// Flush is called when the custom receiver gets a 'flush' directive from a
// parent receiver.
// See seelog.CustomReceiver.
func (sr *StdErrReceiver) Flush() {}

// Close is called when the custom receiver gets a 'close' directive from a
// parent receiver.
// See seelog.CustomReceiver.
func (sr *StdErrReceiver) Close() error {
	return nil
}

func initLogging(logLevel string) error {
	log.RegisterReceiver("stderr", &StdErrReceiver{})

	logConfig := bytes.Replace([]byte(baseStdErrLogConfig), []byte("loglevel"), []byte(logLevel), 1)
	logger, err := log.LoggerFromConfigAsBytes(logConfig)
	if err != nil {
		return err
	}

	log.ReplaceLogger(logger)

	return nil
}
