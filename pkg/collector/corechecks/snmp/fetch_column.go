package snmp

import (
	"fmt"
	"sort"
	"sync"

	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func fetchColumnOidsWithBatching(session sessionAPI, oids map[string]string, oidBatchSize int, bulkMaxRepetitions uint32, fetchWorkers int) (columnResultValuesType, error) {
	columnOids := getOidsMapKeys(oids)
	sort.Strings(columnOids) // sorting columnOids to make them deterministic for testing purpose
	batches, err := createStringBatches(columnOids, oidBatchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create column oid batches: %s", err)
	}

	// TODO: TEST ME
	if fetchWorkers > 1 {
		columnValues := fetchColumnOidsWithBatchingAsync(session, oids, bulkMaxRepetitions, fetchWorkers, batches)
		return columnValues, nil
	}
	return fetchColumnOidsWithBatchingSequential(session, oids, batches, bulkMaxRepetitions)
}

func fetchColumnOidsWithBatchingAsync(session sessionAPI, oids map[string]string, bulkMaxRepetitions uint32, fetchWorkers int, batches [][]string) columnResultValuesType {
	columnResults := newFetchColumnResults(len(oids))

	columnOidsBatchesChan := make(chan []string)
	wg := sync.WaitGroup{}

	log.Debugf("fetch column oids with %d workers", fetchWorkers)
	for t := 0; t < fetchWorkers; t++ {
		wg.Add(1)
		go processBatchAsync(columnOidsBatchesChan, &wg, session, oids, columnResults)
	}

	for _, batchColumnOids := range batches {
		columnOidsBatchesChan <- batchColumnOids
	}

	close(columnOidsBatchesChan) // close to indicate there is no more bathes

	wg.Wait() // wait for all workers to finish

	return columnResults.values
}

func fetchColumnOidsWithBatchingSequential(session sessionAPI, oids map[string]string, batches [][]string, bulkMaxRepetitions uint32) (columnResultValuesType, error) {
	columnResults := newFetchColumnResults(len(oids))
	for _, batchColumnOids := range batches {
		err := processBatch(batchColumnOids, session, oids, bulkMaxRepetitions, columnResults)
		if err != nil {
			return nil, err
		}
	}
	return columnResults.values, nil
}

func processBatchAsync(columnOidsBatchesChan chan []string, wg *sync.WaitGroup, session sessionAPI, oids map[string]string, accumulatedColumnResults *fetchColumnResults) {
	defer wg.Done()

	newSession := session.Copy()

	// Create connection
	connErr := newSession.Connect()
	if connErr != nil {
		log.Warnf("failed to connect: %v", connErr)
		return
		//return tags, nil, fmt.Errorf("snmp connection error: %s", connErr)
	}
	defer func() {
		err := newSession.Close()
		if err != nil {
			log.Warnf("failed to close session: %v", err)
		}
	}()

	for batchColumnOids := range columnOidsBatchesChan {
		// do work
		err := processBatch(batchColumnOids, newSession, oids, accumulatedColumnResults)
		if err != nil {
			log.Warnf("failed to process batchColumnOids %v: %s", batchColumnOids, err)
		}
	}
}

func processBatch(batchColumnOids []string, session sessionAPI, oids map[string]string, bulkMaxRepetitions uint32, accumulatedColumnResults *fetchColumnResults) error {
	oidsToFetch := make(map[string]string, len(batchColumnOids))
	for _, oid := range batchColumnOids {
		oidsToFetch[oid] = oids[oid]
	}

		results, err := fetchColumnOids(session, oidsToFetch, bulkMaxRepetitions)
		if err != nil {
			return fmt.Errorf("failed to fetch column oids: %s", err)
		}

	for columnOid, instanceOids := range results {
		accumulatedColumnResults.addOids(columnOid, instanceOids)
	}
	return nil
}

// fetchColumnOids has an `oids` argument representing a `map[string]string`,
// the key of the map is the column oid, and the value is the oid used to fetch the next value for the column.
// The value oid might be equal to column oid or a row oid of the same column.
func fetchColumnOids(session sessionAPI, oids map[string]string, bulkMaxRepetitions uint32) (columnResultValuesType, error) {
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

		results, err := getResults(session, requestOids, bulkMaxRepetitions)
		if err != nil {
			return nil, err
		}
		newValues, nextOids := resultToColumnValues(columnOids, results)
		updateColumnResultValues(returnValues, newValues)
		curOids = nextOids
	}
	return returnValues, nil
}

func getResults(session sessionAPI, requestOids []string, bulkMaxRepetitions uint32) (*gosnmp.SnmpPacket, error) {
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
		getBulkResults, err := session.GetBulk(requestOids, bulkMaxRepetitions)
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
