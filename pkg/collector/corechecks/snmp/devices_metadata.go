package snmp

// Scalar OIDs
var sysNameOID = "1.3.6.1.2.1.1.5.0"
var sysDescrOID = "1.3.6.1.2.1.1.1.0"
var sysObjectIDOID = "1.3.6.1.2.1.1.2.0"

// TODO: TEST ME
var metadataScalarOIDs = []string{
	sysNameOID,
	sysDescrOID,
	sysObjectIDOID,
}

// Column OIDs
var ifNameOID = "1.3.6.1.2.1.31.1.1.1.1"
var ifAliasOID = "1.3.6.1.2.1.31.1.1.1.18"
var ifDescrOID = "1.3.6.1.2.1.2.2.1.2"
var ifPhysAddressOID = "1.3.6.1.2.1.2.2.1.6"
var ifAdminStatusOID = "1.3.6.1.2.1.2.2.1.7"
var ifOperStatusOID = "1.3.6.1.2.1.2.2.1.8"

// TODO: TEST ME
var metadataColumnOIDs = []string{
	ifNameOID,
	ifAliasOID,
	ifDescrOID,
	ifPhysAddressOID,
	ifAdminStatusOID,
	ifOperStatusOID,
}
