package device_metadata

// Scalar OIDs
var SysNameOID = "1.3.6.1.2.1.1.5.0"
var SysDescrOID = "1.3.6.1.2.1.1.1.0"
var SysObjectIDOID = "1.3.6.1.2.1.1.2.0"

var MetadataScalarOIDs = []string{
	SysNameOID,
	SysDescrOID,
	SysObjectIDOID,
}

// Column OIDs
var IfNameOID = "1.3.6.1.2.1.31.1.1.1.1"
var IfAliasOID = "1.3.6.1.2.1.31.1.1.1.18"
var IfDescrOID = "1.3.6.1.2.1.2.2.1.2"
var IfPhysAddressOID = "1.3.6.1.2.1.2.2.1.6"
var IfAdminStatusOID = "1.3.6.1.2.1.2.2.1.7"
var IfOperStatusOID = "1.3.6.1.2.1.2.2.1.8"

var MetadataColumnOIDs = []string{
	IfNameOID,
	IfAliasOID,
	IfDescrOID,
	IfPhysAddressOID,
	IfAdminStatusOID,
	IfOperStatusOID,
}
