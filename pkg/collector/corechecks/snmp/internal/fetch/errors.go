package fetch

import (
	"fmt"
)

type fetchError struct {
	oidType oidType
	oids    []string
	op      snmpOperation
	err     error
}

type oidType string

const (
	scalarOid oidType = "scalar"
	columnOid oidType = "column"
)

type snmpOperation string

const (
	snmpGet     snmpOperation = "Get"
	snmpGetBulk snmpOperation = "GetBulk"
	snmpGetNext snmpOperation = "GetNext"
)

func (e *fetchError) Error() string {
	return fmt.Sprintf("fetch %s: error getting oids `%v` using %s: %v", e.oidType, e.oids, e.op, e.err)
}

func (e *fetchError) Unwrap() error {
	return e.err
}
