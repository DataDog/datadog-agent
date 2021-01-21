package snmp

import (
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	"testing"
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
	tests := []struct {
		name            string
		rawMetricConfig []byte
		fullIndex       string
		values          *resultValueStore
		expectedTags    []string
	}{
		{
			"index transform",
			[]byte(`
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
			"1.2.3.4.5.6.7.8",
			&resultValueStore{
				columnValues: map[string]map[string]snmpValueType{
					"1.2.3.4.8.1.2": {
						"2.3.7.8": snmpValueType{
							value: "myval",
						},
					},
				},
			},
			[]string{"pdu_name:myval"},
		},
		{
			"index mapping",
			[]byte(`
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
			"3",
			&resultValueStore{},
			[]string{"ipversion:ipv4z"},
		},
		{
			"regex match",
			[]byte(`
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
			"1.2.3.4.5.6.7.8",
			&resultValueStore{
				columnValues: map[string]map[string]snmpValueType{
					"1.2.3.4.8.1.2": {
						"1.2.3.4.5.6.7.8": snmpValueType{
							value: "eth0",
						},
					},
				},
			},
			[]string{"prefix:e", "suffix:th0"},
		},
		{
			"regex does not match",
			[]byte(`
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
			"1.2.3.4.5.6.7.8",
			&resultValueStore{
				columnValues: map[string]map[string]snmpValueType{
					"1.2.3.4.8.1.2": {
						"1.2.3.4.5.6.7.8": snmpValueType{
							value: "....",
						},
					},
				},
			},
			[]string(nil),
		},
		{
			"regex does not match exact",
			[]byte(`
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
			"1.2.3.4.5.6.7.8",
			&resultValueStore{
				columnValues: map[string]map[string]snmpValueType{
					"1.2.3.4.8.1.2": {
						"1.2.3.4.5.6.7.8": snmpValueType{
							value: "abc.",
						},
					},
				},
			},
			[]string(nil),
		},
		{
			"missing index value",
			[]byte(`
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
			"1.2.3.4.5.6.7.8",
			&resultValueStore{
				columnValues: map[string]map[string]snmpValueType{
					"1.2.3.4.8.1.2": {
						"999": snmpValueType{
							value: "abc.",
						},
					},
				},
			},
			[]string(nil),
		},
		{
			"missing column value",
			[]byte(`
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
			"1.2.3.4.5.6.7.8",
			&resultValueStore{
				columnValues: map[string]map[string]snmpValueType{
					"999": {
						"1.2.3.4.5.6.7.8": snmpValueType{
							value: "abc.",
						},
					},
				},
			},
			[]string(nil),
		},
		{
			"mapping does not exist",
			[]byte(`
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
			"20",
			&resultValueStore{
				columnValues: map[string]map[string]snmpValueType{
					"1.2.3.4.8.1.2": {
						"20": snmpValueType{
							value: "abc.",
						},
					},
				},
			},
			[]string(nil),
		},
		{
			"index not found",
			[]byte(`
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
			"1",
			&resultValueStore{
				columnValues: map[string]map[string]snmpValueType{
					"1.2.3.4.8.1.2": {
						"1": snmpValueType{
							value: "abc.",
						},
					},
				},
			},
			[]string(nil),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := metricsConfig{}
			yaml.Unmarshal(tt.rawMetricConfig, &m)

			tags := m.getTags(tt.fullIndex, tt.values)

			assert.ElementsMatch(t, tt.expectedTags, tags)
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
