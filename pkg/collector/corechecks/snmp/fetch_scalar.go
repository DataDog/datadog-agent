package snmp

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/gosnmp/gosnmp"
	"sort"
	"strings"
)

func fetchScalarOidsWithBatching(session sessionAPI, oids []string, oidBatchSize int) (scalarResultValuesType, error) {
	retValues := make(scalarResultValuesType, len(oids))

	batches, err := createStringBatches(oids, oidBatchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create oid batches: %s", err)
	}

	for _, batchOids := range batches {
		results, err := fetchScalarOids(session, batchOids)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch scalar oids: %s", err.Error())
		}
		for k, v := range results {
			retValues[k] = v
		}
	}
	return retValues, nil
}

func fetchScalarOids(session sessionAPI, oids []string) (scalarResultValuesType, error) {
	packet, err := doFetchScalarOids(session, oids)
	if err != nil {
		return nil, err
	}
	values := resultToScalarValues(packet)
	retryFailedScalarOids(session, packet, values)
	return values, nil
}

// retryFailedScalarOids retries on NoSuchObject or NoSuchInstance for scalar oids not ending with `.0`.
// This helps keeping compatibility with python implementation.
// This is not need in normal circumstances where scalar OIDs end with `.0`.
// If the oid does not end with `.0`, we will retry by appending `.0` to it.
func retryFailedScalarOids(session sessionAPI, results *gosnmp.SnmpPacket, valuesToUpdate scalarResultValuesType) {
	retryOids := make(map[string]string)
	for _, variable := range results.Variables {
		oid := strings.TrimLeft(variable.Name, ".")
		if (variable.Type == gosnmp.NoSuchObject || variable.Type == gosnmp.NoSuchInstance) && !strings.HasSuffix(oid, ".0") {
			retryOids[oid] = oid + ".0"
		}
	}
	if len(retryOids) > 0 {
		fetchOids := make([]string, 0, len(retryOids))
		for _, oid := range retryOids {
			fetchOids = append(fetchOids, oid)
		}
		sort.Strings(fetchOids) // needed for stable tests since fetchOids order (from a map values) is undefined
		retryResults, err := doFetchScalarOids(session, fetchOids)
		if err != nil {
			log.Debugf("failed to oids `%v` on retry: %v", retryOids, err)
		} else {
			retryValues := resultToScalarValues(retryResults)
			for initialOid, actualOid := range retryOids {
				if value, ok := retryValues[actualOid]; ok {
					valuesToUpdate[initialOid] = value
				}
			}
		}
	}
}

func doFetchScalarOids(session sessionAPI, oids []string) (*gosnmp.SnmpPacket, error) {
	log.Debugf("fetch scalar: request oids: %v", oids)
	results, err := session.Get(oids)
	log.Debugf("fetch scalar: results: %v", results)
	if err != nil {
		return nil, fmt.Errorf("error getting oids: %s", err.Error())
	}
	return results, nil
}
