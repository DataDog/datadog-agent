// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build !windows
// +build snmp

package network

import (
	"bytes"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/k-sone/snmpgo"
)

const goodV2Cfg = `
ip_address: localhost
port: 161
community_string: public
snmp_version: 2 # Only required for snmp v1, will default to 2
timeout: 1 # second, by default
retries: 5
tags:
  - optional_tag_1
  - optional_tag_2
metrics:
  - MIB: UDP-MIB
    symbol: udpInDatagrams
  - MIB: TCP-MIB
    symbol: tcpActiveOpens
  - OID: 1.3.6.1.2.1.6.6
    name: tcpPassiveOpens
  - OID: 1.3.6.1.4.1.3375.2.1.1.2.1.8.0
    name: F5_TotalCurrentConnections
    forced_type: gauge
  - MIB: IF-MIB
    table: ifTable
    symbols:
      - ifInOctets
      - ifOutOctets
    metric_tags:
      - tag: interface
        column: ifDescr  # specify which column to read the tag value from
  - MIB: IP-MIB
    table: ipSystemStatsTable
    symbols:
      - ipSystemStatsInReceives
    metric_tags:
      - tag: ipversion
        index: 1
`

const goodV3Cfg = `
ip_address: 192.168.34.10
# port: 161 # default value
user: user
authKey: password
privKey: private_key
authProtocol: authProtocol
privProtocol: privProtocol
timeout: 1 # second, by default
retries: 5
tags:
  - optional_tag_1
  - optional_tag_2
metrics:
  - MIB: UDP-MIB
    symbol: udpInDatagrams
  - MIB: TCP-MIB
    symbol: tcpActiveOpens
`

const basicCfg = `
ip_address: localhost
port: 161
community_string: public
snmp_version: 2 # Only required for snmp v1, will default to 2
timeout: 1 # second, by default
retries: 5
tags:
  - optional_tag_1
  - optional_tag_2
metrics:
  - MIB: UDP-MIB
    symbol: udpInDatagrams
  - MIB: TCP-MIB
    symbol: tcpActiveOpens
  - OID: 1.3.6.1.2.1.6.6
    name: tcpPassiveOpens
  - OID: 1.3.6.1.4.1.3375.2.1.1.2.1.8.0
    name: F5_TotalCurrentConnections
    forced_type: gauge
`

func TestTextualOIDConversion(t *testing.T) {

	oid := "1.3.6.1.2.1.4.31.1.1.3"

	initCNetSnmpLib(nil)

	textOID, err := getTextualOID(oid)
	if err != nil {
		t.Fatalf("Unable to get index tag: %v", err)
	}

	if textOID != "IP-MIB::ipSystemStatsInReceives" {
		t.Fatalf("Incorrect tag retrieved. Expected IP-MIB::ipSystemStatsInReceives got: %v", textOID)
	}
}

func TestIndexTagExtraction(t *testing.T) {
	base := "1.3.6.1.2.1.4.31.1.1.3"
	oid := "1.3.6.1.2.1.4.31.1.1.3.1.4"
	oidv6 := "1.3.6.1.2.1.4.31.1.1.3.2.4.9.1"

	initCNetSnmpLib(nil)

	tag, err := getIndexTag(base, oid, 1)
	if err != nil {
		t.Fatalf("Unable to get index tag: %v", err)
	}

	if tag != "ipv4" {
		t.Fatalf("Incorrect tag retrieved. Expected ipv4 got: %v", tag)
	}

	tag, err = getIndexTag(base, oidv6, 1)
	if err != nil {
		t.Fatalf("Unable to get index tag: %v", err)
	}

	if tag != "ipv6" {
		t.Fatalf("Incorrect tag retrieved. Expected ipv6 got: %v", tag)
	}
}

