package snmp

import (
	"fmt"
	"sync"
)

// columnResultValuesType is used to store results fetched for column oids
// Structure: map[<COLUMN OIDS AS STRING>]map[<ROW INDEX>]snmpValueType
// - the first map key is the table column oid
// - the second map key is the index part of oid (not prefixed with column oid)
type columnResultValuesType map[string]map[string]snmpValueType

type fetchColumnResults struct {
	values map[string]map[string]snmpValueType
	mu     sync.Mutex
}

func newFetchColumnResults(totalColumnOids int) *fetchColumnResults {
	return &fetchColumnResults{
		values: make(map[string]map[string]snmpValueType, totalColumnOids),
	}
}

func (f *fetchColumnResults) addOids(columnOid string, instanceOids map[string]snmpValueType) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, ok := f.values[columnOid]; !ok {
		f.values[columnOid] = instanceOids
		return
	}
	for oid, value := range instanceOids {
		f.values[columnOid][oid] = value
	}
}

// scalarResultValuesType is used to store results fetched for scalar oids
// Structure: map[<INSTANCE OID VALUE>]snmpValueType
// - the instance oid value (suffixed with `.0`)
type scalarResultValuesType map[string]snmpValueType

func fetchValues(session sessionAPI, config snmpConfig) (*resultValueStore, error) {
	// fetch scalar values
	scalarResults, err := fetchScalarOidsWithBatching(session, config.oidConfig.scalarOids, config.oidBatchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch scalar oids with batching: %v", err)
	}

	// fetch column values
	oids := make(map[string]string, len(config.oidConfig.columnOids))
	for _, value := range config.oidConfig.columnOids {
		oids[value] = value
	}
	columnResults, err := fetchColumnOidsWithBatching(session, oids, config.oidBatchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch oids with batching: %v", err)
	}

	return &resultValueStore{scalarResults, columnResults}, nil
}
