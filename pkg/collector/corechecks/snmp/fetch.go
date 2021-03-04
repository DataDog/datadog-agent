package snmp

import (
	"fmt"
)

// columnResultValuesType is used to store results fetched for column oids
// Structure: map[<COLUMN OIDS AS STRING>]map[<ROW INDEX>]snmpValueType
// - the first map key is the table column oid
// - the second map key is the index part of oid (not prefixed with column oid)
type columnResultValuesType map[string]map[string]snmpValueType

// scalarResultValuesType is used to store results fetched for scalar oids
// Structure: map[<INSTANCE OID VALUE>]snmpValueType
// - the instance oid value (suffixed with `.0`)
type scalarResultValuesType map[string]snmpValueType

func fetchValues(session sessionAPI, config snmpConfig) (*resultValueStore, error) {
	// fetch scalar values
	scalarResults, err := fetchScalarOidsWithBatching(session, config.oidConfig.scalarOids, config.oidBatchSize)
	if err != nil {
		return &resultValueStore{}, fmt.Errorf("failed to fetch scalar oids with batching: %v", err)
	}

	// fetch column values
	oids := make(map[string]string, len(config.oidConfig.columnOids))
	for _, value := range config.oidConfig.columnOids {
		oids[value] = value
	}
	columnResults, err := fetchColumnOidsWithBatching(session, oids, config.oidBatchSize)
	if err != nil {
		return &resultValueStore{}, fmt.Errorf("failed to fetch oids with batching: %v", err)
	}

	return &resultValueStore{scalarResults, columnResults}, nil
}
