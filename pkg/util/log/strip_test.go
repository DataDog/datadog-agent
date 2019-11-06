// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package log

import (
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
	// make sure we don't strip container ids
	assertClean(t,
		`container_id: "b32bd6f9b73ba7ccb64953a04b82b48e29dfafab65fd57ca01d3b94a0e024885"`,
		`container_id: "b32bd6f9b73ba7ccb64953a04b82b48e29dfafab65fd57ca01d3b94a0e024885"`)
}

func TestConfigAppKey(t *testing.T) {
	assertClean(t,
		`app_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`,
		`app_key: ***********************************abbbb`)
	assertClean(t,
		`app_key: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABBBB`,
		`app_key: ***********************************ABBBB`)
	assertClean(t,
		`app_key: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb"`,
		`app_key: "***********************************abbbb"`)
	assertClean(t,
		`app_key: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb'`,
		`app_key: '***********************************abbbb'`)
	assertClean(t,
		`app_key: |
			aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`,
		`app_key: |
			***********************************abbbb`)
	assertClean(t,
		`app_key: >
			aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`,
		`app_key: >
			***********************************abbbb`)
	assertClean(t,
		`   app_key:   'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb'   `,
		`   app_key:   '***********************************abbbb'   `)
}

func TestConfigStripURLPassword(t *testing.T) {
	assertClean(t,
		`random_url_key: http://user:password@host:port`,
		`random_url_key: http://user:********@host:port`)
	assertClean(t,
		`random_url_key: http://user:p@ssw0r)@host:port`,
		`random_url_key: http://user:********@host:port`)
	assertClean(t,
		`random_url_key: http://user:🔑🔒🔐🔓@host:port`,
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
		`random_domain_key: 'user:password@host:port'`,
		`random_domain_key: 'user:********@host:port'`)
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
		`Connection dropped : ftp://user:password@host:port`,
		`Connection dropped : ftp://user:********@host:port`)
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
	assertClean(t,
		`pwd: 'password'`,
		`pwd: ********`)
	assertClean(t,
		`pwd: p@ssw0r`,
		`pwd: ********`)
	assertClean(t,
		`cert_key_password: p@ssw0r`,
		`cert_key_password: ********`)
	assertClean(t,
		`cert_key_password: 🔑 🔒 🔐 🔓`,
		`cert_key_password: ********`)
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

func TestYamlConfig(t *testing.T) {
	contents := `foobar: baz`
	cleaned, err := CredentialsCleanerBytes([]byte(contents))
	assert.Nil(t, err)
	cleanedString := string(cleaned)

	// Sanity check
	assert.Equal(t, contents, cleanedString)

	SetStrippedKeys([]string{"community_string", "authKey", "privKey", "foobar"})
	// Restore default value
	defer SetStrippedKeys([]string{"community_string", "authKey", "privKey"})

	assertClean(t, contents, `foobar: ********`)
}

