// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func assertClean(t *testing.T, contents, cleanContents string) {
	cleaned, err := ScrubBytes([]byte(contents))
	assert.Nil(t, err)
	cleanedString := string(cleaned)

	assert.Equal(t, strings.TrimSpace(cleanContents), strings.TrimSpace(cleanedString))
}

func TestConfigScrubbedValidYaml(t *testing.T) {
	wd, _ := os.Getwd()

	inputConf := filepath.Join(wd, "test", "conf.yaml")
	inputConfData, err := os.ReadFile(inputConf)
	require.NoError(t, err)

	outputConf := filepath.Join(wd, "test", "conf_scrubbed.yaml")
	outputConfData, err := os.ReadFile(outputConf)
	require.NoError(t, err)

	cleaned, err := ScrubBytes([]byte(inputConfData))
	require.Nil(t, err)

	// First test that the a scrubbed yaml is still a valid yaml
	var out interface{}
	err = yaml.Unmarshal(cleaned, &out)
	assert.NoError(t, err, "Could not load YAML configuration after being scrubbed")

	// We replace windows line break by linux so the tests pass on every OS
	trimmedOutput := strings.TrimSpace(strings.Replace(string(outputConfData), "\r\n", "\n", -1))
	trimmedCleaned := strings.TrimSpace(strings.Replace(string(cleaned), "\r\n", "\n", -1))

	assert.Equal(t, trimmedOutput, trimmedCleaned)
}

func TestConfigScrubbedYaml(t *testing.T) {
	wd, _ := os.Getwd()

	inputConf := filepath.Join(wd, "test", "conf_multiline.yaml")
	inputConfData, err := os.ReadFile(inputConf)
	require.NoError(t, err)

	outputConf := filepath.Join(wd, "test", "conf_multiline_scrubbed.yaml")
	outputConfData, err := os.ReadFile(outputConf)
	require.NoError(t, err)

	cleaned, err := ScrubYaml([]byte(inputConfData))
	require.Nil(t, err)

	// First test that the a scrubbed yaml is still a valid yaml
	var out interface{}
	err = yaml.Unmarshal(cleaned, &out)
	assert.NoError(t, err, "Could not load YAML configuration after being scrubbed")

	// We replace windows line break by linux so the tests pass on every OS
	trimmedOutput := strings.TrimSpace(strings.Replace(string(outputConfData), "\r\n", "\n", -1))
	trimmedCleaned := strings.TrimSpace(strings.Replace(string(cleaned), "\r\n", "\n", -1))
	assert.Equal(t, trimmedOutput, trimmedCleaned)
}

func TestConfigScrubbedJson(t *testing.T) {
	wd, _ := os.Getwd()

	inputConf := filepath.Join(wd, "test", "config.json")
	inputConfData, err := os.ReadFile(inputConf)
	require.NoError(t, err)
	cleaned, err := ScrubJSON([]byte(inputConfData))
	require.Nil(t, err)
	// First test that the a scrubbed json is still valid
	var actualOutJSON interface{}
	err = json.Unmarshal(cleaned, &actualOutJSON)
	assert.NoError(t, err, "Could not load JSON configuration after being scrubbed")

	outputConf := filepath.Join(wd, "test", "config_scrubbed.json")
	outputConfData, err := os.ReadFile(outputConf)
	require.NoError(t, err)
	var expectedOutJSON interface{}
	err = json.Unmarshal(outputConfData, &expectedOutJSON)
	require.NoError(t, err)
	outputConfData, err = json.Marshal(expectedOutJSON)
	require.NoError(t, err)
	assert.Equal(t, cleaned, outputConfData)
}

func TestEmptyYaml(t *testing.T) {
	cleaned, err := ScrubYaml(nil)
	require.Nil(t, err)
	assert.Equal(t, "", string(cleaned))

	cleaned, err = ScrubYaml([]byte(""))
	require.Nil(t, err)
	assert.Equal(t, "", string(cleaned))
}

