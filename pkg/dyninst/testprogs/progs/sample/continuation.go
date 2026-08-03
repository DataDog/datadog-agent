// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

// This file contains test functions designed to produce events that exceed
// the 32KiB BPF scratch buffer, exercising the continuation (multi-fragment)
// event path.

// paddedData is a struct large enough that pointer-chasing many instances of it
// will exceed the 32KiB scratch buffer. Each instance is 512 bytes.
type paddedData struct {
	id   int
	data [63]uint64
}

// manyPointers holds 70 distinct pointer fields. When all are non-nil, the BPF
// pointer chaser will serialize each one as a separate data item:
//
//	70 pointers * (512 bytes data + 16 bytes header) ≈ 37KiB
//
// Combined with the event root, stack trace, and event header, the total
// exceeds the 32KiB scratch buffer and forces at least one continuation
// fragment.
type manyPointers struct {
	p00 *paddedData
	p01 *paddedData
	p02 *paddedData
	p03 *paddedData
	p04 *paddedData
	p05 *paddedData
	p06 *paddedData
	p07 *paddedData
	p08 *paddedData
	p09 *paddedData
	p10 *paddedData
	p11 *paddedData
	p12 *paddedData
	p13 *paddedData
	p14 *paddedData
	p15 *paddedData
	p16 *paddedData
	p17 *paddedData
	p18 *paddedData
	p19 *paddedData
	p20 *paddedData
	p21 *paddedData
	p22 *paddedData
	p23 *paddedData
	p24 *paddedData
	p25 *paddedData
	p26 *paddedData
	p27 *paddedData
	p28 *paddedData
	p29 *paddedData
	p30 *paddedData
	p31 *paddedData
	p32 *paddedData
	p33 *paddedData
	p34 *paddedData
	p35 *paddedData
	p36 *paddedData
	p37 *paddedData
	p38 *paddedData
	p39 *paddedData
	p40 *paddedData
	p41 *paddedData
	p42 *paddedData
	p43 *paddedData
	p44 *paddedData
	p45 *paddedData
	p46 *paddedData
	p47 *paddedData
	p48 *paddedData
	p49 *paddedData
	p50 *paddedData
	p51 *paddedData
	p52 *paddedData
	p53 *paddedData
	p54 *paddedData
	p55 *paddedData
	p56 *paddedData
	p57 *paddedData
	p58 *paddedData
	p59 *paddedData
	p60 *paddedData
	p61 *paddedData
	p62 *paddedData
	p63 *paddedData
	p64 *paddedData
	p65 *paddedData
	p66 *paddedData
	p67 *paddedData
	p68 *paddedData
	p69 *paddedData
}

//nolint:all
//go:noinline
func testManyPointers(m manyPointers) {}

func makePaddedData(id int) *paddedData {
	p := &paddedData{id: id}
	for i := range p.data {
		p.data[i] = uint64(id)*100 + uint64(i)
	}
	return p
}

//nolint:all
func executeContinuationFuncs() {
	m := manyPointers{
		p00: makePaddedData(0),
		p01: makePaddedData(1),
		p02: makePaddedData(2),
		p03: makePaddedData(3),
		p04: makePaddedData(4),
		p05: makePaddedData(5),
		p06: makePaddedData(6),
		p07: makePaddedData(7),
		p08: makePaddedData(8),
		p09: makePaddedData(9),
		p10: makePaddedData(10),
		p11: makePaddedData(11),
		p12: makePaddedData(12),
		p13: makePaddedData(13),
		p14: makePaddedData(14),
		p15: makePaddedData(15),
		p16: makePaddedData(16),
		p17: makePaddedData(17),
		p18: makePaddedData(18),
		p19: makePaddedData(19),
		p20: makePaddedData(20),
		p21: makePaddedData(21),
		p22: makePaddedData(22),
		p23: makePaddedData(23),
		p24: makePaddedData(24),
		p25: makePaddedData(25),
		p26: makePaddedData(26),
		p27: makePaddedData(27),
		p28: makePaddedData(28),
		p29: makePaddedData(29),
		p30: makePaddedData(30),
		p31: makePaddedData(31),
		p32: makePaddedData(32),
		p33: makePaddedData(33),
		p34: makePaddedData(34),
		p35: makePaddedData(35),
		p36: makePaddedData(36),
		p37: makePaddedData(37),
		p38: makePaddedData(38),
		p39: makePaddedData(39),
		p40: makePaddedData(40),
		p41: makePaddedData(41),
		p42: makePaddedData(42),
		p43: makePaddedData(43),
		p44: makePaddedData(44),
		p45: makePaddedData(45),
		p46: makePaddedData(46),
		p47: makePaddedData(47),
		p48: makePaddedData(48),
		p49: makePaddedData(49),
		p50: makePaddedData(50),
		p51: makePaddedData(51),
		p52: makePaddedData(52),
		p53: makePaddedData(53),
		p54: makePaddedData(54),
		p55: makePaddedData(55),
		p56: makePaddedData(56),
		p57: makePaddedData(57),
		p58: makePaddedData(58),
		p59: makePaddedData(59),
		p60: makePaddedData(60),
		p61: makePaddedData(61),
		p62: makePaddedData(62),
		p63: makePaddedData(63),
		p64: makePaddedData(64),
		p65: makePaddedData(65),
		p66: makePaddedData(66),
		p67: makePaddedData(67),
		p68: makePaddedData(68),
		p69: makePaddedData(69),
	}
	testManyPointers(m)
}
