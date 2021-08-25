package metadata

// Scalar OIDs
var (
	// SysNameOID is the OID for SysName
	SysNameOID = "1.3.6.1.2.1.1.5.0"
	// SysDescrOID is the OID for SysDescr
	SysDescrOID = "1.3.6.1.2.1.1.1.0"
	// SysObjectIDOID is the OID for SysObjectID
	SysObjectIDOID = "1.3.6.1.2.1.1.2.0"
)

// ScalarOIDs is the list of all scalar OIDs needed for device metadata
var ScalarOIDs = []string{
	SysNameOID,
	SysDescrOID,
	SysObjectIDOID,
}

var (
	// IfNameOID is the OID for IfName
	IfNameOID = "1.3.6.1.2.1.31.1.1.1.1"
	// IfAliasOID is the OID for IfAlias
	IfAliasOID = "1.3.6.1.2.1.31.1.1.1.18"
	// IfDescrOID is the OID for IfDescr
	IfDescrOID = "1.3.6.1.2.1.2.2.1.2"
	// IfPhysAddressOID is the OID for IfPhysAddress
	IfPhysAddressOID = "1.3.6.1.2.1.2.2.1.6"
	// IfAdminStatusOID is the OID for IfAdminStatus
	IfAdminStatusOID = "1.3.6.1.2.1.2.2.1.7"
	// IfOperStatusOID is the OID for IfOperStatus
	IfOperStatusOID = "1.3.6.1.2.1.2.2.1.8"
)

// ColumnOIDs is the list of all column OIDs needed for device metadata
var ColumnOIDs = []string{
	IfNameOID,
	IfAliasOID,
	IfDescrOID,
	IfPhysAddressOID,
	IfAdminStatusOID,
	IfOperStatusOID,
}
