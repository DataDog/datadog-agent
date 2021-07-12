package snmp

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

func Test_transformIndex(t *testing.T) {
	tests := []struct {
		name               string
		indexes            []string
		transformRules     []metricIndexTransform
		expectedNewIndexes []string
	}{
		{
			"no rule",
			[]string{"10", "11", "12", "13"},
			[]metricIndexTransform{},
			nil,
		},
		{
			"one",
			[]string{"10", "11", "12", "13"},
			[]metricIndexTransform{
				{2, 3},
			},
			[]string{"12", "13"},
		},
		{
			"multi",
			[]string{"10", "11", "12", "13"},
			[]metricIndexTransform{
				{2, 2},
				{0, 1},
			},
			[]string{"12", "10", "11"},
		},
		{
			"out of index end",
			[]string{"10", "11", "12", "13"},
			[]metricIndexTransform{
				{2, 1000},
			},
			nil,
		},
		{
			"out of index start and end",
			[]string{"10", "11", "12", "13"},
			[]metricIndexTransform{
				{1000, 2000},
			},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newIndexes := transformIndex(tt.indexes, tt.transformRules)
			assert.Equal(t, tt.expectedNewIndexes, newIndexes)
		})
	}
}

func Test_metricsConfig_getTags(t *testing.T) {
	type logCount struct {
		log   string
		count int
	}
	tests := []struct {
		name            string
		rawMetricConfig []byte
		fullIndex       string
		values          *resultValueStore
		expectedTags    []string
		expectedLogs    []logCount
	}{
		{
			name: "index transform",
			// language=yaml
			rawMetricConfig: []byte(`
table:
  OID:  1.2.3.4.5
  name: cpiPduBranchTable
symbols:
  - OID: 1.2.3.4.5.1.2
    name: cpiPduBranchCurrent
metric_tags:
  - column:
      OID:  1.2.3.4.8.1.2
      name: cpiPduName
    table: cpiPduTable
    index_transform:
      - start: 1
        end: 2
      - start: 6
        end: 7
    tag: pdu_name
`),
			fullIndex: "1.2.3.4.5.6.7.8",
			values: &resultValueStore{
				columnValues: map[string]map[string]snmpValueType{
					"1.2.3.4.8.1.2": {
						"2.3.7.8": snmpValueType{
							value: "myval",
						},
					},
				},
			},
			expectedTags: []string{"pdu_name:myval"},
		},
		{
			name: "index mapping",
			// language=yaml
			rawMetricConfig: []byte(`
table:
  OID: 1.3.6.1.2.1.4.31.3
  name: ipIfStatsTable
symbols:
  - OID: 1.3.6.1.2.1.4.31.3.1.6
    name: ipIfStatsHCInOctets
metric_tags:
  - index: 1
    tag: ipversion
    mapping:
      0: unknown
      1: ipv4
      2: ipv6
      3: ipv4z
      4: ipv6z
      16: dns
`),
			fullIndex:    "3",
			values:       &resultValueStore{},
			expectedTags: []string{"ipversion:ipv4z"},
		},
		{
			name: "regex match",
			// language=yaml
			rawMetricConfig: []byte(`
table:
  OID:  1.2.3.4.5
  name: cpiPduBranchTable
symbols:
  - OID: 1.2.3.4.5.1.2
    name: cpiPduBranchCurrent
metric_tags:
  - column:
      OID:  1.2.3.4.8.1.2
      name: cpiPduName
    table: cpiPduTable
    match: '(\w)(\w+)'
    tags:
      prefix: '$1'
      suffix: '$2'
`),
			fullIndex: "1.2.3.4.5.6.7.8",
			values: &resultValueStore{
				columnValues: map[string]map[string]snmpValueType{
					"1.2.3.4.8.1.2": {
						"1.2.3.4.5.6.7.8": snmpValueType{
							value: "eth0",
						},
					},
				},
			},
			expectedTags: []string{"prefix:e", "suffix:th0"},
		},
		{
			name: "regex match only once",
			// language=yaml
			rawMetricConfig: []byte(`
table:
  OID:  1.2.3.4.5
  name: cpiPduBranchTable
symbols:
  - OID: 1.2.3.4.5.1.2
    name: cpiPduBranchCurrent
metric_tags:
  - column:
      OID:  1.2.3.4.8.1.2
      name: cpiPduName
    table: cpiPduTable
    match: '([A-z0-9]*)-([A-z]*[-A-z]*)-([A-z0-9]*)'
    tags:
      tag1: '${1}'
      tag2: '\1'
`),
			fullIndex: "1.2.3.4.5.6.7.8",
			values: &resultValueStore{
				columnValues: map[string]map[string]snmpValueType{
					"1.2.3.4.8.1.2": {
						"1.2.3.4.5.6.7.8": snmpValueType{
							value: "f5-vm-aa.c.datadog-integrations-lab.internal",
						},
					},
				},
			},
			expectedTags: []string{"tag1:f5", "tag2:f5"},
		},
		{
			name: "regex does not match",
			// language=yaml
			rawMetricConfig: []byte(`
table:
  OID:  1.2.3.4.5
  name: cpiPduBranchTable
symbols:
  - OID: 1.2.3.4.5.1.2
    name: cpiPduBranchCurrent
metric_tags:
  - column:
      OID:  1.2.3.4.8.1.2
      name: cpiPduName
    table: cpiPduTable
    match: '(\w)(\w+)'
    tags:
      prefix: '$1'
      suffix: '$2'
`),
			fullIndex: "1.2.3.4.5.6.7.8",
			values: &resultValueStore{
				columnValues: map[string]map[string]snmpValueType{
					"1.2.3.4.8.1.2": {
						"1.2.3.4.5.6.7.8": snmpValueType{
							value: "....",
						},
					},
				},
			},
			expectedTags: []string(nil),
		},
		{
			name: "regex does not match exact",
			// language=yaml
			rawMetricConfig: []byte(`
table:
  OID:  1.2.3.4.5
  name: cpiPduBranchTable
symbols:
  - OID: 1.2.3.4.5.1.2
    name: cpiPduBranchCurrent
metric_tags:
  - column:
      OID:  1.2.3.4.8.1.2
      name: cpiPduName
    table: cpiPduTable
    match: '^(\w)(\w+)$'
    tags:
      prefix: '$1'
      suffix: '$2'
`),
			fullIndex: "1.2.3.4.5.6.7.8",
			values: &resultValueStore{
				columnValues: map[string]map[string]snmpValueType{
					"1.2.3.4.8.1.2": {
						"1.2.3.4.5.6.7.8": snmpValueType{
							value: "abc.",
						},
					},
				},
			},
			expectedTags: []string(nil),
		},
		{
			name: "missing index value",
			// language=yaml
			rawMetricConfig: []byte(`
table:
  OID:  1.2.3.4.5
  name: cpiPduBranchTable
symbols:
  - OID: 1.2.3.4.5.1.2
    name: cpiPduBranchCurrent
metric_tags:
  - column:
      OID:  1.2.3.4.8.1.2
      name: cpiPduName
    table: cpiPduTable
    tag: abc
`),
			fullIndex: "1.2.3.4.5.6.7.8",
			values: &resultValueStore{
				columnValues: map[string]map[string]snmpValueType{
					"1.2.3.4.8.1.2": {
						"999": snmpValueType{
							value: "abc.",
						},
					},
				},
			},
			expectedTags: []string(nil),
			expectedLogs: []logCount{
				{"[DEBUG] getTags: index not found for column value: tag=abc, index=1.2.3.4.5.6.7.8", 1},
			},
		},
		{
			name: "error converting tag value",
			// language=yaml
			rawMetricConfig: []byte(`
table:
  OID:  1.2.3.4.5
  name: cpiPduBranchTable
symbols:
  - OID: 1.2.3.4.5.1.2
    name: cpiPduBranchCurrent
metric_tags:
  - column:
      OID:  1.2.3.4.8.1.2
      name: cpiPduName
    table: cpiPduTable
    tag: abc
`),
			fullIndex: "1.2.3.4.5.6.7.8",
			values: &resultValueStore{
				columnValues: map[string]map[string]snmpValueType{
					"1.2.3.4.8.1.2": {
						"1.2.3.4.5.6.7.8": snmpValueType{
							value: snmpValueType{},
						},
					},
				},
			},
			expectedTags: []string(nil),
			expectedLogs: []logCount{
				{"[DEBUG] getTags: error converting tagValue", 1},
			},
		},
		{
			name: "missing column value",
			// language=yaml
			rawMetricConfig: []byte(`
table:
  OID:  1.2.3.4.5
  name: cpiPduBranchTable
symbols:
  - OID: 1.2.3.4.5.1.2
    name: cpiPduBranchCurrent
metric_tags:
  - column:
      OID:  1.2.3.4.8.1.2
      name: cpiPduName
    table: cpiPduTable
    tag: abc
`),
			fullIndex: "1.2.3.4.5.6.7.8",
			values: &resultValueStore{
				columnValues: map[string]map[string]snmpValueType{
					"999": {
						"1.2.3.4.5.6.7.8": snmpValueType{
							value: "abc.",
						},
					},
				},
			},
			expectedTags: []string(nil),
			expectedLogs: []logCount{
				{"[DEBUG] getTags: error getting column value: value for Column OID `1.2.3.4.8.1.2`", 1},
			},
		},
		{
			name: "mapping does not exist",
			// language=yaml
			rawMetricConfig: []byte(`
table:
  OID:  1.2.3.4.5
  name: cpiPduBranchTable
symbols:
  - OID: 1.2.3.4.5.1.2
    name: cpiPduBranchCurrent
metric_tags:
  - index: 1
    tag: abc
    mapping:
      0: unknown
      1: ipv4
      2: ipv6
      3: ipv4z
      4: ipv6z
      16: dns
`),
			fullIndex: "20",
			values: &resultValueStore{
				columnValues: map[string]map[string]snmpValueType{
					"1.2.3.4.8.1.2": {
						"20": snmpValueType{
							value: "abc.",
						},
					},
				},
			},
			expectedTags: []string(nil),
			expectedLogs: []logCount{
				{"[DEBUG] getTags: error getting tags. mapping for `20` does not exist.", 1},
			},
		},
		{
			name: "index not found",
			// language=yaml
			rawMetricConfig: []byte(`
table:
  OID:  1.2.3.4.5
  name: cpiPduBranchTable
symbols:
  - OID: 1.2.3.4.5.1.2
    name: cpiPduBranchCurrent
metric_tags:
  - index: 100
    tag: abc
`),
			fullIndex: "1",
			values: &resultValueStore{
				columnValues: map[string]map[string]snmpValueType{
					"1.2.3.4.8.1.2": {
						"1": snmpValueType{
							value: "abc.",
						},
					},
				},
			},
			expectedTags: []string(nil),
			expectedLogs: []logCount{
				{"[DEBUG] getTags: error getting tags. index `100` not found in indexes `[1]`", 1},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			w := bufio.NewWriter(&b)

			l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
			assert.Nil(t, err)
			log.SetupLogger(l, "debug")

			m := metricsConfig{}
			yaml.Unmarshal(tt.rawMetricConfig, &m)

			validateEnrichMetrics([]metricsConfig{m})
			tags := m.getTags(tt.fullIndex, tt.values)

			assert.ElementsMatch(t, tt.expectedTags, tags)

			w.Flush()
			logs := b.String()

			for _, aLogCount := range tt.expectedLogs {
				assert.Equal(t, aLogCount.count, strings.Count(logs, aLogCount.log), logs)
			}
		})
	}
}

func Test_normalizeRegexReplaceValue(t *testing.T) {
	tests := []struct {
		val                   string
		expectedReplacedValue string
	}{
		{
			"abc",
			"abc",
		},
		{
			"a\\1b",
			"a$1b",
		},
		{
			"a$1b",
			"a$1b",
		},
		{
			"\\1",
			"$1",
		},
		{
			"\\2",
			"$2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.val, func(t *testing.T) {
			assert.Equal(t, tt.expectedReplacedValue, normalizeRegexReplaceValue(tt.val))
		})
	}
}
