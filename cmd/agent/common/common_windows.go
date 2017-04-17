package common

import (
	"path/filepath"

	log "github.com/cihub/seelog"
	"golang.org/x/sys/windows/registry"
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

// UpdateDistPath If necessary, change the DistPath variable to the right location
func UpdateDistPath() {
	// fetch the installation path from the registry
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\DataDog\Datadog Agent`, registry.QUERY_VALUE)
	if err != nil {
		log.Warn("Failed to open registry key %s", err)
		return
	}
	defer k.Close()
	s, _, err := k.GetStringValue("InstallPath")
	if err != nil {
		log.Warn("Installpath not found in registry %s", err)
		return
	}
	DistPath = filepath.Join(s, `bin/agent/dist`)
	log.Debug("DisPath is now %s", DistPath)
	return
}