func TestEmptyYamlString(t *testing.T) {
	cleaned, err := ScrubYamlString("")
	require.Nil(t, err)
	assert.Equal(t, "", string(cleaned))
}

func TestConfigStripApiKey(t *testing.T) {
	assertClean(t,
		`api_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`,
		`api_key: "***************************abbbb"`)
	assertClean(t,
		`api_key: AAAAAAAAAAAAAAAAAAAAAAAAAAAABBBB`,
		`api_key: "***************************ABBBB"`)
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
			- "***************************abbbb",
			- "***************************baaaa",
			"https://dog.datadoghq.com":
			- "***************************abbbb",
			- "***************************baaaa"`)
	// make sure we don't strip container ids
	assertClean(t,
		`container_id: "b32bd6f9b73ba7ccb64953a04b82b48e29dfafab65fd57ca01d3b94a0e024885"`,
		`container_id: "b32bd6f9b73ba7ccb64953a04b82b48e29dfafab65fd57ca01d3b94a0e024885"`)
}

func TestConfigAppKey(t *testing.T) {
	assertClean(t,
		`app_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`,
		`app_key: "***********************************abbbb"`)
	assertClean(t,
		`app_key: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABBBB`,
		`app_key: "***********************************ABBBB"`)
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

func TestConfigRCAppKey(t *testing.T) {
	assertClean(t,
		`key: "DDRCM_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABCDE"`,
		`key: "***********************************ABCDE"`)
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
	assertClean(t,
		`   random_url_key:   'mongodb+s.r-v://user:password@host:port'   `,
		`   random_url_key:   'mongodb+s.r-v://user:********@host:port'   `)
	assertClean(t,
		`   random_url_key:   'mongodb+srv://user:pass-with-hyphen@abc.example.com/database'   `,
		`   random_url_key:   'mongodb+srv://user:********@abc.example.com/database'   `)
	assertClean(t,
		`   random_url_key:   'http://user-with-hyphen:pass-with-hyphen@abc.example.com/database'   `,
		`   random_url_key:   'http://user-with-hyphen:********@abc.example.com/database'   `)
	assertClean(t,
		`   random_url_key:   'http://user-with-hyphen:pass@abc.example.com/database'   `,
		`   random_url_key:   'http://user-with-hyphen:********@abc.example.com/database'   `)

	assertClean(t,
		`flushing serie: {"metric":"kubeproxy","tags":["image_id":"foobar/foobaz@sha256:e8dabc7d398d25ecc8a3e33e3153e988e79952f8783b81663feb299ca2d0abdd"]}`,
		`flushing serie: {"metric":"kubeproxy","tags":["image_id":"foobar/foobaz@sha256:e8dabc7d398d25ecc8a3e33e3153e988e79952f8783b81663feb299ca2d0abdd"]}`)

	assertClean(t,
		`"simple.metric:44|g|@1.00000"`,
		`"simple.metric:44|g|@1.00000"`)
}

func TestTextStripApiKey(t *testing.T) {
	assertClean(t,
		`Error status code 500 : http://dog.tld/api?key=3290abeefc68e1bbe852a25252bad88c`,
		`Error status code 500 : http://dog.tld/api?key=***************************ad88c`)
	assertClean(t,
		`hintedAPIKeyReplacer : http://dog.tld/api_key=InvalidLength12345abbbb`,
		`hintedAPIKeyReplacer : http://dog.tld/api_key=***************************abbbb`)
	assertClean(t,
		`hintedAPIKeyReplacer : http://dog.tld/apikey=InvalidLength12345abbbb`,
		`hintedAPIKeyReplacer : http://dog.tld/apikey=***************************abbbb`)
	assertClean(t,
		`apiKeyReplacer: https://agent-http-intake.logs.datadoghq.com/v1/input/aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`,
		`apiKeyReplacer: https://agent-http-intake.logs.datadoghq.com/v1/input/***************************abbbb`)
}

