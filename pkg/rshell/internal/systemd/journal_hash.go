// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"encoding/binary"
	"math/bits"
)

// journalJenkinsHash64 implements lookup3 hashlittle2 with zero seeds, which
// older journal files use for DATA and FIELD objects.
func journalJenkinsHash64(data []byte) uint64 {
	a := uint32(0xdeadbeef) + uint32(len(data))
	b := a
	c := a

	for len(data) > 12 {
		a += binary.LittleEndian.Uint32(data[0:4])
		b += binary.LittleEndian.Uint32(data[4:8])
		c += binary.LittleEndian.Uint32(data[8:12])
		a, b, c = journalJenkinsMix(a, b, c)
		data = data[12:]
	}
	if len(data) == 0 {
		return uint64(c)<<32 | uint64(b)
	}
	for index, value := range data {
		shift := uint(index%4) * 8
		switch index / 4 {
		case 0:
			a += uint32(value) << shift
		case 1:
			b += uint32(value) << shift
		case 2:
			c += uint32(value) << shift
		}
	}
	a, b, c = journalJenkinsFinal(a, b, c)
	return uint64(c)<<32 | uint64(b)
}

func journalJenkinsMix(a, b, c uint32) (uint32, uint32, uint32) {
	a -= c
	a ^= bits.RotateLeft32(c, 4)
	c += b
	b -= a
	b ^= bits.RotateLeft32(a, 6)
	a += c
	c -= b
	c ^= bits.RotateLeft32(b, 8)
	b += a
	a -= c
	a ^= bits.RotateLeft32(c, 16)
	c += b
	b -= a
	b ^= bits.RotateLeft32(a, 19)
	a += c
	c -= b
	c ^= bits.RotateLeft32(b, 4)
	b += a
	return a, b, c
}

func journalJenkinsFinal(a, b, c uint32) (uint32, uint32, uint32) {
	c ^= b
	c -= bits.RotateLeft32(b, 14)
	a ^= c
	a -= bits.RotateLeft32(c, 11)
	b ^= a
	b -= bits.RotateLeft32(a, 25)
	c ^= b
	c -= bits.RotateLeft32(b, 16)
	a ^= c
	a -= bits.RotateLeft32(c, 4)
	b ^= a
	b -= bits.RotateLeft32(a, 14)
	c ^= b
	c -= bits.RotateLeft32(b, 24)
	return a, b, c
}

// journalSipHash24 implements SipHash-2-4 with the journal file ID as its
// 128-bit key, which modern journal files use for DATA and FIELD objects.
func journalSipHash24(data []byte, key journalID) uint64 {
	k0 := binary.LittleEndian.Uint64(key[0:8])
	k1 := binary.LittleEndian.Uint64(key[8:16])
	v0 := uint64(0x736f6d6570736575) ^ k0
	v1 := uint64(0x646f72616e646f6d) ^ k1
	v2 := uint64(0x6c7967656e657261) ^ k0
	v3 := uint64(0x7465646279746573) ^ k1
	length := len(data)

	for len(data) >= 8 {
		message := binary.LittleEndian.Uint64(data[:8])
		v3 ^= message
		journalSipRound(&v0, &v1, &v2, &v3)
		journalSipRound(&v0, &v1, &v2, &v3)
		v0 ^= message
		data = data[8:]
	}

	last := uint64(length) << 56
	for index, value := range data {
		last |= uint64(value) << (uint(index) * 8)
	}
	v3 ^= last
	journalSipRound(&v0, &v1, &v2, &v3)
	journalSipRound(&v0, &v1, &v2, &v3)
	v0 ^= last
	v2 ^= 0xff
	journalSipRound(&v0, &v1, &v2, &v3)
	journalSipRound(&v0, &v1, &v2, &v3)
	journalSipRound(&v0, &v1, &v2, &v3)
	journalSipRound(&v0, &v1, &v2, &v3)
	return v0 ^ v1 ^ v2 ^ v3
}

func journalSipRound(v0, v1, v2, v3 *uint64) {
	*v0 += *v1
	*v1 = bits.RotateLeft64(*v1, 13)
	*v1 ^= *v0
	*v0 = bits.RotateLeft64(*v0, 32)
	*v2 += *v3
	*v3 = bits.RotateLeft64(*v3, 16)
	*v3 ^= *v2
	*v0 += *v3
	*v3 = bits.RotateLeft64(*v3, 21)
	*v3 ^= *v0
	*v2 += *v1
	*v1 = bits.RotateLeft64(*v1, 17)
	*v1 ^= *v2
	*v2 = bits.RotateLeft64(*v2, 32)
}
