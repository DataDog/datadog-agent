package snmp

type oidConfig struct {
	scalarOids []string
	columnOids []string
}

func (oc *oidConfig) hasOids() bool {
	return len(oc.columnOids) != 0 || len(oc.scalarOids) != 0
}
