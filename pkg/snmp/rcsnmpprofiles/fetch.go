package rcsnmpprofiles

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/gosnmp/gosnmp"
	"strings"
)

func FetchAllFirstRowOIDsVariables(session gosnmp.GoSNMP) []gosnmp.SnmpPDU {
	var savedPDUs []gosnmp.SnmpPDU
	curRequestOid := "1.0"
	alreadySeenOIDs := make(map[string]bool)

	for {
		//log.Debugf("GetNext request: %s", curRequestOid)
		results, err := session.GetNext([]string{curRequestOid})
		//log.Debugf("GetNext results: %+v", results)
		if err != nil {
			log.Debugf("GetNext error: %s", err)
			break
		}
		if len(results.Variables) != 1 {
			log.Debugf("Expect 1 variable, but got %d: %+v", len(results.Variables), results.Variables)
			break
		}
		variable := results.Variables[0]
		if variable.Type == gosnmp.EndOfContents || variable.Type == gosnmp.EndOfMibView {
			log.Debug("No more OIDs to fetch")
			break
		}
		oid := strings.TrimLeft(variable.Name, ".")

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

		if alreadySeenOIDs[curRequestOid] {
			// breaking on already seen OIDs prevent infinite loop if the device mis behave by responding with non-sequential OIDs when called with GETNEXT
			log.Debug("error: received non sequential OIDs")
			break
		}
		alreadySeenOIDs[curRequestOid] = true

		savedPDUs = append(savedPDUs, variable)
	}
	return savedPDUs
}
