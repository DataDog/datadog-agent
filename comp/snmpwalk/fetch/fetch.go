package fetch

import (
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"
	"github.com/gosnmp/gosnmp"
	"strings"
	"time"
)

type fetchStrategyType string

const (
	useGetBulk fetchStrategyType = "GETBULK"
	useGetNext fetchStrategyType = "GETNEXT"
)

func FetchAllFirstRowOIDsVariables(session gosnmp.GoSNMP, fetchStrategy fetchStrategyType) []gosnmp.SnmpPDU {
	var savedPDUs []gosnmp.SnmpPDU
	curRequestOid := "1.0"
	alreadySeenOIDs := make(map[string]bool)
	counter := 0

	throttler := time.NewTicker(100 * time.Millisecond)
	defer throttler.Stop()

	for {
		counter += 1

		if alreadySeenOIDs[curRequestOid] {
			// breaking on already seen OIDs prevent infinite loop if the device mis behave by responding with non-sequential OIDs when called with GETNEXT
			log.Debug("error: received non sequential OIDs")
			break
		}
		alreadySeenOIDs[curRequestOid] = true

		var results *gosnmp.SnmpPacket
		if session.Version == gosnmp.Version1 || fetchStrategy == useGetNext {
			// snmp v1 doesn't support GetBulk
			log.Infof("GetNext request (%d): %s", counter, curRequestOid)
			res, err := session.GetNext([]string{curRequestOid})
			//log.Infof("GetNext results: %+v", results)
			if err != nil {
				log.Debugf("GetNext error: %s", err)
				break
			}
			results = res
		} else {
			log.Infof("GetBulk request (%d): %s", counter, curRequestOid)
			getBulkResults, err := session.GetBulk([]string{curRequestOid}, 0, 20)
			if err != nil {
				log.Debugf("fetch column: failed getting oids `%v` using GetBulk: %s", curRequestOid, err)
			}
			log.Infof("GetBulk results, num of variables: %d", len(getBulkResults.Variables))
			results = getBulkResults
			if log.ShouldLog(seelog.DebugLvl) {
				log.Debugf("fetch column: GetBulk results: %v", gosnmplib.PacketAsString(results))
			}
		}

		// throttle
		select {
		case <-throttler.C:
		}

		//if len(results.Variables) != 1 {
		//	log.Debugf("Expect 1 variable, but got %d: %+v", len(results.Variables), results.Variables)
		//	break
		//}
		for _, variable := range results.Variables {
			if variable.Type == gosnmp.EndOfContents || variable.Type == gosnmp.EndOfMibView {
				log.Debug("No more OIDs to fetch")
				break
			}
			oid := strings.TrimLeft(variable.Name, ".")
			log.Infof("Variable oid %s", oid)

			if strings.HasSuffix(oid, ".0") { // check if it's a scalar OID
				curRequestOid = oid
			} else {
				nextColumn := GetNextColumnOidNaive(oid)
				curRequestOid = nextColumn
				//if err != nil {
				//	log.Debugf("Invalid column oid %s: %s", oid, err)
				//	curRequestOid = oid // fallback on continuing by using the response oid as next oid to request
				//} else {
				//	curRequestOid = nextColumn
				//}
			}
			//alreadySeenOIDs[curRequestOid] = true

			savedPDUs = append(savedPDUs, variable)
		}
	}
	return savedPDUs
}
