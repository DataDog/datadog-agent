// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventplatformimpl

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	sdsscanner "github.com/DataDog/datadog-agent/comp/core/sdsscanner/def"
	sdspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/sds"
	sds "github.com/DataDog/datadog-agent/pkg/util/sds"
)

// fakeScanner is a test sds.Scanner that, for each configured (substring,
// ruleID) pair, emits one Match per occurrence of the substring in the scanned
// value. Emitting multiple matches for a single value exercises the per-row
// de-duplication in scanColumns.
type fakeScanner struct {
	rules map[string]string // substring -> ruleID
}

func (s fakeScanner) Scan(event []byte) ([]sds.Match, error) {
	var matches []sds.Match
	for substr, ruleID := range s.rules {
		for i := 0; i < strings.Count(string(event), substr); i++ {
			matches = append(matches, sds.Match{RuleID: ruleID})
		}
	}
	return matches, nil
}

func (fakeScanner) ScanMap(map[string]interface{}) ([]sds.Match, error) { return nil, nil }
func (fakeScanner) Close() error                                        { return nil }

// fakeScanners is a test sdsscanner.Component backed by a static map.
type fakeScanners struct {
	scanners map[string]sds.Scanner
}

func (f fakeScanners) Register(string, []sds.RuleDefinition) (sds.Scanner, error) {
	return nil, nil
}
func (f fakeScanners) Unregister(string) error { return nil }
func (f fakeScanners) Get(name string) (sds.Scanner, bool) {
	s, ok := f.scanners[name]
	return s, ok
}

var (
	_ sds.Scanner          = fakeScanner{}
	_ sdsscanner.Component = fakeScanners{}
)

func TestBuildSDSResultPayload(t *testing.T) {
	tests := []struct {
		name string
		// rules registered for taskID "task" (when scanner is true).
		rules map[string]string
		// scanner controls whether a scanner is registered for taskID "task".
		scanner bool
		// raw is the JSON query-result event submitted to the forwarder.
		raw string
		// expectedJSON is the protojson rendering (proto field names) of the
		// resulting payload.
		expectedJSON string
		expectErr    bool
	}{
		{
			name:    "counts matched rows once per row",
			rules:   map[string]string{"@": "email-rule"},
			scanner: true,
			// "x@@y" has two hits but must be counted once; "name" matches no
			// row so it yields no table match.
			raw: `{
				"timestamp": 1700000000,
				"task_id": "task",
				"db_type": "postgres",
				"db_name": "users",
				"duration_s": 1.5,
				"columns": [
					{"name": "email", "values": ["a@b.com", "x@@y", "plain"]},
					{"name": "name", "values": ["alice", "bob"]}
				]
			}`,
			expectedJSON: `{
				"timestamp": "1700000000",
				"resource": {"type": "postgres", "name": "users"},
				"scan_results": [{
					"duration": "1500",
					"table_matches": [
						{"rule_id": "email-rule", "column_name": "email", "count_matched_rows": "2"}
					]
				}]
			}`,
		},
		{
			name:    "multiple columns each match a distinct rule",
			rules:   map[string]string{"@": "email-rule", "#": "phone-rule"},
			scanner: true,
			raw: `{
				"task_id": "task",
				"db_type": "mysql",
				"db_name": "contacts",
				"duration_s": 0.2,
				"columns": [
					{"name": "email", "values": ["a@b", "c@d"]},
					{"name": "phone", "values": ["1#2"]}
				]
			}`,
			expectedJSON: `{
				"resource": {"type": "mysql", "name": "contacts"},
				"scan_results": [{
					"duration": "200",
					"table_matches": [
						{"rule_id": "email-rule", "column_name": "email", "count_matched_rows": "2"},
						{"rule_id": "phone-rule", "column_name": "phone", "count_matched_rows": "1"}
					]
				}]
			}`,
		},
		{
			name:    "no matches yields a scan result without table matches",
			rules:   map[string]string{"@": "email-rule"},
			scanner: true,
			raw: `{
				"task_id": "task",
				"db_type": "snowflake",
				"db_name": "warehouse",
				"duration_s": 2,
				"columns": [{"name": "col", "values": ["plain", "none"]}]
			}`,
			expectedJSON: `{
				"resource": {"type": "snowflake", "name": "warehouse"},
				"scan_results": [{"duration": "2000"}]
			}`,
		},
		{
			name:      "no scanner registered for task",
			scanner:   false,
			raw:       `{"task_id": "task"}`,
			expectErr: true,
		},
		{
			name:      "invalid json",
			raw:       "not-json",
			expectErr: true,
		},
	}

	marshaler := protojson.MarshalOptions{UseProtoNames: true}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scanners := fakeScanners{scanners: map[string]sds.Scanner{}}
			if tc.scanner {
				scanners.scanners["task"] = fakeScanner{rules: tc.rules}
			}

			out, err := buildSDSResultPayload(scanners, []byte(tc.raw))
			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			var payload sdspb.SdsResultPayload
			require.NoError(t, payload.UnmarshalVT(out))

			gotJSON, err := marshaler.Marshal(&payload)
			require.NoError(t, err)
			assert.JSONEq(t, tc.expectedJSON, string(gotJSON))
		})
	}
}