func TestTextStripAppKey(t *testing.T) {
	assertClean(t,
		`hintedAPPKeyReplacer : http://dog.tld/app_key=InvalidLength12345abbbb`,
		`hintedAPPKeyReplacer : http://dog.tld/app_key=***********************************abbbb`)
	assertClean(t,
		`hintedAPPKeyReplacer : http://dog.tld/appkey=InvalidLength12345abbbb`,
		`hintedAPPKeyReplacer : http://dog.tld/appkey=***********************************abbbb`)
	assertClean(t,
		`hintedAPPKeyReplacer : http://dog.tld/application_key=InvalidLength12345abbbb`,
		`hintedAPPKeyReplacer : http://dog.tld/application_key=***********************************abbbb`)
	assertClean(t,
		`appKeyReplacer: http://dog.tld/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`,
		`appKeyReplacer: http://dog.tld/***********************************abbbb`)
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
		`mysql_password: "********"`)
	assertClean(t,
		`mysql_pass: password`,
		`mysql_pass: "********"`)
	assertClean(t,
		`password_mysql: password`,
		`password_mysql: "********"`)
	assertClean(t,
		`mysql_password: p@ssw0r)`,
		`mysql_password: "********"`)
	assertClean(t,
		`mysql_password: 🔑 🔒 🔐 🔓`,
		`mysql_password: "********"`)
	assertClean(t,
		`mysql_password: password`,
		`mysql_password: "********"`)
	assertClean(t,
		`mysql_password: p@ssw0r)`,
		`mysql_password: "********"`)
	assertClean(t,
		`mysql_password: "password"`,
		`mysql_password: "********"`)
	assertClean(t,
		`mysql_password: 'password'`,
		`mysql_password: "********"`)
	assertClean(t,
		`   mysql_password:   'password'   `,
		`   mysql_password: "********"`)
	assertClean(t,
		`pwd: 'password'`,
		`pwd: "********"`)
	assertClean(t,
		`pwd: p@ssw0r`,
		`pwd: "********"`)
	assertClean(t,
		`cert_key_password: p@ssw0r`,
		`cert_key_password: "********"`)
	assertClean(t,
		`cert_key_password: 🔑 🔒 🔐 🔓`,
		`cert_key_password: "********"`)
}

func TestSNMPConfig(t *testing.T) {
	assertClean(t,
		`community_string: password`,
		`community_string: "********"`)
	assertClean(t,
		`authKey: password`,
		`authKey: "********"`)
	assertClean(t,
		`privKey: password`,
		`privKey: "********"`)
	assertClean(t,
		`community_string: p@ssw0r)`,
		`community_string: "********"`)
	assertClean(t,
		`community_string: 🔑 🔒 🔐 🔓`,
		`community_string: "********"`)
	assertClean(t,
		`community_string: password`,
		`community_string: "********"`)
	assertClean(t,
		`community_string: p@ssw0r)`,
		`community_string: "********"`)
	assertClean(t,
		`community_string: "password"`,
		`community_string: "********"`)
	assertClean(t,
		`community_string: 'password'`,
		`community_string: "********"`)
	assertClean(t,
		`   community_string:   'password'   `,
		`   community_string: "********"`)
	assertClean(t,
		`
network_devices:
  snmp_traps:
    community_strings:
		- 'password1'
		- 'password2'
other_config: 1
other_config_with_list: [abc]
`,
		`
network_devices:
  snmp_traps:
    community_strings: "********"
other_config: 1
other_config_with_list: [abc]
`)
	assertClean(t,
		`
network_devices:
  snmp_traps:
    community_strings: ['password1', 'password2']
other_config: 1
other_config_with_list: [abc]
`,
		`
network_devices:
  snmp_traps:
    community_strings: "********"
other_config: 1
other_config_with_list: [abc]
`)
	assertClean(t,
		`
network_devices:
  snmp_traps:
    community_strings: []
other_config: 1
other_config_with_list: [abc]
`,
		`
network_devices:
  snmp_traps:
    community_strings: "********"
other_config: 1
other_config_with_list: [abc]
`)
	assertClean(t,
		`
network_devices:
  snmp_traps:
    community_strings: [
   'password1',
   'password2']
other_config: 1
other_config_with_list: [abc]
`,
		`
network_devices:
  snmp_traps:
    community_strings: "********"
other_config: 1
other_config_with_list: [abc]
`)
	assertClean(t,
		`
snmp_traps_config:
  community_strings:
  - 'password1'
  - 'password2'
other_config: 1
other_config_with_list: [abc]
`,
		`snmp_traps_config:
  community_strings: "********"
other_config: 1
other_config_with_list: [abc]
`)
	assertClean(t,
		`community: password`,
		`community: "********"`)
	assertClean(t,
		`authentication_key: password`,
		`authentication_key: "********"`)
	assertClean(t,
		`privacy_key: password`,
		`privacy_key: "********"`)
}