func TestCertConfig(t *testing.T) {
	assertClean(t,
		`cert_key: >
		   -----BEGIN PRIVATE KEY-----
		   MIICdQIBADANBgkqhkiG9w0BAQEFAASCAl8wggJbAgEAAoGBAOLJKRals8tGoy7K
		   ljG6/hMcoe16W6MPn47Q601ttoFkMoSJZ1Jos6nxn32KXfG6hCiB0bmf1iyZtaMa
		   idae/ceT7ZNGvqcVffpDianq9r08hClhnU8mTojl38fsvHf//yqZNzn1ZUcLsY9e
		   wG6wl7CsbWCafxaw+PfaCB1uWlnhAgMBAAECgYAI+tQgrHEBFIvzl1v5HiFfWlvj
		   DlxAiabUvdsDVtvKJdCGRPaNYc3zZbjd/LOZlbwT6ogGZJjTbUau7acVk3gS8uKl
		   ydWWODSuxVYxY8Poxt9SIksOAk5WmtMgIg2bTltTb8z3AWAT3qZrHth03la5Zbix
		   ynEngzyj1+ND7YwQAQJBAP00t8/1aqub+rfza+Ddd8OYSMARFH22oxgy2W1O+Gwc
		   Y8Gn3z6TkadfhPxFaUPnBPx8wm3mN+XeSB1nf0KCAWECQQDlSc7jQ/Ps5rxcoekB
		   ldB+VmuR8TfcWdrWSOdHUiLyoJoj+Z7yfrf70gONPP9tUnwX6MYdT8YwzHK34aWv
		   8KiBAkBHddlql5jDVgIsaEbJ77cdPJ1Ll4Zw9FqTOcajUuZJnLmKrhYTUxKIaize
		   BbjvsQN3Pr6gxZiBB3rS0aLY4lgBAkApsH3ZfKWBUYK2JQpEq4S5M+VjJ8TMX9oW
		   VDMZGKoaC3F7UQvBc6DoPItAxvJ6YiEGB+Ddu3+Bp+rD3FdP4iYBAkBh17O56A/f
		   QX49RjRCRIT0w4nvZ3ph9gHEe50E4+Ky5CLQNOPLD/RbBXSEzez8cGysVvzDO3DZ
		   /iN4a8gloY3d
		   -----END PRIVATE KEY-----`,
		`cert_key: >
		   ********`)
	assertClean(t,
		`cert_key: |
			-----BEGIN CERTIFICATE-----
			MIICdQIBADANBgkqhkiG9w0BAQEFAASCAl8wggJbAgEAAoGBAOLJKRals8tGoy7K
			ljG6/hMcoe16W6MPn47Q601ttoFkMoSJZ1Jos6nxn32KXfG6hCiB0bmf1iyZtaMa
			idae/ceT7ZNGvqcVffpDianq9r08hClhnU8mTojl38fsvHf//yqZNzn1ZUcLsY9e
			wG6wl7CsbWCafxaw+PfaCB1uWlnhAgMBAAECgYAI+tQgrHEBFIvzl1v5HiFfWlvj
			DlxAiabUvdsDVtvKJdCGRPaNYc3zZbjd/LOZlbwT6ogGZJjTbUau7acVk3gS8uKl
			ydWWODSuxVYxY8Poxt9SIksOAk5WmtMgIg2bTltTb8z3AWAT3qZrHth03la5Zbix
			ynEngzyj1+ND7YwQAQJBAP00t8/1aqub+rfza+Ddd8OYSMARFH22oxgy2W1O+Gwc
			Y8Gn3z6TkadfhPxFaUPnBPx8wm3mN+XeSB1nf0KCAWECQQDlSc7jQ/Ps5rxcoekB
			ldB+VmuR8TfcWdrWSOdHUiLyoJoj+Z7yfrf70gONPP9tUnwX6MYdT8YwzHK34aWv
			8KiBAkBHddlql5jDVgIsaEbJ77cdPJ1Ll4Zw9FqTOcajUuZJnLmKrhYTUxKIaize
			BbjvsQN3Pr6gxZiBB3rS0aLY4lgBAkApsH3ZfKWBUYK2JQpEq4S5M+VjJ8TMX9oW
			VDMZGKoaC3F7UQvBc6DoPItAxvJ6YiEGB+Ddu3+Bp+rD3FdP4iYBAkBh17O56A/f
			QX49RjRCRIT0w4nvZ3ph9gHEe50E4+Ky5CLQNOPLD/RbBXSEzez8cGysVvzDO3DZ
			/iN4a8gloY3d
			-----END CERTIFICATE-----`,
		`cert_key: |
			********`)
}

func assertClean(t *testing.T, contents, cleanContents string) {
	cleaned, err := CredentialsCleanerBytes([]byte(contents))
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
log_level: info`

	wd, _ := os.Getwd()
	filePath := filepath.Join(wd, "test", "datadog.yaml")
	cleaned, err := CredentialsCleanerFile(filePath)
	assert.Nil(t, err)
	cleanedString := string(cleaned)

	assert.Equal(t, cleanedConfigFile, cleanedString)
}
