// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fetch

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
