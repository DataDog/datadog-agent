package fetcher

import (
	"fmt"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/session"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/valuestore"
)

// Fetcher is used to fetch oids from snmp device
type Fetcher struct {
	session session.Session
	config  *checkconfig.CheckConfig
}

// NewFetcher creates a new instance of Fetcher
func NewFetcher(session session.Session, config *checkconfig.CheckConfig) *Fetcher {
	return &Fetcher{
		session: session,
		config:  config,
	}
}

// Fetch oid values from device
// TODO: pass only specific configs instead of the whole CheckConfig
func (f *Fetcher) Fetch() (*valuestore.ResultValueStore, error) {
	// fetch scalar values
	scalarResults, err := fetchScalarOidsWithBatching(f.session, f.config.OidConfig.ScalarOids, f.config.OidBatchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch scalar oids with batching: %v", err)
	}

	// fetch column values
	oids := make(map[string]string, len(f.config.OidConfig.ColumnOids))
	for _, value := range f.config.OidConfig.ColumnOids {
		oids[value] = value
	}

	columnResults, err := fetchColumnOidsWithMaxRepAdjustment(f.session, oids, f.config.OidBatchSize, f.config.BulkMaxRepetitions)
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
		bulkMaxRep = bulkMaxRep / 2
	}
	return nil, lastErr
}
