package snmp

import (
	"fmt"
)

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
