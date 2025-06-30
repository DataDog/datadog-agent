// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ir

// Location is the location of a parameter or variable in the subprogram.
type Location struct {
	// PCRange is the range of PC values that will be probed.
	Range PCRange
	// The locations of the pieces of the parameter or variable.
	Pieces []Piece
}

// Piece represents part of data.
type Piece struct {
	Size uint32
	// Op may be nil, if specific piece is unavailable.
	Op PieceOp
}

// PieceOp represents a way to resolve variable value.
type PieceOp interface {
	pieceOp() // marker
}

func (p Piece) pieceOp() {}

// Addr represents DW_OP_addr.
type Addr struct {
	Addr uint64
}

func (a Addr) pieceOp() {}

// Cfa represents DW_OP_fbreg and DW_OP_call_frame_cfa.
type Cfa struct {
	CfaOffset int32
}

func (c Cfa) pieceOp() {}

// Register represents DW_OP_reg* and DW_OP_breg*.
type Register struct {
	RegNo uint8
	Shift int64
}

func (r Register) pieceOp() {}
