// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package flare

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigStripApiKey(t *testing.T) {
	assertClean(t,
		`api_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`,
		`api_key: ***************************abbbb`)
	assertClean(t,
		`api_key: AAAAAAAAAAAAAAAAAAAAAAAAAAAABBBB`,
		`api_key: ***************************ABBBB`)
	assertClean(t,
		`api_key: "aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb"`,
		`api_key: "***************************abbbb"`)
	assertClean(t,
		`api_key: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb'`,
		`api_key: '***************************abbbb'`)
	assertClean(t,
		`api_key: |
			aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`,
		`api_key: |
			***************************abbbb`)
	assertClean(t,
		`api_key: >
			aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`,
		`api_key: >
			***************************abbbb`)
	assertClean(t,
		`   api_key:   'aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb'   `,
		`   api_key:   '***************************abbbb'   `)
	assertClean(t,
		`
		additional_endpoints:
			"https://app.datadoghq.com":
			- aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb,
			- bbbbbbbbbbbbbbbbbbbbbbbbbbbbaaaa,
			"https://dog.datadoghq.com":
			- aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb,
			- bbbbbbbbbbbbbbbbbbbbbbbbbbbbaaaa`,
		`
		additional_endpoints:
			"https://app.datadoghq.com":
			- ***************************abbbb,
			- ***************************baaaa,
			"https://dog.datadoghq.com":
			- ***************************abbbb,
			- ***************************baaaa`)
}

func TestConfigStripURLPassword(t *testing.T) {
	assertClean(t,
		`random_url_key: http://user:password@host:port`,
		`random_url_key: http://user:********@host:port`)
	assertClean(t,
		`random_url_key: http://user:p@ssw0r)@host:port`,
		`random_url_key: http://user:********@host:port`)
	assertClean(t,
		`random_url_key: http://user:🔑 🔒 🔐 🔓@host:port`,
		`random_url_key: http://user:********@host:port`)
	assertClean(t,
		`random_url_key: http://user:password@host`,
		`random_url_key: http://user:********@host`)
	assertClean(t,
		`random_url_key: protocol://user:p@ssw0r)@host:port`,
		`random_url_key: protocol://user:********@host:port`)
	assertClean(t,
		`random_url_key: "http://user:password@host:port"`,
		`random_url_key: "http://user:********@host:port"`)
	assertClean(t,
		`random_url_key: 'http://user:password@host:port'`,
		`random_url_key: 'http://user:********@host:port'`)
	assertClean(t,
		`random_url_key: |
			http://user:password@host:port`,
		`random_url_key: |
			http://user:********@host:port`)
	assertClean(t,
		`random_url_key: >
			http://user:password@host:port`,
		`random_url_key: >
			http://user:********@host:port`)
	assertClean(t,
		`   random_url_key:   'http://user:password@host:port'   `,
		`   random_url_key:   'http://user:********@host:port'   `)
}

func TestTextStripApiKey(t *testing.T) {
	assertClean(t,
		`Error status code 500 : http://dog.tld/api?key=3290abeefc68e1bbe852a25252bad88c`,
		`Error status code 500 : http://dog.tld/api?key=***************************ad88c`)
}

func TestTextStripURLPassword(t *testing.T) {
	assertClean(t,
		`Connection droped : ftp://user:password@host:port`,
		`Connection droped : ftp://user:********@host:port`)
}

func TestDockerSelfInspectApiKey(t *testing.T) {
	assertClean(t,
		`
	"Env": [
		"DD_API_KEY=3290abeefc68e1bbe852a25252bad88c",
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"DOCKER_DD_AGENT=yes",
		"AGENT_VERSION=1:6.0",
		"DD_AGENT_HOME=/opt/datadog-agent6/"
	]`,
		`
	"Env": [
		"DD_API_KEY=***************************ad88c",
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"DOCKER_DD_AGENT=yes",
		"AGENT_VERSION=1:6.0",
		"DD_AGENT_HOME=/opt/datadog-agent6/"
	]`)
}

func TestConfigPassword(t *testing.T) {
	assertClean(t,
		`mysql_password: password`,
		`mysql_password: ********`)
	assertClean(t,
		`mysql_pass: password`,
		`mysql_pass: ********`)
	assertClean(t,
		`password_mysql: password`,
		`password_mysql: ********`)
	assertClean(t,
		`mysql_password: p@ssw0r)`,
		`mysql_password: ********`)
	assertClean(t,
		`mysql_password: 🔑 🔒 🔐 🔓`,
		`mysql_password: ********`)
	assertClean(t,
		`mysql_password: password`,
		`mysql_password: ********`)
	assertClean(t,
		`mysql_password: p@ssw0r)`,
		`mysql_password: ********`)
	assertClean(t,
		`mysql_password: "password"`,
		`mysql_password: ********`)
	assertClean(t,
		`mysql_password: 'password'`,
		`mysql_password: ********`)
	assertClean(t,
		`   mysql_password:   'password'   `,
		`   mysql_password: ********`)
}

func TestSNMPConfig(t *testing.T) {
	assertClean(t,
		`community_string: password`,
		`community_string: ********`)
	assertClean(t,
		`authKey: password`,
		`authKey: ********`)
	assertClean(t,
		`privKey: password`,
		`privKey: ********`)
	assertClean(t,
		`community_string: p@ssw0r)`,
		`community_string: ********`)
	assertClean(t,
		`community_string: 🔑 🔒 🔐 🔓`,
		`community_string: ********`)
	assertClean(t,
		`community_string: password`,
		`community_string: ********`)
	assertClean(t,
		`community_string: p@ssw0r)`,
		`community_string: ********`)
	assertClean(t,
		`community_string: "password"`,
		`community_string: ********`)
	assertClean(t,
		`community_string: 'password'`,
		`community_string: ********`)
	assertClean(t,
		`   community_string:   'password'   `,
		`   community_string: ********`)
}

func assertClean(t *testing.T, contents, cleanContents string) {
	cleaned, err := credentialsCleanerBytes([]byte(contents))
	assert.Nil(t, err)
	cleanedString := string(cleaned)

	assert.Equal(t, strings.TrimSpace(cleanContents), strings.TrimSpace(cleanedString))
}
func TestConfig(t *testing.T) {
	assertClean(t,
		`dd_url: https://app.datadoghq.com
api_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
proxy: http://user:password@host:port
password: foo
auth_token: bar
# comment to strip
log_level: info`,
		`dd_url: https://app.datadoghq.com
api_key: ***************************aaaaa
proxy: http://user:********@host:port
password: ********
auth_token: ********
log_level: info`)
}

func TestConfigFile(t *testing.T) {
	cleanedConfigFile := `dd_url: https://app.datadoghq.com
api_key: ***************************aaaaa
proxy: http://user:********@host:port
dogstatsd_port : 8125
log_level: info
`

	wd, _ := os.Getwd()
	filePath := filepath.Join(wd, "test", "datadog.yaml")

	data, err := ioutil.ReadFile(filePath)
	assert.Nil(t, err)

	cleaned, err := credentialsCleanerBytes(data)
	assert.Nil(t, err)
	cleanedString := string(cleaned)

	assert.Equal(t, cleanedConfigFile, cleanedString)
}
