// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package flare

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanConfig(t *testing.T) {
	configFile := `dd_url: https://app.datadoghq.com
api_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
proxy: http://user:password@host:port
password: foo
auth_token: bar
# comment to strip
log_level: info
`
	cleanedConfigFile := `dd_url: https://app.datadoghq.com
api_key: **************************aaaaa
proxy: http://user:********@host:port
password: ********
auth_token: ********
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
	filePath := filepath.Join(wd, "test", "datadog.yaml")
	cleaned, err := credentialsCleanerFile(filePath)
	assert.Nil(t, err)
	cleanedString := string(cleaned)

	assert.Equal(t, cleanedConfigFile, cleanedString)
}
