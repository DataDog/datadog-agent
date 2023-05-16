// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probes

const (
	// SecurityAgentUID is the UID used for all the runtime security module probes
	SecurityAgentUID = "security"
)

const (
	// ERPCResolveParentDentryKey is the key to the eRPC dentry parent resolver tail call program
	ERPCResolveParentDentryKey uint32 = iota
	// ERPCResolvePathWatermarkReaderKey is the key to the eRPC path watermark reader tail call program
	ERPCResolvePathWatermarkReaderKey
	// ERPCResolvePathSegmentkReaderKey is the key to the eRPC path segment reader tail call program
	ERPCResolvePathSegmentkReaderKey
)

const (
	// TCDNSRequestKey is the key to DNS request program
	TCDNSRequestKey uint32 = iota + 1
	// TCDNSRequestParserKey is the key to DNS request parser program
	TCDNSRequestParserKey
)

const (
	// ExecGetEnvsOffsetKey is the key to the program that computes the environment variables offset
	ExecGetEnvsOffsetKey uint32 = iota
	// ExecParseArgsEnvsSplitKey is the key to the program that splits the parsing of arguments and environment variables between tailcalls
	ExecParseArgsEnvsSplitKey
	// ExecParseArgsEnvsKey is the key to the program that parses arguments and then environment variables
	ExecParseArgsEnvsKey
)
