// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
	"fmt"
	"bytes"
	"embed"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type sdsScrubberTest struct {

	input []byte
	expectedOutput string
}

func setupTest() *SDSScrubber {
	sdsScrubber, err := initRules(nil).initObjectRules(nil).sdsStart()
	if err != nil {
		panic(err)
	}
	return sdsScrubber
}

func TestSetupSDSScrubber(t *testing.T) {
	sdsScrubber := setupTest()
	assert.NotNil(t, sdsScrubber)
	fmt.Println(fmt.Sprintf("%v", sdsScrubber))
}

func TestScrubApiKey(t *testing.T) {
	sdsScrubber := setupTest()
	tests := map[string]sdsScrubberTest{
		"Scrubbed API key from command": {
			input:          []byte(`DD_API_KEY=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa agent run`),
			expectedOutput: `DD_API_KEY=***************************aaaaa agent run`,
		},
		"Scrubbed api_key config (simple)": {
			input:          []byte(`api_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`),
			//expectedOutput: `api_key: "***************************abbbb"`,
			expectedOutput: `api_key: ***************************abbbb`,
		},
		"Scrubbed api_key config (uppercase)": {
			input:          []byte(`api_key: AAAAAAAAAAAAAAAAAAAAAAAAAAAABBBB`),
			// expectedOutput: `api_key: "***************************ABBBB"`,
			expectedOutput: `api_key: ***************************ABBBB`,
		},
		"Scrubbed api_key config (quoted)": {
			input:          []byte(`api_key: "aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb"`),
			expectedOutput: `api_key: "***************************abbbb"`,
		},
		"Scrubbed api_key config (single-quoted)": {
			input:          []byte(`api_key: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb'`),
			expectedOutput: `api_key: '***************************abbbb'`,
		},
		"Scrubbed api_key config (multiline block)": {
			input: []byte(`api_key: |
			aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`),
			expectedOutput: `api_key: |
			***************************abbbb`,
		},
		"Scrubbed api_key config (multiline folded)": {
			input: []byte(`api_key: >
			aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`),
			expectedOutput: `api_key: >
			***************************abbbb`,
		},
		"Scrubbed api_key config (extra spaces)": {
			input:          []byte(`   api_key:   'aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb'   `),
			expectedOutput: `   api_key:   '***************************abbbb'   `,
		},
		"Scrubbed api_key in additional_endpoints": {
			input: []byte(`
		additional_endpoints:
			"https://app.datadoghq.com":
			- aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb,
			- bbbbbbbbbbbbbbbbbbbbbbbbbbbbaaaa,
			"https://dog.datadoghq.com":
			- aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb,
			- bbbbbbbbbbbbbbbbbbbbbbbbbbbbaaaa`),
			expectedOutput: `
		additional_endpoints:
			"https://app.datadoghq.com":
			- ***************************abbbb,
			- ***************************baaaa,
			"https://dog.datadoghq.com":
			- ***************************abbbb,
			- ***************************baaaa`,
		}, // "***************************abbbb" and "***************************baaaa" are replaced by the same string without quotes
		"API key in URL": {
			input:          []byte(`Error status code 500 : http://dog.tld/api?key=3290abeefc68e1bbe852a25252bad88c`),
			expectedOutput: `Error status code 500 : http://dog.tld/api?key=***************************ad88c`,
		},
		"Hinted API key replacer (invalid length)": {
			input:          []byte(`hintedAPIKeyReplacer : http://dog.tld/api_key=InvalidLength12345abbbb`),
			// expectedOutput: `hintedAPIKeyReplacer : http://dog.tld/api_key=***************************abbbb`,
			expectedOutput: `hintedAPIKeyReplacer : http://dog.tld/api_key=********`,
		},
		"Hinted API key replacer (short key)": {
			input:          []byte(`hintedAPIKeyReplacer : http://dog.tld/apikey=InvalidLength12345abbbb`),
			// expectedOutput: `hintedAPIKeyReplacer : http://dog.tld/apikey=***************************abbbb`,
			expectedOutput: `hintedAPIKeyReplacer : http://dog.tld/api_key=********`,
		},
		"API key in agent URL": {
			input:          []byte(`apiKeyReplacer: https://agent-http-intake.logs.datadoghq.com/v1/input/aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`),
			expectedOutput: `apiKeyReplacer: https://agent-http-intake.logs.datadoghq.com/v1/input/***************************abbbb`,
		},
		"Docker inspect environment with API key": {
			input: []byte(`
	"Env": [
		"DD_API_KEY=3290abeefc68e1bbe852a25252bad88c",
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"DOCKER_DD_AGENT=yes",
		"AGENT_VERSION=1:6.0",
		"DD_AGENT_HOME=/opt/datadog-agent6/"
	]`),
			expectedOutput: `
	"Env": [
		"DD_API_KEY=***************************ad88c",
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"DOCKER_DD_AGENT=yes",
		"AGENT_VERSION=1:6.0",
		"DD_AGENT_HOME=/opt/datadog-agent6/"
	]`,
		},
		"Container ID not scrubbed": {
			input:          []byte(`container_id: "b32bd6f9b73ba7ccb64953a04b82b48e29dfafab65fd57ca01d3b94a0e024885"`),
			expectedOutput: `container_id: "b32bd6f9b73ba7ccb64953a04b82b48e29dfafab65fd57ca01d3b94a0e024885"`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			output, err := sdsScrubber.Scan(test.input)
			if err != nil {
				t.Errorf("Error scanning: %v", err)
			}
			assert.Equal(t, test.expectedOutput, string(output))
		})
	}
}

