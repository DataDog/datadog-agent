package snmp

type oidConfig struct {
	scalarOids []string
	columnOids []string
}

func (oc *oidConfig) hasOids() bool {
	return len(oc.columnOids) != 0 || len(oc.scalarOids) != 0
}

func (oc *oidConfig) addScalarOids(oidsToAdd []string) {
	oc.scalarOids = oc.addOidsIfNotPresent(oc.scalarOids, oidsToAdd)
}

func (oc *oidConfig) addColumnOids(oidsToAdd []string) {
	oc.columnOids = oc.addOidsIfNotPresent(oc.columnOids, oidsToAdd)
}

func (oc *oidConfig) addOidsIfNotPresent(configOids []string, oidsToAdd []string) []string {
	for _, oidToAdd := range oidsToAdd {
		isAlreadyPresent := false
		for _, oid := range configOids {
			if oid == oidToAdd {
				isAlreadyPresent = true
				break
			}
		}
		if isAlreadyPresent {
			continue
		}
		configOids = append(configOids, oidToAdd)
	}
	return configOids
}
