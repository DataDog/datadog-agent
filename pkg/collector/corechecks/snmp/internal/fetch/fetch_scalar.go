// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fetch

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cihub/seelog"
	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
)

func fetchScalarOidsWithBatching(sess session.Session, oids []string, oidBatchSize int) (valuestore.ScalarResultValuesType, error) {
	retValues := make(valuestore.ScalarResultValuesType, len(oids))

	batches, err := common.CreateStringBatches(oids, oidBatchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create oid batches: %s", err)
	}

	for _, batchOids := range batches {
		results, err := fetchScalarOids(sess, batchOids)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch scalar oids: %s", err.Error())
		}
		for k, v := range results {
			retValues[k] = v
		}
	}
	return retValues, nil
}

func fetchScalarOids(sess session.Session, oids []string) (valuestore.ScalarResultValuesType, error) {
	packet, err := doFetchScalarOids(sess, oids)
	if err != nil {
		return nil, err
	}
	values := valuestore.ResultToScalarValues(packet)
	retryFailedScalarOids(sess, packet, values)
	return values, nil
}

// retryFailedScalarOids retries on NoSuchObject or NoSuchInstance for scalar oids not ending with `.0`.
// This helps keeping compatibility with python implementation.
// This is not need in normal circumstances where scalar OIDs end with `.0`.
// If the oid does not end with `.0`, we will retry by appending `.0` to it.
func retryFailedScalarOids(sess session.Session, results *gosnmp.SnmpPacket, valuesToUpdate valuestore.ScalarResultValuesType) {
	retryOids := make(map[string]string)
	for _, variable := range results.Variables {
		oid := strings.TrimLeft(variable.Name, ".")
		if (variable.Type == gosnmp.NoSuchObject || variable.Type == gosnmp.NoSuchInstance || variable.Type == gosnmp.Null) && !strings.HasSuffix(oid, ".0") {
			retryOids[oid] = oid + ".0"
		}
	}
	if len(retryOids) > 0 {
		fetchOids := make([]string, 0, len(retryOids))
		for _, oid := range retryOids {
			fetchOids = append(fetchOids, oid)
		}
		sort.Strings(fetchOids) // needed for stable tests since fetchOids order (from a map values) is undefined
		retryResults, err := doFetchScalarOids(sess, fetchOids)
		if err != nil {
			log.Debugf("failed to oids `%v` on retry: %v", retryOids, err)
		} else {
			retryValues := valuestore.ResultToScalarValues(retryResults)
			for initialOid, actualOid := range retryOids {
				if value, ok := retryValues[actualOid]; ok {
					valuesToUpdate[initialOid] = value
				}
			}
		}
	}
}

func doFetchScalarOids(session session.Session, oids []string) (*gosnmp.SnmpPacket, error) {
	var results *gosnmp.SnmpPacket
	if session.GetVersion() == gosnmp.Version1 {
		// When using snmp v1, if one of the oids return a NoSuchName, all oids will have value of Null.
		// The response will contain Error=NoSuchName and ErrorIndex with index of the erroneous oid.
		// If that happen, we remove the erroneous oid and try again until we succeed or until there is no oid anymore.
		for {
			scalarOids, err := doDoFetchScalarOids(session, oids)
			if err != nil {
				return nil, err
			}
			if scalarOids.Error == gosnmp.NoSuchName {
				zeroBaseIndex := int(scalarOids.ErrorIndex) - 1 // ScalarOids.ErrorIndex is 1-based
				if (zeroBaseIndex < 0) || (zeroBaseIndex > len(oids)-1) {
					return nil, fmt.Errorf("invalid ErrorIndex `%d` when fetching oids `%v`", scalarOids.ErrorIndex, oids)
				}
				oids = append(oids[:zeroBaseIndex], oids[zeroBaseIndex+1:]...)
				if len(oids) == 0 {
					// If all oids are not found, return an empty packet with no variable and no error
					return &gosnmp.SnmpPacket{}, nil
				}
				continue
			}
			results = scalarOids
			break
		}
	} else {
		scalarOids, err := doDoFetchScalarOids(session, oids)
		if err != nil {
			return nil, err
		}
		results = scalarOids
	}
	return results, nil
}

func doDoFetchScalarOids(session session.Session, oids []string) (*gosnmp.SnmpPacket, error) {
	log.Debugf("fetch scalar: request oids: %v", oids)
	results, err := session.Get(oids)
	if err != nil {
		log.Debugf("fetch scalar: error getting oids `%v`: %v", oids, err)
		return nil, fmt.Errorf("fetch scalar: error getting oids `%v`: %v", oids, err)
	}
	if log.ShouldLog(seelog.DebugLvl) {
		log.Debugf("fetch scalar: results: %s", gosnmplib.PacketAsString(results))
	}
	return results, nil
}