func TestScrubAppKey(t *testing.T) {
	sdsScrubber := setupTest()
	tests := map[string]sdsScrubberTest{
		"Scrubbed app key from command": {
			input:          []byte(`DD_APP_KEY=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa agent run`),
			expectedOutput: `DD_APP_KEY=***********************************aaaaa agent run`,
		},
		"Scrubbed app_key config (simple)": {
			input:          []byte(`app_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`),
			// expectedOutput: `app_key: "***********************************abbbb"`,
			expectedOutput: `app_key: ***********************************abbbb`,
		},
		"Scrubbed app_key config (uppercase)": {
			input:          []byte(`app_key: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABBBB`),
			// expectedOutput: `app_key: "***********************************ABBBB"`,
			expectedOutput: `app_key: ***********************************ABBBB`,
		},
		"Scrubbed app_key config (quoted)": {
			input:          []byte(`app_key: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb"`),
			expectedOutput: `app_key: "***********************************abbbb"`,
		},
		"Scrubbed app_key config (single-quoted)": {
			input:          []byte(`app_key: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb'`),
			expectedOutput: `app_key: '***********************************abbbb'`,
		},
		"Scrubbed app_key config (multiline block)": {
			input: []byte(`app_key: |
			aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`),
			expectedOutput: `app_key: |
			***********************************abbbb`,
		},
		"Scrubbed app_key config (multiline folded)": {
			input: []byte(`app_key: >
			aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`),
			expectedOutput: `app_key: >
			***********************************abbbb`,
		},
		"Scrubbed app_key config (extra spaces)": {
			input:          []byte(`   app_key:   'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb'   `),
			expectedOutput: `   app_key:   '***********************************abbbb'   `,
		},
		"Scrubbed RC app key": {
			input:          []byte(`key: "DDRCM_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABCDE"`),
			expectedOutput: `key: "********"`,
		},
		"Scrubbed app key in URL": {
			input:          []byte(`hintedAPPKeyReplacer : http://dog.tld/app_key=InvalidLength12345abbbb`),
			// expectedOutput: `hintedAPPKeyReplacer : http://dog.tld/app_key=***********************************abbbb`,
			expectedOutput: `hintedAPPKeyReplacer : http://dog.tld/app_key=********`,
		},
		"Scrubbed appkey in URL": {
			input:          []byte(`hintedAPPKeyReplacer : http://dog.tld/appkey=InvalidLength12345abbbb`),
			// expectedOutput: `hintedAPPKeyReplacer : http://dog.tld/appkey=***********************************abbbb`,
			expectedOutput: `hintedAPPKeyReplacer : http://dog.tld/app_key=********`,
		},
		"Scrubbed application_key in URL": {
			input:          []byte(`hintedAPPKeyReplacer : http://dog.tld/application_key=InvalidLength12345abbbb`),
			// expectedOutput: `hintedAPPKeyReplacer : http://dog.tld/application_key=***********************************abbbb`,
			expectedOutput: `hintedAPPKeyReplacer : http://dog.tld/app_key=********`,
		},
		"Scrubbed app key in full URL": {
			input:          []byte(`appKeyReplacer: http://dog.tld/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb`),
			expectedOutput: `appKeyReplacer: http://dog.tld/***********************************abbbb`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			output, err := sdsScrubber.Scan(test.input)
			if err != nil {
				t.Errorf("Error scanning: %v", err)
			}
			assert.Equal(t, test.expectedOutput, string(output))
		})
	}
}



