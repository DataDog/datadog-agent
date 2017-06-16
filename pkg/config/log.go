package config

import (
	"fmt"
	"strings"

	log "github.com/cihub/seelog"
)

const logFileMaxSize = 10 * 1024 * 1024         // 10MB
const logDateFormat = "2006-01-02 15:04:05 MST" // see time.Format for format syntax

// SetupLogger sets up the default logger
func SetupLogger(logLevel, logFile string) error {
	configTemplate := `<seelog minlevel="%s">
    <outputs formatid="common">
        <console />`
	if logFile != "" {
		configTemplate += `<rollingfile type="size" filename="%s" maxsize="%d" maxrolls="1" />`
	}
	configTemplate += `</outputs>
    <formats>
        <format id="common" format="%%Date(%s) | %%LEVEL | (%%RelFile:%%Line) | %%Msg%%n"/>
    </formats>
</seelog>`
	config := fmt.Sprintf(configTemplate, strings.ToLower(logLevel), logFile, logFileMaxSize, logDateFormat)

	logger, err := log.LoggerFromConfigAsString(config)
	if err != nil {
		return err
	}
	log.ReplaceLogger(logger)
	return nil
}
