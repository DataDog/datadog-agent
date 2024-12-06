// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package snmpparse

import "github.com/gosnmp/gosnmp"

const (
	// DefaultPort is the standard SNMP port
	DefaultPort = 161
	// DefaultCommunityString is the default on most v2 implementations
	DefaultCommunityString = "public"
)

// AuthOpts maps string names to gosnmp auth protocols
var AuthOpts = NewOptions(OptPairs[gosnmp.SnmpV3AuthProtocol]{
	{"", gosnmp.NoAuth},
	{"MD5", gosnmp.MD5},
	{"SHA", gosnmp.SHA},
	{"SHA224", gosnmp.SHA224},
	{"SHA256", gosnmp.SHA256},
	{"SHA384", gosnmp.SHA384},
	{"SHA512", gosnmp.SHA512},
})

// PrivOpts maps string names to gosnmp privacy protocols
var PrivOpts = NewOptions(OptPairs[gosnmp.SnmpV3PrivProtocol]{
	{"", gosnmp.NoPriv},
	{"DES", gosnmp.DES},
	{"AES", gosnmp.AES},
	{"AES192", gosnmp.AES192},
	{"AES192C", gosnmp.AES192C},
	{"AES256", gosnmp.AES256},
	{"AES256C", gosnmp.AES256C},
})

// VersionOpts maps string names to gosnmp versions
var VersionOpts = NewOptions(OptPairs[gosnmp.SnmpVersion]{
	{"1", gosnmp.Version1},
	{"2c", gosnmp.Version2c},
	{"3", gosnmp.Version3},
})

// LevelOpts maps string names to gosnmp auth levels
var LevelOpts = NewOptions(OptPairs[gosnmp.SnmpV3MsgFlags]{
	{"noAuthNoPriv", gosnmp.NoAuthNoPriv},
	{"authNoPriv", gosnmp.AuthNoPriv},
	{"authPriv", gosnmp.AuthPriv},
})