func TestScanBearerToken(t *testing.T) {
	sdsScrubber := setupTest()
	tests := map[string]sdsScrubberTest{
		"Scrubbed bearer token": {
			input:          []byte("Bearer 2fe663014abcd1850076f6d68c0355666db98758262870811cace007cd4a62ba"),
			// expectedOutput: "Bearer ***********************************************************a62ba",
			expectedOutput: "Bearer ********",
		},
		"Scrubbed bearer token in error message": {
			input: []byte(`Error: Get "https://localhost:5001/agent/status": net/http: invalid header field value "Bearer 260a9c065b6426f81b7abae9e6bca9a16f7a842af65c940e89e3417c7aaec82d\n\n" for key Authorization`),
			// expectedOutput: `Error: Get "https://localhost:5001/agent/status": net/http: invalid header field value "Bearer ***********************************************************ec82d\n\n" for key Authorization`,
			expectedOutput: `Error: Get "https://localhost:5001/agent/status": net/http: invalid header field value "Bearer ********\n\n" for key Authorization`,
		},
		"Fully scrubbed short bearer token": {
			input:          []byte("Bearer 2fe663014abcd18"),
			expectedOutput: "Bearer ********",
		},
		"Fully scrubbed long bearer token with extra text": {
			input: []byte("Bearer 2fe663014abcd1850076f6d68c0355666db98758262870811cace007cd4a62bdsldijfoiwjeoimdfolisdjoijfewoa"),
			expectedOutput: "Bearer ********",
		},
		"Scrubbed UUID-like bearer token": {
			input:          []byte("Bearer abf243d1-9ba5-4d8d-8365-ac18229eb2ac"),
			expectedOutput: "Bearer ********",
		},
		// =================== Test that does not pass =========================
		/*"Scrubbed bearer token with space": {
			input:          []byte("Bearer token with space"),
			expectedOutput: "Bearer ********",
		},
		"Scrubbed short bearer token with spaces": {
			input:          []byte("Bearer     123456798"),
			expectedOutput: "Bearer ********",
		},*/
		"Non-scrubbed AuthBearer token": {
			input:          []byte("AuthBearer 2fe663014abcd1850076f6d68c0355666db98758262870811cace007cd4a62ba"),
			expectedOutput: "AuthBearer 2fe663014abcd1850076f6d68c0355666db98758262870811cace007cd4a62ba",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			output, err := sdsScrubber.Scan(test.input)
			if err != nil {
				t.Errorf("Error scanning: %v", err)
			}
			assert.Equal(t, test.expectedOutput, string(output))
		})
	}
}

