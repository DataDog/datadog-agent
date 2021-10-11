package fetch

import (
	"fmt"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/session"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/valuestore"
)

// Fetch oid values from device
// TODO: pass only specific configs instead of the whole CheckConfig
func Fetch(sess session.Session, config *checkconfig.CheckConfig) (*valuestore.ResultValueStore, error) {
	// fetch scalar values
	var scalarResults valuestore.ScalarResultValuesType
	//scalarResults, err := fetchScalarOidsWithBatching(sess, config.OidConfig.ScalarOids, config.OidBatchSize)
	//if err != nil {
	//	return nil, fmt.Errorf("failed to fetch scalar oids with batching: %v", err)
	//}

	// fetch column values
	oids := make(map[string]string, len(config.OidConfig.ColumnOids))
	for _, value := range config.OidConfig.ColumnOids {
		oids[value] = value
	}

	columnResults, err := fetchColumnOidsWithMaxRepAdjustment(sess, oids, config.OidBatchSize, config.BulkMaxRepetitions)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch column oids with batching: %v", err)
	}

	return &valuestore.ResultValueStore{ScalarValues: scalarResults, ColumnValues: columnResults}, nil
}

func fetchColumnOidsWithMaxRepAdjustment(sess session.Session, oids map[string]string, oidBatchSize int, bulkMaxRep uint32) (valuestore.ColumnResultValuesType, error) {
	var lastErr error
	var useGetNext bool

	for bulkMaxRep > 0 {
		log.Debugf("fetch column oids (oidBatchSize=%d, bulkMaxRep=%d)", oidBatchSize, bulkMaxRep)
		if bulkMaxRep <= 1 {
			useGetNext = true
		}
		columnResults, err := fetchColumnOidsWithBatching(sess, oids, oidBatchSize, bulkMaxRep, useGetNext)
		if err != nil {
			lastErr = err
		} else {
			return columnResults, nil
		}
		bulkMaxRep = bulkMaxRep/2
	}
	return nil, lastErr
}