func TestConfigureV2(t *testing.T) {
	cfg := new(snmpConfig)

	err := cfg.Parse(bytes.NewBufferString(goodV2Cfg).Bytes(), []byte{})
	if err != nil {
		t.Fatalf("Unable to parse configuration: %v", err)
	}

	if cfg.instance.Host != "localhost" {
		t.Fatalf("Failed hostname: expected 'localhost' got '%v'", cfg.instance.Host)
	}
	if cfg.instance.Port != 161 {
		t.Fatalf("Failed port: expected '161' got '%v'", cfg.instance.Port)
	}
	if cfg.instance.Community != "public" {
		t.Fatalf("Failed community: expected 'public' got '%v'", cfg.instance.Community)
	}
	if cfg.instance.Version != 2 {
		t.Fatalf("Failed snmp version: expected '2' got '%v'", cfg.instance.Version)
	}
	if cfg.instance.Timeout != 1 {
		t.Fatalf("Failed retries: expected '5' got '%v'", cfg.instance.Retries)
	}
	if cfg.instance.Retries != 5 {
		t.Fatalf("Failed retries: expected '5' got '%v'", cfg.instance.Retries)
	}

	tag1Found := false
	tag2Found := false

	for _, tag := range cfg.instance.Tags {
		if tag == "optional_tag_1" {
			tag1Found = true
		}
		if tag == "optional_tag_2" {
			tag2Found = true
		}
	}

	if !tag1Found || !tag2Found {
		t.Fatalf("Instance tags not properly unmarshalled.")
	}
}

func TestConfigureV3(t *testing.T) {
	cfg := new(snmpConfig)

	err := cfg.Parse(bytes.NewBufferString(goodV3Cfg).Bytes(), []byte{})
	if err != nil {
		t.Fatalf("Unable to parse configuration: %v", err)
	}
}

func TestSubmitSNMP(t *testing.T) {
	snmpCheck := new(SNMPCheck)
	cfg := new(snmpConfig)

	initCNetSnmpLib(nil)

	mock := aggregator.NewMockSender(snmpCheck.ID())

	if err := cfg.Parse(bytes.NewBufferString(basicCfg).Bytes(), []byte{}); err != nil {
		t.Fatalf("Unable to parse configuration: %v", err)
	}
	snmpCheck.cfg = cfg

	if err := snmpCheck.cfg.instance.generateOIDs(); err != nil {
		t.Fatalf("Unable to generate OIDs: %v", err)
	}
	if err := snmpCheck.cfg.instance.generateTagMap(); err != nil {
		t.Fatalf("Unable to generate OIDs: %v", err)
	}

	//Check OIDs...
	oidvalues := snmpCheck.cfg.instance.OIDTranslator.Values()
	oidList := make([]string, len(oidvalues))
	for i, v := range oidvalues {
		//should be true for each v
		if vstr, ok := v.(string); ok {
			oidList[i] = vstr
		}
	}

	oids, err := snmpgo.NewOids(oidList)
	if err != nil {
		// Failed creating Native OID list.
		t.Fatalf("Unable to create Native OID list: %v", err)
	}

	//Mocked VarBinds
	binds := []*snmpgo.VarBind{}
	mtype := make(map[string]string)
	cnt := 0
	for _, oid := range oids {
		v := snmpgo.VarBind{Oid: oid}
		if oid.String() == "1.3.6.1.4.1.3375.2.1.1.2.1.8.0" {
			// forced type - gauge despite being counter
			v.Variable = snmpgo.NewCounter32(uint32(42949670))
			mtype[oid.String()] = "Gauge"
		} else {
			switch cnt { // just different acceptable types.
			case 0:
				v.Variable = snmpgo.NewInteger(int32(42949670))
				mtype[oid.String()] = "Gauge"
			case 1:
				v.Variable = snmpgo.NewGauge32(uint32(42949670))
				mtype[oid.String()] = "Gauge"
			case 2:
				v.Variable = snmpgo.NewCounter64(uint64(42949670))
				mtype[oid.String()] = "Rate"
			}
			cnt++
		}
		binds = append(binds, &v)
	}

	expectedTags := []string{
		"optional_tag_1",
		"optional_tag_2",
		"snmp_device:localhost:161"}

	for _, oid := range oids {
		symbolicOID, err := snmpCheck.cfg.instance.OIDTranslator.GetKVReverse(oid.String())
		if err != nil {
			t.Fatalf("Unable to get symbolic OID for assertion: %v", err)
		}

		name := ""
		if m, ok := snmpCheck.cfg.instance.MetricMap[oid.String()]; ok {
			if m.Name != "" {
				name = "snmp." + m.Name
			}
		}
		if m, ok := symbolicOID.(string); ok {
			if name == "" {
				if metric, ok := snmpCheck.cfg.instance.NameLookup[m]; ok {
					name = metric
				} else {
					name = "snmp." + m
				}
			}
			mt, ok := mtype[oid.String()]
			if !ok {
				t.FailNow()
			}
			mock.On(mt, name, float64(42949670), "", expectedTags).Return().Times(1)
		}
	}
	mock.On("Commit").Return().Times(1)
	snmpCheck.submitSNMP(oids, binds)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 3)
	mock.AssertNumberOfCalls(t, "Rate", 1)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}
