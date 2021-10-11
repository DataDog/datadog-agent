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
	session       session.Session
	config        *checkconfig.CheckConfig
	curBulkMaxRep uint32
}

// NewFetcher creates a new instance of Fetcher
func NewFetcher(session session.Session, config *checkconfig.CheckConfig) *Fetcher {
	f := Fetcher{
		session: session,
		config:  config,
	}
	f.resetBulkMaxRepetitions()
	return &f
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

	columnResults, err := f.fetchColumnOidsWithMaxRepAdjustment(oids)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch column oids with batching: %v", err)
	}

	return &valuestore.ResultValueStore{ScalarValues: scalarResults, ColumnValues: columnResults}, nil
}

func (f *Fetcher) fetchColumnOidsWithMaxRepAdjustment(oids map[string]string) (valuestore.ColumnResultValuesType, error) {
	var lastErr error
	var useGetNext bool

	for f.curBulkMaxRep > 0 {
		log.Debugf("fetch column oids (oidBatchSize=%d, curBulkMaxRep=%d)", f.config.OidBatchSize, f.curBulkMaxRep)
		if f.curBulkMaxRep <= 1 {
			useGetNext = true
		}
		columnResults, err := fetchColumnOidsWithBatching(f.session, oids, f.config.OidBatchSize, f.curBulkMaxRep, useGetNext)
		if err != nil {
			lastErr = err
		} else {
			return columnResults, nil
		}
		f.curBulkMaxRep = f.curBulkMaxRep / 2
	}
	// TODO: test resetBulkMaxRepetitions
	// TODO: test f.curBulkMaxRep is persisted over check runs
	if lastErr != nil {
		f.resetBulkMaxRepetitions()
	}
	return nil, lastErr
}

func (f *Fetcher) resetBulkMaxRepetitions() {
	f.curBulkMaxRep = f.config.BulkMaxRepetitions
}
