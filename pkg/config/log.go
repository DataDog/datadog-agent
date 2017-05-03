package config

import (
	"fmt"
	"strings"

	log "github.com/cihub/seelog"
)

const defaultLogLevel = "info"
const logFileMaxSize = 10 * 1024 * 1024 // 10MB

func init() {
	Datadog.SetDefault("log_level", defaultLogLevel)
}

// SetupLogger sets up the default logger
func SetupLogger(logLevel, logFile string) error {
	configTemplate := `<seelog minlevel="%s">
    <outputs>
        <console />
        <rollingfile type="size" filename="%s" maxsize="%d" maxrolls="1" />
    </outputs>
</seelog>`
	config := fmt.Sprintf(configTemplate, strings.ToLower(logLevel), logFile, logFileMaxSize)

	logger, err := log.LoggerFromConfigAsString(config)
	if err != nil {
		return err
	}
	log.ReplaceLogger(logger)
	return nil
}
