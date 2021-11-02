package fetch

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/session"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/valuestore"
)

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
	columnResults, err := fetchColumnOidsWithBatching(sess, oids, config.OidBatchSize, config.BulkMaxRepetitions)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch oids with batching: %v", err)
	}

	return &valuestore.ResultValueStore{ScalarValues: scalarResults, ColumnValues: columnResults}, nil
}