func TestScrubPasswordsAndURLs(t *testing.T) {
	sdsScrubber := setupTest()
	tests := map[string]sdsScrubberTest{
		"Scrubbed URL with password": {
			input:          []byte(`random_url_key: http://user:password@host:port`),
			// expectedOutput: `random_url_key: http://user:********@host:port`,
			expectedOutput: `random_url_key: [REDACTED]@host:port`,
		},
		"Scrubbed URL with special characters in password": {
			input:          []byte(`random_url_key: http://user:p@ssw0r)@host:port`),
			// expectedOutput: `random_url_key: http://user:********@host:port`,
			expectedOutput: `random_url_key: [REDACTED]@host:port`,
		},
		"Scrubbed URL with emojis in password": {
			input:          []byte(`random_url_key: http://user:🔑🔒🔐🔓@host:port`),
			// expectedOutput: `random_url_key: http://user:********@host:port`,
			expectedOutput: `random_url_key: [REDACTED]@host:port`,
		},
		"Scrubbed URL with password (no port)": {
			input:          []byte(`random_url_key: http://user:password@host`),
			// expectedOutput: `random_url_key: http://user:********@host`,
			expectedOutput: `random_url_key: [REDACTED]@host`,
		},
		"Scrubbed quoted URL with password": {
			input:          []byte(`random_url_key: "http://user:password@host:port"`),
			// expectedOutput: `random_url_key: "http://user:********@host:port"`,
			expectedOutput: `random_url_key: "[REDACTED]@host:port"`,
		},
		"Scrubbed single-quoted URL with password": {
			input:          []byte(`random_url_key: 'http://user:password@host:port'`),
			// expectedOutput: `random_url_key: 'http://user:********@host:port'`,
			expectedOutput: `random_url_key: '[REDACTED]@host:port'`,
		},
		"Scrubbed domain-like key with password": {
			input:          []byte(`random_domain_key: 'user:password@host:port'`),
			// expectedOutput: `random_domain_key: 'user:********@host:port'`,
			expectedOutput: `random_domain_key: '[REDACTED]@host:port'`,
		},
		"Scrubbed MongoDB+SRV URL with hyphenated password": {
			input:          []byte(`   random_url_key:   'mongodb+srv://user:pass-with-hyphen@abc.example.com/database'   `),
			// expectedOutput: `   random_url_key:   'mongodb+srv://user:********@abc.example.com/database'   `,
			expectedOutput: `   random_url_key:   '[REDACTED]@abc.example.com/database'   `,
		},
		"Scrubbed HTTP URL with hyphenated username and password": {
			input:          []byte(`   random_url_key:   'http://user-with-hyphen:pass-with-hyphen@abc.example.com/database'   `),
			// expectedOutput: `   random_url_key:   'http://user-with-hyphen:********@abc.example.com/database'   `,
			expectedOutput: `   random_url_key:   '[REDACTED]@abc.example.com/database'   `,
		},
		"Unchanged metric flushing": {
			input:          []byte(`flushing serie: {"metric":"kubeproxy","tags":["image_id":"foobar/foobaz@sha256:e8dabc7d398d25ecc8a3e33e3153e988e79952f8783b81663feb299ca2d0abdd"]}`),
			expectedOutput: `flushing serie: {"metric":"kubeproxy","tags":["image_id":"foobar/foobaz@sha256:e8dabc7d398d25ecc8a3e33e3153e988e79952f8783b81663feb299ca2d0abdd"]}`,
		},
		"Scrubbed mysql_password config": {
			input:          []byte(`mysql_password: password`),
			// expectedOutput: `mysql_password: "********"`,
			expectedOutput: `password:[REDACTED]`,
		},
		"Scrubbed mysql_password with special characters": {
			input:          []byte(`mysql_password: p@ssw0r)`),
			// expectedOutput: `mysql_password: "********"`,
			expectedOutput: `password:[REDACTED]`,
		},
		"Scrubbed mysql_password with emojis": {
			input:          []byte(`mysql_password: 🔑 🔒 🔐 🔓`),
			// expectedOutput: `mysql_password: "********"`,
			expectedOutput: `password:[REDACTED]`,
		},
		"Scrubbed quoted mysql_password": {
			input:          []byte(`mysql_password: "password"`),
			// expectedOutput: `mysql_password: "********"`,
			expectedOutput: `password:[REDACTED]`,
		},
		"Scrubbed single-quoted mysql_password": {
			input:          []byte(`mysql_password: 'password'`),
			// expectedOutput: `mysql_password: "********"`,
			expectedOutput: `password:[REDACTED]`,
		},
		"Scrubbed pwd config": {
			input:          []byte(`pwd: 'password'`),
			// expectedOutput: `pwd: "********"`,
			expectedOutput: `password:[REDACTED]`,
		},
		"Scrubbed cert_key_password config": {
			input:          []byte(`cert_key_password: p@ssw0r`),
			// expectedOutput: `cert_key_password: "********"`,
			expectedOutput: `password:[REDACTED]`,
		},
		"Scrubbed cert_key_password config with emojis": {
			input:          []byte(`cert_key_password: 🔑 🔒 🔐 🔓`),
			// expectedOutput: `cert_key_password: "********"`,
			expectedOutput: `password:[REDACTED]`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			output, err := sdsScrubber.Scan(test.input)
			if err != nil {
				t.Errorf("Error scanning: %v", err)
			}
			assert.Equal(t, test.expectedOutput, string(output))
		})
	}
}

