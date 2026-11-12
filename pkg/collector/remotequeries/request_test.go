// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeries

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const resolveInput = `{"integration":"postgres","target":{"host":"db","port":5432,"dbname":"orders"},"resolveOnly":true}`
const executeInput = `{"integration":"postgres","target":{"database_instance":"orders"},"resolveOnly":false,"query":"select secret_value","includeSchema":false,"timeout":3000,"resultDelivery":{"runId":"7c4e2b1a-9f3d-4c8e-b6a5-1d2f3e4a5b6c","taskId":"d25e7f9f-6883-4462-b31e-b449f70c0894","uploadId":"upload-1","baseUrl":"https://dd.datadoghq.com/api/unstable/its-agent-intake","limits":{"maxFileBytes":16777216,"maxResultBytes":107374182400,"maxRowBytes":16777216,"maxColumns":1024,"maxSchemaBytes":1048576,"maxPages":1280,"timeoutMs":3000000}}}`

func changeInput(t *testing.T, original string, change func(map[string]interface{})) []byte {
	t.Helper()
	var input map[string]interface{}
	dec := json.NewDecoder(strings.NewReader(original))
	dec.UseNumber()
	require.NoError(t, dec.Decode(&input))
	change(input)
	raw, err := json.Marshal(input)
	require.NoError(t, err)
	return raw
}

// ENABLE-03: no request accepts database, Agent API, or upload credentials.
func TestENABLE03(t *testing.T) {
	for _, field := range []string{"credentials", "apiKey", "password", "token", "connectionString"} {
		t.Run(field, func(t *testing.T) {
			for _, where := range []string{"input", "target", "resultDelivery", "limits"} {
				raw := changeInput(t, executeInput, func(input map[string]interface{}) {
					container := input
					switch where {
					case "target":
						container = input["target"].(map[string]interface{})
					case "resultDelivery":
						container = input["resultDelivery"].(map[string]interface{})
					case "limits":
						container = input["resultDelivery"].(map[string]interface{})["limits"].(map[string]interface{})
					}
					container[field] = "do-not-forward"
				})
				_, err := Parse(raw, "datadoghq.com")
				require.Error(t, err)
			}
		})
	}
}

// INPUT-01
func TestINPUT01(t *testing.T) {
	_, err := Parse([]byte(resolveInput), "datadoghq.com")
	require.NoError(t, err)
	for _, field := range []string{"query", "includeSchema", "resultDelivery", "timeout"} {
		for _, value := range []interface{}{nil, "", false, map[string]interface{}{}} {
			raw := changeInput(t, resolveInput, func(input map[string]interface{}) { input[field] = value })
			_, err := Parse(raw, "datadoghq.com")
			require.ErrorContains(t, err, field)
		}
	}
	for _, value := range []interface{}{nil, "true", 1} {
		raw := changeInput(t, resolveInput, func(input map[string]interface{}) { input["resolveOnly"] = value })
		_, err := Parse(raw, "datadoghq.com")
		require.ErrorContains(t, err, "resolveOnly")
	}
}

// INPUT-02
func TestINPUT02(t *testing.T) {
	for _, tc := range []struct {
		value string
		valid bool
	}{{" PoStGrEs_2 ", true}, {"", false}, {" ", false}, {"post-gres", false}, {"post.gres", false}, {"pöstgres", false}, {"postgres\npassword", false}} {
		raw := changeInput(t, resolveInput, func(input map[string]interface{}) { input["integration"] = tc.value })
		req, err := Parse(raw, "datadoghq.com")
		if tc.valid {
			require.NoError(t, err)
			require.Equal(t, "postgres_2", req.Integration)
		} else {
			require.ErrorContains(t, err, "integration")
		}
	}
}

// INPUT-03
func TestINPUT03(t *testing.T) {
	for _, tc := range []struct {
		target string
		valid  bool
	}{
		{`{"host":" DB.EXAMPLE. ","port":65535,"dbname":"orders"}`, true},
		{`{"host":"db..","port":5432,"dbname":"orders"}`, true},
		{`{"database_instance":"orders"}`, true},
		{`{"database_instance":" orders"}`, false}, {`{"database_instance":""}`, false},
		{`{"database_instance":"orders","host":""}`, false}, {`{"database_instance":"orders","port":null}`, false},
		{`{"host":"db","port":0,"dbname":"orders"}`, false}, {`{"host":"db","port":65536,"dbname":"orders"}`, false},
		{`{"host":"db","port":1.5,"dbname":"orders"}`, false}, {`{"host":"db","port":"5432","dbname":"orders"}`, false},
		{`{"host":"db","port":null,"dbname":"orders"}`, false}, {`{"host":"db","port":5432}`, false},
		{`{"host":" ","port":5432,"dbname":"orders"}`, false}, {`{"host":"db","port":5432,"dbname":" "}`, false},
		{`{"host":"db","port":5432,"dbname":"orders","extra":0}`, false}, {`null`, false}, {`[]`, false},
	} {
		raw := changeInput(t, resolveInput, func(input map[string]interface{}) { input["target"] = json.RawMessage(tc.target) })
		req, err := Parse(raw, "datadoghq.com")
		if tc.valid {
			require.NoError(t, err)
			if req.Target.Host != "" {
				var original Target
				require.NoError(t, json.Unmarshal([]byte(tc.target), &original))
				require.Equal(t, original.Host, req.Target.Host, "checks must canonicalize the original host once")
			}
		} else {
			require.ErrorContains(t, err, "target")
		}
	}
}

