package config

import (
	"fmt"

	log "github.com/cihub/seelog"
)

const logFileMaxSize = 10 * 1024 * 1024 // 10MB

// SetupLogger sets up the default logger
func SetupLogger(logFile string) error {
	configTemplate := `<seelog>
    <outputs>
        <console />
        <rollingfile type="size" filename="%s" maxsize="%d" maxrolls="1" />
    </outputs>
</seelog>`
	config := fmt.Sprintf(configTemplate, logFile, logFileMaxSize)

	logger, err := log.LoggerFromConfigAsString(config)
	if err != nil {
		return err
	}
	log.ReplaceLogger(logger)
	return nil
}
