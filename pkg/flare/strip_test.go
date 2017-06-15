package flare

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanConfig(t *testing.T) {
	configFile := `dd_url: https://app.datadoghq.com
api_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
proxy: http://user:password@host:port
# comment to strip
log_level: info
`
	cleanedConfigFile := `dd_url: https://app.datadoghq.com
api_key: **************************aaaaa
proxy: http://user:********@host:port
log_level: info
`

	cleaned, err := credentialsCleanerBytes([]byte(configFile))
	assert.Nil(t, err)
	cleanedString := string(cleaned)

	assert.Equal(t, cleanedConfigFile, cleanedString)

}

func TestCleanConfigFile(t *testing.T) {
	cleanedConfigFile := `dd_url: https://app.datadoghq.com
api_key: **************************aaaaa
proxy: http://user:********@host:port
dogstatsd_port : 8125
log_level: info
`

	wd, _ := os.Getwd()
	filePath := path.Join(wd, "test", "datadog.yaml")
	cleaned, err := credentialsCleanerFile(filePath)
	assert.Nil(t, err)
	cleanedString := string(cleaned)

	assert.Equal(t, cleanedConfigFile, cleanedString)
}
