// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fetch

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/common"
	"github.com/gosnmp/gosnmp"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

type columnFetchStrategy int

const (
	useGetBulk columnFetchStrategy = iota
	useGetNext
)

func (c columnFetchStrategy) String() string {
	switch c {
	case useGetBulk:
		return "useGetBulk"
	case useGetNext:
		return "useGetNext"
	default:
		return strconv.Itoa(int(c))
	}
}

const defaultDeviceScanRootOid = "1.0"

// Fetch oid values from device
// TODO: pass only specific configs instead of the whole CheckConfig
func Fetch(sess session.Session, config *checkconfig.CheckConfig) (*valuestore.ResultValueStore, error) {
	// fetch scalar values
	scalarResults, err := fetchScalarOidsWithBatching(sess, config.OidConfig.ScalarOids, config.OidBatchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch scalar oids with batching: %v", err)
	}

	// fetch column values
	oids := make(map[string]string, len(config.OidConfig.ColumnOids))
	for _, value := range config.OidConfig.ColumnOids {
		oids[value] = value
	}

	columnResults, err := fetchColumnOidsWithBatching(sess, oids, config.OidBatchSize, config.BulkMaxRepetitions, useGetBulk)
	if err != nil {
		log.Debugf("failed to fetch oids with GetBulk batching: %v", err)

		columnResults, err = fetchColumnOidsWithBatching(sess, oids, config.OidBatchSize, config.BulkMaxRepetitions, useGetNext)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch oids with GetNext batching: %v", err)
		}
	}

	results := getDeviceScanValues(sess, config)

	return &valuestore.ResultValueStore{ScalarValues: scalarResults, ColumnValues: columnResults, DeviceScanValues: results}, nil
}

// BuildOidTrie2 builds the OIDTrie from a list of OIDs
func BuildOidTrie2(rawOIDTrie map[string]rawOIDTrieNode) *common.OIDTrie {
	newTrieRoot := common.NewOidTrie()
	BuildOidTrie2Recursive(rawOIDTrie, newTrieRoot)
	return newTrieRoot
}

// BuildOidTrie2Recursive builds the OIDTrie from a list of OIDs
func BuildOidTrie2Recursive(rawOIDTrie map[string]rawOIDTrieNode, newTrieNode *common.OIDTrie) {
	for key, node := range rawOIDTrie {
		num, err := strconv.Atoi(key)
		if err != nil {
			log.Warnf("[BuildOidTrie2Recursive] Error %s", err)
			return
		}

		if newTrieNode.Children == nil {
			newTrieNode.Children = make(map[int]*common.OIDTrie)
		}
		if _, ok := newTrieNode.Children[num]; !ok {
			newTrieNode.Children[num] = common.NewOidTrie()
		}
		BuildOidTrie2Recursive(node, newTrieNode.Children[num])
	}
}

//go:embed oid_trie_full.json
var oidTrie []byte

type rawOIDTrieNode map[string]rawOIDTrieNode

func getDeviceScanValues(sess session.Session, config *checkconfig.CheckConfig) []gosnmp.SnmpPDU {
	// TODO: avoid unmarshalling every check run
	rawTrie := rawOIDTrieNode{}
	err := json.Unmarshal(oidTrie, &rawTrie)
	if err != nil {
		log.Warnf("[FetchAllFirstRowOIDsVariables] json.Unmarshal Error %s", err)
		return nil
	}

	//log.Warnf("[FetchAllFirstRowOIDsVariables] RAW oidTrie json %s", string(oidTrie))
	//log.Warnf("[FetchAllFirstRowOIDsVariables] RAW oidTrie struct %s", rawTrie)
	//log.Warnf("[FetchAllFirstRowOIDsVariables] rawTrie %+v", rawTrie)
	newTrie := BuildOidTrie2(rawTrie)

	//newTrieAsJson, _ := json.Marshal(newTrie)
	//log.Warnf("[FetchAllFirstRowOIDsVariables] NEW Trie %+v", string(newTrieAsJson))
	//for _, child := range newTrie.Children {
	//	log.Warnf("[FetchAllFirstRowOIDsVariables] NEW Trie child %+v", child)
	//}

	// TODO: Use a internal type instead of gosnmp.SnmpPDU to avoid leaking gosnmp types ?
	var results []gosnmp.SnmpPDU
	if config.DeviceScanEnabled {
		// TODO: ONLY RUN once every X time

		//rootOid := config.DeviceScanLastOid // default root Oid
		//if rootOid == "" {
		//	// NEW DEVICE SCAN
		//	rootOid = defaultDeviceScanRootOid
		//	config.DeviceScanCurScanStart = time.Now()
		//	config.DeviceScanCurScanOidsCount = 0
		//}

		fetchStart := time.Now()
		fetchedResults := session.FetchAllFirstRowOIDsVariables(sess, newTrie)

		fetchDuration := time.Since(fetchStart)
		log.Warnf("[FetchAllFirstRowOIDsVariables] PRINT PDUs (len: %d)", len(results))
		for _, resultPdu := range fetchedResults {
			log.Warnf("[FetchAllFirstRowOIDsVariables] PDU: %+v", resultPdu)
		}
		//config.DeviceScanCurScanOidsCount += len(fetchedResults)

		log.Warnf("[FetchAllFirstRowOIDsVariables] Device Scan (Total Count: %d, Duration: %.2f Sec)",
			len(fetchedResults),
			fetchDuration.Seconds(),
		)

		//// TODO: ADD TELEMETRY for each check run
		//if len(fetchedResults) == config.DeviceScanMaxOidsPerRun {
		//	log.Warnf("[FetchAllOIDsUsingGetNext] Partial Device Scan (Total Count: %d, Fetch Duration Ms: %d)",
		//		config.DeviceScanCurScanOidsCount,
		//		fetchDuration.Milliseconds(),
		//	)
		//	// Partial Device Scan
		//	//config.DeviceScanLastOid = lastOid
		//} else {
		//	log.Warnf("[FetchAllOIDsUsingGetNext] Full Device Scan (Total Count: %d, Duration: %.2f Sec)",
		//		config.DeviceScanCurScanOidsCount,
		//		time.Since(config.DeviceScanCurScanStart).Seconds(),
		//	)
		//	// TODO: ADD TELEMETRY for complete device scan
		//	// Full Device Scan completed
		//	//config.DeviceScanLastOid = ""
		//}
		results = fetchedResults
	}
	return results
}
