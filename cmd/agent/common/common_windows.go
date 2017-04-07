package common

import (
	log "github.com/cihub/seelog"
)

const defaultConfPath = "c:\\programdata\\datadog"

// ConfigureFileWriter -- set up logging to file
func ConfigureFileWriter() {
	seeConfig := `
<seelog>
	<outputs>
		<rollingfile type="size" filename="c:\\ProgramData\\DataDog\\Logs\\agent.log" maxsize="1000000" maxrolls="2" />
	</outputs>
</seelog>`
	logger, _ := log.LoggerFromConfigAsBytes([]byte(seeConfig))
	log.ReplaceLogger(logger)
}
