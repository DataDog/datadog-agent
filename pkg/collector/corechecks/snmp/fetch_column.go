package snmp

import (
	"fmt"
	"sort"

	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func fetchColumnOidsWithBatching(session sessionAPI, oids map[string]string, oidBatchSize int) (columnResultValuesType, error) {
	retValues := make(columnResultValuesType, len(oids))

	columnOids := getOidsMapKeys(oids)
	sort.Strings(columnOids) // sorting columnOids to make them deterministic for testing purpose
	batches, err := createStringBatches(columnOids, oidBatchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create column oid batches: %s", err)
	}

	for _, batchColumnOids := range batches {
		oidsToFetch := make(map[string]string, len(batchColumnOids))
		for _, oid := range batchColumnOids {
			oidsToFetch[oid] = oids[oid]
		}

		results, err := fetchColumnOids(session, oidsToFetch)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch column oids: %s", err)
		}

		for columnOid, instanceOids := range results {
			if _, ok := retValues[columnOid]; !ok {
				retValues[columnOid] = instanceOids
				continue
			}
			for oid, value := range instanceOids {
				retValues[columnOid][oid] = value
			}
		}
	}
	return retValues, nil
}

// fetchColumnOids has an `oids` argument representing a `map[string]string`,
// the key of the map is the column oid, and the value is the oid used to fetch the next value for the column.
// The value oid might be equal to column oid or a row oid of the same column.
func fetchColumnOids(session sessionAPI, oids map[string]string) (columnResultValuesType, error) {
	returnValues := make(columnResultValuesType, len(oids))
	curOids := oids
	for {
		if len(curOids) == 0 {
			break
		}
		log.Debugf("fetch column: request oids: %v", curOids)
		var columnOids, requestOids []string
		for k, v := range curOids {
			columnOids = append(columnOids, k)
			requestOids = append(requestOids, v)
		}
		// sorting columnOids and requestOids to make them deterministic for testing purpose
		sort.Strings(columnOids)
		sort.Strings(requestOids)

		results, err := getResults(session, requestOids)
		if err != nil {
			return nil, err
		}
		newValues, nextOids := resultToColumnValues(columnOids, results)
		updateColumnResultValues(returnValues, newValues)
		curOids = nextOids
	}
	return returnValues, nil
}

func getResults(session sessionAPI, requestOids []string) (*gosnmp.SnmpPacket, error) {
	var results *gosnmp.SnmpPacket
	if session.GetVersion() == gosnmp.Version1 {
		// snmp v1 doesn't support GetBulk
		getNextResults, err := session.GetNext(requestOids)
		if err != nil {
			log.Debugf("fetch column: failed getting oids `%v` using GetNext: %s", requestOids, err)
			return nil, fmt.Errorf("fetch column: failed getting oids `%v` using GetNext: %s", requestOids, err)
		}
		results = getNextResults
		log.Debugf("fetch column: GetNext results Variables: %v", results.Variables)
	} else {
		getBulkResults, err := session.GetBulk(requestOids)
		if err != nil {
			log.Debugf("fetch column: failed getting oids `%v` using GetBulk: %s", requestOids, err)
			return nil, fmt.Errorf("fetch column: failed getting oids `%v` using GetBulk: %s", requestOids, err)
		}
		results = getBulkResults
		log.Debugf("fetch column: GetBulk results Variables: %v", results.Variables)
	}
	return results, nil
}

func updateColumnResultValues(valuesToUpdate columnResultValuesType, extraValues columnResultValuesType) {
	for columnOid, columnValues := range extraValues {
		for oid, value := range columnValues {
			if _, ok := valuesToUpdate[columnOid]; !ok {
				valuesToUpdate[columnOid] = make(map[string]snmpValueType)
			}
			valuesToUpdate[columnOid][oid] = value
		}
	}
}

func getOidsMapKeys(oidsMap map[string]string) []string {
	keys := make([]string, len(oidsMap))
	i := 0
	for k := range oidsMap {
		keys[i] = k
		i++
	}
	return keys
}
