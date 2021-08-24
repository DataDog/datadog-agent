package snmp

import (
	"fmt"
)

// columnResultValuesType is used to store results fetched for column oids
// Structure: map[<COLUMN OIDS AS STRING>]map[<ROW INDEX>]ResultValue
// - the first map key is the table column oid
// - the second map key is the index part of oid (not prefixed with column oid)
type columnResultValuesType map[string]map[string]ResultValue

// scalarResultValuesType is used to store results fetched for scalar oids
// Structure: map[<INSTANCE OID VALUE>]ResultValue
// - the instance oid value (suffixed with `.0`)
type scalarResultValuesType map[string]ResultValue

func fetchValues(session sessionAPI, config CheckConfig) (*ResultValueStore, error) {
	// fetch scalar values
	scalarResults, err := fetchScalarOidsWithBatching(session, config.oidConfig.ScalarOids, config.oidBatchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch scalar oids with batching: %v", err)
	}

	// fetch column values
	oids := make(map[string]string, len(config.oidConfig.ColumnOids))
	for _, value := range config.oidConfig.ColumnOids {
		oids[value] = value
	}
	columnResults, err := fetchColumnOidsWithBatching(session, oids, config.oidBatchSize, config.bulkMaxRepetitions)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch oids with batching: %v", err)
	}

	return &ResultValueStore{scalarResults, columnResults}, nil
}
