// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package flare

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigStripApiKey(t *testing.T) {
	assertClean(t,
		`api_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`,
		`api_key: ***************************abbbb`)
	assertClean(t,
		`api_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`,
		`api_key: ***************************abbbb`)
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

func assertClean(t *testing.T, contents, cleanContents string) {
	cleaned, err := credentialsCleanerBytes([]byte(contents))
	assert.Nil(t, err)
	cleanedString := string(cleaned)

	assert.Equal(t, strings.TrimSpace(cleanContents), strings.TrimSpace(cleanedString))
}
