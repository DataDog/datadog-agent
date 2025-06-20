// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//nolint:revive // TODO(NDM) Fix revive linter
package snmp

// DeviceReachableGetNextOid is used in getNext call to check if the device is reachable
// GETNEXT 1.0 should be able to fetch the first available SNMP OID.
// There is no need to handle top node other than iso(1) since it the only valid SNMP tree starting point.
// Other top nodes like ccitt(0) and joint(2) do not pertain to SNMP.
// Source: https://docstore.mik.ua/orelly/networking_2ndEd/snmp/ch02_03.htm
const DeviceReachableGetNextOid = "1.0"

// DeviceSysUptimeOid is the OID for the device system uptime
const DeviceSysUptimeOid = "1.3.6.1.2.1.1.3.0"

// DeviceSysNameOid is the OID for the device system name
const DeviceSysNameOid = "1.3.6.1.2.1.1.5.0"

// DeviceSysDescrOid is the OID for the device system description
const DeviceSysDescrOid = "1.3.6.1.2.1.1.1.0"

// DeviceSysObjectIDOid is the OID for the device system object ID
const DeviceSysObjectIDOid = "1.3.6.1.2.1.1.2.0"
