package analyzer

import (
	"fmt"

	"github.com/gosnmp/gosnmp"
)

const _cached_sys_obj_id = ".1.3.6.1.2.1.1.2"

func findSysOID(pdus []gosnmp.SnmpPDU) string {
	for _, pdu := range pdus {
		if pdu.Name == _cached_sys_obj_id {
			return fmt.Sprintf("%v", pdu.Value)
		}
	}
	return ""
}
