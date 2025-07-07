// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package compiler supports compiling probe ir into a stack machine program.
package compiler

//go:generate go run golang.org/x/tools/cmd/stringer -type=Opcode -trimprefix=Opcode

//revive:disable:exported
type Opcode uint8

const (
	OpcodeInvalid Opcode = iota
	OpcodeCall
	OpcodeReturn
	OpcodeIllegal
	OpcodeIncrementOutputOffset
	OpcodeExprPrepare
	OpcodeExprSave
	OpcodeExprDereferenceCfa
	OpcodeExprReadRegister
	OpcodeExprDereferencePtr
	OpcodeProcessPointer
	OpcodeProcessSlice
	OpcodeProcessArrayDataPrep
	OpcodeProcessSliceDataPrep
	OpcodeProcessSliceDataRepeat
	OpcodeProcessString
	OpcodeProcessGoEmptyInterface
	OpcodeProcessGoInterface
	OpcodeProcessGoHmap
	OpcodeProcessGoSwissMap
	OpcodeProcessGoSwissMapGroups
	OpcodeChasePointers
	OpcodePrepareEventRoot
)

//revive:enable:exported
