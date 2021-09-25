package checkconfig

// OidConfig holds configs for OIDs to fetch
type OidConfig struct {
	// ScalarOids are all scalar oids to fetch
	ScalarOids []string
	// ColumnOids are all column oids to fetch
	ColumnOids []string
}

func (oc *OidConfig) addScalarOids(oidsToAdd []string) {
	oc.ScalarOids = oc.addOidsIfNotPresent(oc.ScalarOids, oidsToAdd)
}

func (oc *OidConfig) addColumnOids(oidsToAdd []string) {
	oc.ColumnOids = oc.addOidsIfNotPresent(oc.ColumnOids, oidsToAdd)
}

func (oc *OidConfig) addOidsIfNotPresent(configOids []string, oidsToAdd []string) []string {
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