func TestAddStrippedKeys(t *testing.T) {
	contents := `foobar: baz`
	cleaned, err := ScrubBytes([]byte(contents))
	require.Nil(t, err)

	// Sanity check
	assert.Equal(t, contents, string(cleaned))

	AddStrippedKeys([]string{"foobar"})

	assertClean(t, contents, `foobar: "********"`)
}

func TestAddStrippedKeysNewReplacer(t *testing.T) {
	contents := `foobar: baz`
	AddStrippedKeys([]string{"foobar"})

	newScrubber := New()
	AddDefaultReplacers(newScrubber)

	cleaned, err := newScrubber.ScrubBytes([]byte(contents))
	require.Nil(t, err)
	assert.Equal(t, strings.TrimSpace(`foobar: "********"`), strings.TrimSpace(string(cleaned)))
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

func TestConfig(t *testing.T) {
	assertClean(t,
		`dd_url: https://app.datadoghq.com
api_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
proxy: http://user:password@host:port
password: foo
auth_token: bar
auth_token_file_path: /foo/bar/baz
kubelet_auth_token_path: /foo/bar/kube_token
# comment to strip
network_devices:
  snmp_traps:
    community_strings:
    - 'password1'
    - 'password2'
log_level: info`,
		`dd_url: https://app.datadoghq.com
api_key: "***************************aaaaa"
proxy: http://user:********@host:port
password: "********"
auth_token: "********"
auth_token_file_path: /foo/bar/baz
kubelet_auth_token_path: /foo/bar/kube_token
network_devices:
  snmp_traps:
    community_strings: "********"
log_level: info`)
}

func TestConfigFile(t *testing.T) {
	cleanedConfigFile := `dd_url: https://app.datadoghq.com

api_key: "***************************aaaaa"

proxy: http://user:********@host:port






dogstatsd_port : 8125


log_level: info
`

	wd, _ := os.Getwd()
	filePath := filepath.Join(wd, "test", "datadog.yaml")
	cleaned, err := ScrubFile(filePath)
	assert.Nil(t, err)
	cleanedString := string(cleaned)

	assert.Equal(t, cleanedConfigFile, cleanedString)
}

func TestBearerToken(t *testing.T) {
	assertClean(t,
		`Bearer 2fe663014abcd1850076f6d68c0355666db98758262870811cace007cd4a62ba`,
		`Bearer ***********************************************************a62ba`)
	assertClean(t,
		`Error: Get "https://localhost:5001/agent/status": net/http: invalid header field value "Bearer 260a9c065b6426f81b7abae9e6bca9a16f7a842af65c940e89e3417c7aaec82d\n\n" for key Authorization`,
		`Error: Get "https://localhost:5001/agent/status": net/http: invalid header field value "Bearer ***********************************************************ec82d\n\n" for key Authorization`)
	assertClean(t,
		`AuthBearer 2fe663014abcd1850076f6d68c0355666db98758262870811cace007cd4a62ba`,
		`AuthBearer 2fe663014abcd1850076f6d68c0355666db98758262870811cace007cd4a62ba`)
}
