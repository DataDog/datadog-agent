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

type StdErrReceiver struct{}

// Implement seelog.CustomReceiver to log to stderr instead of stdout
func (sr *StdErrReceiver) ReceiveMessage(message string, level log.LogLevel, context log.LogContextInterface) error {
	fmt.Fprintf(os.Stderr, message)
	return nil
}

func (sr *StdErrReceiver) AfterParse(initArgs log.CustomReceiverInitArgs) error {
	return nil
}

func (sr *StdErrReceiver) Flush() {}

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