// INPUT-04
func TestINPUT04(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(map[string]interface{})
		valid  bool
	}{
		{"valid", func(map[string]interface{}) {}, true},
		{"no runner timeout", func(i map[string]interface{}) { delete(i, "timeout") }, true},
		{"empty query", func(i map[string]interface{}) { i["query"] = "" }, false},
		{"missing schema", func(i map[string]interface{}) { delete(i, "includeSchema") }, false},
		{"missing delivery", func(i map[string]interface{}) { delete(i, "resultDelivery") }, false},
		{"run UUID", func(i map[string]interface{}) { i["resultDelivery"].(map[string]interface{})["runId"] = "bad" }, false},
		{"task UUID", func(i map[string]interface{}) { i["resultDelivery"].(map[string]interface{})["taskId"] = "bad" }, false},
		{"upload ID", func(i map[string]interface{}) { i["resultDelivery"].(map[string]interface{})["uploadId"] = "bad/id" }, false},
		{"unknown delivery", func(i map[string]interface{}) { i["resultDelivery"].(map[string]interface{})["artifactVersion"] = "v1" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(changeInput(t, executeInput, tc.change), "datadoghq.com")
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
	for _, field := range []string{"maxFileBytes", "maxResultBytes", "maxRowBytes", "maxColumns", "maxSchemaBytes", "maxPages", "timeoutMs", "unknown"} {
		for _, value := range []interface{}{nil, -1, 0, 0.5, "1", true, json.Number("999999999999999999999999999999999999")} {
			raw := changeInput(t, executeInput, func(input map[string]interface{}) {
				input["resultDelivery"].(map[string]interface{})["limits"].(map[string]interface{})[field] = value
			})
			_, err := Parse(raw, "datadoghq.com")
			if field != "unknown" && value == json.Number("999999999999999999999999999999999999") {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "limits")
			}
		}
		raw := changeInput(t, executeInput, func(input map[string]interface{}) {
			delete(input["resultDelivery"].(map[string]interface{})["limits"].(map[string]interface{}), field)
		})
		_, err := Parse(raw, "datadoghq.com")
		if field != "unknown" {
			require.Error(t, err)
		}
	}
}

// INPUT-05
func TestINPUT05(t *testing.T) {
	for _, tc := range []struct {
		address, site string
		valid         bool
	}{
		{"https://DD.DATADOGHQ.COM/path", "datadoghq.com", true}, {"https://datadoghq.com:443/path", "DATADOGHQ.COM", true},
		{"HTTPS://datadoghq.com:0443/path", "datadoghq.com", true},
		{"https://datadoghq.com:/path", "datadoghq.com", false},
		{"http://datadoghq.com/path", "datadoghq.com", false}, {"https://datadoghq.com.evil/path", "datadoghq.com", false},
		{"https://evildatadoghq.com/path", "datadoghq.com", false}, {"https://evil.com/datadoghq.com", "datadoghq.com", false},
		{"https://u:p@datadoghq.com/path", "datadoghq.com", false}, {"https://datadoghq.com:444/path", "datadoghq.com", false},
		{"https://datadoghq.com/path?x=1", "datadoghq.com", false}, {"https://datadoghq.com/path?", "datadoghq.com", false},
		{"https://datadoghq.com/path#x", "datadoghq.com", false}, {"https://datadoghq.com/path#", "datadoghq.com", false},
		{"https://datadoghq.com/pa th", "datadoghq.com", false}, {"https://datadoghq.com/pa\nth", "datadoghq.com", false},
		{"https://datadoghq.com/pa\\th", "datadoghq.com", false}, {"https://datadoghq.com/path", "", false},
		{"/path", "datadoghq.com", false}, {"https://datadoghq.com/path", "datadoghq.eu", false},
	} {
		raw := changeInput(t, executeInput, func(input map[string]interface{}) {
			input["resultDelivery"].(map[string]interface{})["baseUrl"] = tc.address
		})
		_, err := Parse(raw, tc.site)
		if tc.valid {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "baseUrl")
		}
	}
}

// CHECK-04: pure answer combination, independent of transport and check behavior.
func TestCHECK04(t *testing.T) {
	for _, tc := range []struct {
		matches int
		code    string
	}{{0, "target_not_found"}, {1, "matched"}, {2, "ambiguous_target"}, {50, "ambiguous_target"}} {
		require.Equal(t, tc.code, resolutionCode(tc.matches))
	}
}

// OUT-06
func TestOUT06(t *testing.T) {
	for _, tc := range []struct {
		raw            string
		resolve, valid bool
	}{
		{`{"status":"matched"}`, true, true}, {`{"status":"target_not_found","error":{"code":"target_not_found","message":"not found"}}`, true, true},
		{`{"status":"SUCCEEDED"}`, false, true}, {`{"status":"FAILED","error":{"code":"query_failed","message":"safe message"}}`, false, true},
		{`{"status":"SUCCEEDED","rows":[]}`, false, false}, {`{"status":"FAILED","error":{"code":"internal_error","message":"safe","details":{}}}`, false, false},
		{`{"status":"matched","error":null}`, true, false}, {`{"status":"target_not_found"}`, true, false},
		{`{"status":"FAILED","error":{"code":"target_not_found","message":"safe"}}`, true, false}, {`{"status":"matched","status":"matched"}`, true, false},
		{`{"status":"SUCCEEDED"} {"status":"SUCCEEDED"}`, false, false}, {`{"status":"FAILED","error":null}`, false, false},
	} {
		answer, err := ParseActionAnswer([]byte(tc.raw), tc.resolve)
		if tc.valid {
			require.NoError(t, err)
			encoded, err := json.Marshal(answer)
			require.NoError(t, err)
			require.JSONEq(t, tc.raw, string(encoded))
		} else {
			require.Error(t, err)
		}
	}
}