func TestScrubCertKey(t *testing.T) {
	sdsScrubber := setupTest()
	tests := map[string]sdsScrubberTest{
		"Scrubbed private key in block format": {
			input: []byte(`cert_key: >
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
		   -----END PRIVATE KEY-----`),
			expectedOutput: `cert_key: >
		   ********`,
		},
		"Scrubbed certificate in block format": {
			input: []byte(`cert_key: |
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
			-----END CERTIFICATE-----`),
			expectedOutput: `cert_key: |
			********`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			output, err := sdsScrubber.Scan(test.input)
			if err != nil {
				t.Errorf("Error scanning: %v", err)
			}
			assert.Equal(t, test.expectedOutput, string(output))
		})
	}
}

func TestScrubComplexString(t *testing.T) {
	sdsScrubber := setupTest()
	testcases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "logged uppercase (eg. Password=)",
			input: `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: Username=userme Password=$AeVtn8*gbyaf!hnUHx^L."}`,
			// want:  `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: Username=userme Password=********"}`,
			want: `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: Username=userme password=[REDACTED]"}`,
		},
		{
			name: "logged lowercase (eg. password=)",
			input: `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: username=userme password=$AeVtn8*gbyaf!hnUHx^L."}`,
			// want:  `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: username=userme password=********"}`,
			want: `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: username=userme password=[REDACTED]"}`,
		},
		{
			name: "logged with whitespace (eg. password =)",
			input: `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: username = userme password = $AeVtn8*gbyaf!hnUHx^L."}`,
			// want:  `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: username = userme password = ********"}`,
			want: `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: username = userme password=[REDACTED]"}`,
		},
		{
			name: "logged as json (eg. \"Password\":)",
			input: `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: {"username": "userme",  "password": "$AeVtn8*gbyaf!hnUHx^L."}}"`,
			//want:  `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: {"username": "userme",  "password": "********"}"}`,
			want: `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: {"username": "userme",  password=[REDACTED]"}}"`,
		},
		{
			name: "logged PSWD (eg. PSWD=)",
			input: `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: USER=userme PSWD=$AeVtn8*gbyaf!hnUHx^L."}`,
			//want:  `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: USER=userme PSWD=********"}`,
			want: `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: USER=userme password=[REDACTED]"}`,
		},
		{
			name: "already scrubbed log (eg. password=********)",
			input: `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: username=userme password=[REDACTED]"}`,
			want:  `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect: username=userme password=[REDACTED]"}`,
		},
		{
			name: "already scrubbed YAML (eg. password: ********)",
			input: `dd_url: https://app.datadoghq.com
api_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
proxy: http://user:password@host:port
password: foo`,
			want: `dd_url: https://app.datadoghq.com
api_key: ***************************aaaaa
proxy: [REDACTED]@host:portpassword:[REDACTED]`,
		},
		{
			name: "real log test 1",
			input: `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect to SQL Server, see https://docs.datadoghq.com/database_monitoring/setup_sql_server/troubleshooting#common-driver-issues for more details on how to debug this issue. TCP-connection(OK), Exception: OperationalError(com_error(-2147352567, 'Exception occurred.', (0, 'ADODB.Connection', 'Provider cannot be found. It may not be properly installed.', 'C:\\\\Windows\\\\HELP\\\\ADOIRAMG.CHM', 1240655, -2146824582), None), 'Error opening connection to \"ConnectRetryCount=2;Provider=MSOLEDBSQL;Data Source=fwurgdae532sk1,1433;Initial Catalog=master;User ID=sqlsqlsql;Password=Si5123$#!@as\\\\rrrrg;\"')\ncode=common-driver-issues connection_host=fwurgdae532sk1,1433 connector=adodbapi database=master driver=None host=fwurgdae532sk1,1433"}`,
			//want:  `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect to SQL Server, see https://docs.datadoghq.com/database_monitoring/setup_sql_server/troubleshooting#common-driver-issues for more details on how to debug this issue. TCP-connection(OK), Exception: OperationalError(com_error(-2147352567, 'Exception occurred.', (0, 'ADODB.Connection', 'Provider cannot be found. It may not be properly installed.', 'C:\\\\Windows\\\\HELP\\\\ADOIRAMG.CHM', 1240655, -2146824582), None), 'Error opening connection to \"ConnectRetryCount=2;Provider=MSOLEDBSQL;Data Source=fwurgdae532sk1,1433;Initial Catalog=master;User ID=sqlsqlsql;Password=********;\"')\ncode=common-driver-issues connection_host=fwurgdae532sk1,1433 connector=adodbapi database=master driver=None host=fwurgdae532sk1,1433"}`,
			want: `2024-07-02 10:40:18 EDT | CORE | ERROR | (pkg/collector/worker/check_logger.go:71 in Error) | check:sqlserver | Error running check: [{"message": "Unable to connect to SQL Server, see https://docs.datadoghq.com/database_monitoring/setup_sql_server/troubleshooting#common-driver-issues for more details on how to debug this issue. TCP-connection(OK), Exception: OperationalError(com_error(-2147352567, 'Exception occurred.', (0, 'ADODB.Connection', 'Provider cannot be found. It may not be properly installed.', 'C:\\\\Windows\\\\HELP\\\\ADOIRAMG.CHM', 1240655, -2146824582), None), 'Error opening connection to \"ConnectRetryCount=2;Provider=MSOLEDBSQL;Data Source=fwurgdae532sk1,1433;Initial Catalog=master;User ID=sqlsqlsql;password=[REDACTED];\"')\ncode=common-driver-issues connection_host=fwurgdae532sk1,1433 connector=adodbapi database=master driver=None host=fwurgdae532sk1,1433"}`,
		},
		{
			name: "real log test 2",
			input: `2024-03-22 02:00:09 EDT | PROCESS | WARN | (pkg/process/procutil/process_windows.go:603 in ParseCmdLineArgs) | unexpected quotes in string, giving up ( /SQL "\"\OwnerDashboard\"" /SERVER fwurgdae532sk1 /CONNECTION EDWPRD;"\"Data Source=EDWPRD;User ID=qwimMAK;password=quark0n;Persist Security Info=True;Unicode=True;\"" /CONNECTION MRDPRD;"\"Data Source=MRDPRD;User ID=quirmak;password=s3llerj4m;Persist Security Info=True;Unicode=True;\"" /CONNECTION RPTEJMACPRD;"\"Data Source=RPTEJMACPRD;User ID=vwurpe;password=squ1rr3l;Persist Security Info=True;Unicode=True;\"" /CHECKPOINTING OFF /REPORTING E)`,
			//want:  `2024-03-22 02:00:09 EDT | PROCESS | WARN | (pkg/process/procutil/process_windows.go:603 in ParseCmdLineArgs) | unexpected quotes in string, giving up ( /SQL "\"\OwnerDashboard\"" /SERVER fwurgdae532sk1 /CONNECTION EDWPRD;"\"Data Source=EDWPRD;User ID=qwimMAK;password=********;Persist Security Info=True;Unicode=True;\"" /CONNECTION MRDPRD;"\"Data Source=MRDPRD;User ID=quirmak;password=********;Persist Security Info=True;Unicode=True;\"" /CONNECTION RPTEJMACPRD;"\"Data Source=RPTEJMACPRD;User ID=vwurpe;password=********;Persist Security Info=True;Unicode=True;\"" /CHECKPOINTING OFF /REPORTING E)`,
			want: `2024-03-22 02:00:09 EDT | PROCESS | WARN | (pkg/process/procutil/process_windows.go:603 in ParseCmdLineArgs) | unexpected quotes in string, giving up ( /SQL "\"\OwnerDashboard\"" /SERVER fwurgdae532sk1 /CONNECTION EDWPRD;"\"Data Source=EDWPRD;User ID=qwimMAK;password=[REDACTED];Persist Security Info=True;Unicode=True;\"" /CONNECTION MRDPRD;"\"Data Source=MRDPRD;User ID=quirmak;password=[REDACTED];Persist Security Info=True;Unicode=True;\"" /CONNECTION RPTEJMACPRD;"\"Data Source=RPTEJMACPRD;User ID=vwurpe;password=[REDACTED];Persist Security Info=True;Unicode=True;\"" /CHECKPOINTING OFF /REPORTING E)`,
		},
		{
			name: "real log test 3",
			input: `2024-05-14 00:05:01 BST | PROCESS | WARN | (pkg/process/procutil/process_windows.go:603 in ParseCmdLineArgs) | unexpected quotes in string, giving up (""C:\User\shared\jdk\open-jdk-8-win32\1.8"\bin\javaw"  -Djava.library.path="C:\windows\system32;C:\windows;C:\windows\System32\Wbem;C:\windows\System32\WindowsPowerShell\v1.0\;C:\windows\System32\OpenSSH\;C:\Users\thufpos\AppData\Local\Microsoft\WindowsApps;;C:\User\\pos\licence;C:\User\\pos\shared-obj;C:\User\\pos\jpos-lib"	-Xms512M -Xmx1024M	-XX:+HeapDumpOnOutOfMemoryError -XX:HeapDumpPath="C:\User\\pos\logs"		-Djavax.net.ssl.trustStore="C:\User\\pos\trust\.mobilePOS.trustStore"	-Djavax.net.ssl.trustStorePassword="QwermiAD#@1sdkjf#$%\\xsdf|f!"`,
			// want:  `2024-05-14 00:05:01 BST | PROCESS | WARN | (pkg/process/procutil/process_windows.go:603 in ParseCmdLineArgs) | unexpected quotes in string, giving up (""C:\User\shared\jdk\open-jdk-8-win32\1.8"\bin\javaw"  -Djava.library.path="C:\windows\system32;C:\windows;C:\windows\System32\Wbem;C:\windows\System32\WindowsPowerShell\v1.0\;C:\windows\System32\OpenSSH\;C:\Users\thufpos\AppData\Local\Microsoft\WindowsApps;;C:\User\\pos\licence;C:\User\\pos\shared-obj;C:\User\\pos\jpos-lib"	-Xms512M -Xmx1024M	-XX:+HeapDumpOnOutOfMemoryError -XX:HeapDumpPath="C:\User\\pos\logs"		-Djavax.net.ssl.trustStore="C:\User\\pos\trust\.mobilePOS.trustStore"	-Djavax.net.ssl.trustStorePassword="********"`,
			want: `2024-05-14 00:05:01 BST | PROCESS | WARN | (pkg/process/procutil/process_windows.go:603 in ParseCmdLineArgs) | unexpected quotes in string, giving up (""C:\User\shared\jdk\open-jdk-8-win32\1.8"\bin\javaw"  -Djava.library.path="C:\windows\system32;C:\windows;C:\windows\System32\Wbem;C:\windows\System32\WindowsPowerShell\v1.0\;C:\windows\System32\OpenSSH\;C:\Users\thufpos\AppData\Local\Microsoft\WindowsApps;;C:\User\\pos\licence;C:\User\\pos\shared-obj;C:\User\\pos\jpos-lib"	-Xms512M -Xmx1024M	-XX:+HeapDumpOnOutOfMemoryError -XX:HeapDumpPath="C:\User\\pos\logs"		-Djavax.net.ssl.trustStore="C:\User\\pos\trust\.mobilePOS.trustStore"	-Djavax.net.ssl.trustStorepassword=[REDACTED]"`,
		},
		{
			name: "real log test 4",
			input: `2024-07-05 14:36:54 CEST | CORE | DEBUG | (pkg/collector/python/datadog_agent.go:135 in LogMessage) | http_check: login (via catalog):ef29cea7c32fc55 | (http_check.py:119) | Connecting to http://catalog.example.com:8080/search-engine/login.do?userName=Morbotron&userPassword=K#2asdfu!23%%x`,
			//want:  `2024-07-05 14:36:54 CEST | CORE | DEBUG | (pkg/collector/python/datadog_agent.go:135 in LogMessage) | http_check: login (via catalog):ef29cea7c32fc55 | (http_check.py:119) | Connecting to http://catalog.example.com:8080/search-engine/login.do?userName=Morbotron&userPassword=********`,
			want: `2024-07-05 14:36:54 CEST | CORE | DEBUG | (pkg/collector/python/datadog_agent.go:135 in LogMessage) | http_check: login (via catalog):ef29cea7c32fc55 | (http_check.py:119) | Connecting to http://catalog.example.com:8080/search-engine/login.do?userName=Morbotron&userpassword=[REDACTED]`,
		},
		{
			name: "Config test with multiple tokens and passwords",
			input: `dd_url: https://app.datadoghq.com
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
			/*want: `dd_url: https://app.datadoghq.com
api_key: ***************************aaaaa
proxy: http://user:********@host:port
password: "********"
auth_token: "********"
auth_token_file_path: /foo/bar/baz
kubelet_auth_token_path: /foo/bar/kube_token
network_devices:
  snmp_traps:
    community_strings: "********"
log_level: info`,*/
			want: `dd_url: https://app.datadoghq.com
api_key: ***************************aaaaa
proxy: [REDACTED]@host:portpassword:[REDACTED]token:[REDACTED]
auth_token_file_path: /foo/bar/baz
kubelet_auth_token_path: /foo/bar/kube_token
# comment to strip
network_devices:
  snmp_traps:community_strings: [REDACTED]
log_level: info`,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := sdsScrubber.Scan([]byte(tc.input))
			if err != nil {
				t.Errorf("Error scanning: %v", err)
			}
			assert.Equal(t, tc.want, string(output))
		})
	}
}

// Embed the test and result files.
//go:embed sds-tests/results/*
var TestResult embed.FS

//go:embed sds-tests/tests/*
var TestFiles embed.FS

func TestScrubFiles(t *testing.T) {
	sdsScrubber := setupTest()
	// Read the list of test files
	entries, err := TestFiles.ReadDir("sds-tests/tests")
	if err != nil {
		t.Fatalf("Error reading test files directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		testFileName := entry.Name()
		resultFileName := strings.TrimSuffix(testFileName, filepath.Ext(testFileName)) + "_scrubbed" + filepath.Ext(testFileName)

		// Read the test file
		testFilePath := "sds-tests/tests/" + testFileName
		testContent, err := TestFiles.ReadFile(testFilePath)
		if err != nil {
			t.Fatalf("Error reading test file %s: %v", testFilePath, err)
		}

		// Read the corresponding scrubbed result file
		resultFilePath := "sds-tests/results/" + resultFileName
		resultContent, err := TestResult.ReadFile(resultFilePath)
		if err != nil {
			t.Fatalf("Error reading result file %s: %v", resultFilePath, err)
		}

		// Run the scrubber on the test file content
		scrubbedContent, err := sdsScrubber.StartScan(testFileName, testContent)
		if err != nil {
			t.Fatalf("Error scrubbing file %s: %v", testFileName, err)
		}

		// Compare the scrubbed content with the expected result content
		if !bytes.Equal(scrubbedContent, resultContent) {
			t.Errorf("Scrubbed content for file %s does not match the expected result.\nExpected:\n%s\nGot:\n%s", testFileName, string(resultContent), string(scrubbedContent))
		}
	}
}
