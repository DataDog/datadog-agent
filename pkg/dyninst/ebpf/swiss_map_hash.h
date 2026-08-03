// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#ifndef __SWISS_MAP_HASH_H__
#define __SWISS_MAP_HASH_H__

// Swiss map hash utilities for BPF. The actual hash computation is driven by
// SM_OP_SWISS_MAP_SETUP (wyhash path) and SM_OP_SWISS_MAP_AESENC +
// SM_OP_SWISS_MAP_HASH_FINISH (AES path). This file provides constants,
// the AES S-box, and helper functions used by those opcodes.
//
// See pkg/dyninst/irgen/go_swiss_maps.md for the algorithm specifications.

#include "types.h"

#define SWISS_MAP_MAX_STR_KEY_LEN 512

// Hash phase enum for the AES multi-step hash computation.
// SETUP initializes the phase; HASH_FINISH handles transitions.
//
// The Go runtime's aeshashbody has length tiers:
//   0:       1 lane, extra self-keyed round
//   1-16:    1 lane  (X0)
//   17-32:   2 lanes (X0, X1)
//   33-64:   4 lanes (X0-X3)
//   65-128:  8 lanes (X0-X7)
//   129+:    8 lanes, loop over 128-byte blocks
//
// Each tier prepares N seeds from unscrambled XOR aeskeysched[16*i], then
// loads N data chunks, XORs with seeds, and does 3 self-keyed AESENC rounds.
// The results are XOR-folded down to one 16-byte value.
enum swiss_map_hash_phase {
  // Initial phase: key data has been copied, hash computation not yet started.
  SWISS_HASH_PHASE_INIT = 0,
  // memhash32/64: extract hash after 3 keyed AESENC rounds.
  SWISS_HASH_PHASE_DIRECT_DONE,
  // After initial 1-round self-keyed seed scramble.
  SWISS_HASH_PHASE_SEED_SCRAMBLE_DONE,
  // len==0 case: after extra self-keyed round.
  SWISS_HASH_PHASE_FINAL_EXTRA,

  // --- 1-16 byte path ---
  SWISS_HASH_PHASE_SINGLE_LANE_DONE,

  // --- 17-32 byte path (2 lanes) ---
  SWISS_HASH_PHASE_LANE0_DONE,  // lane0 rounds done, need seed1 + lane1
  SWISS_HASH_PHASE_SEED1_DONE,  // seed1 scrambled, do data1 rounds
  SWISS_HASH_PHASE_LANE1_DONE,  // both lanes done, XOR-fold

  // --- 33-64 byte path (4 lanes) ---
  // Uses multi-lane seed prep and data rounds.
  // After seed scramble: prepare seeds 1-3 one at a time via AESENC.
  SWISS_HASH_PHASE_MULTI_SEED_PREP,  // preparing next seed
  SWISS_HASH_PHASE_MULTI_DATA_ROUNDS, // doing 3 self-keyed rounds on current lane
  SWISS_HASH_PHASE_MULTI_DONE,       // all lanes done, XOR-fold and finalize

  // --- 129+ byte path (8 lanes, block loop) ---
  // After all 8 seeds prepared and initial data loaded:
  SWISS_HASH_PHASE_BLOCK_SELF_ROUNDS, // 1 self-keyed round on all 8 lanes
  SWISS_HASH_PHASE_BLOCK_DATA_ROUND,  // 1 data-keyed round on current lane
  SWISS_HASH_PHASE_BLOCK_ADVANCE,     // advance to next block or finish
  SWISS_HASH_PHASE_BLOCK_FINAL_ROUNDS, // 3 final self-keyed rounds on all lanes
};

// Number of AES lanes (matches Go runtime's use of XMM0-XMM7).
#define SWISS_MAP_MAX_AES_LANES 8

// Lane/seed access helpers. Mask the BYTE OFFSET with & 0x70 so the
// BPF verifier sees range [0, 112] regardless of compiler reordering.
// 0x70 = 7 * 16, matching lanes[0..7] each 16 bytes.
static __always_inline uint8_t* _lane_ptr(uint8_t lanes[][16], uint8_t idx) {
  return (uint8_t*)((char*)lanes + (((uint32_t)idx * 16) & 0x70));
}
static __always_inline uint8_t* _seed_ptr(uint8_t seeds[][16], uint8_t idx) {
  return (uint8_t*)((char*)seeds + (((uint32_t)idx * 16) & 0x70));
}

#define LANE_WRITE(hs, idx, src) copy16(_lane_ptr((hs).lanes, idx), src)
#define LANE_READ(hs, idx, dst)  copy16(dst, _lane_ptr((hs).lanes, idx))
#define LANE_XOR(hs, idx, src)   xor16(_lane_ptr((hs).lanes, idx), src)
#define SEED_WRITE(hs, idx, src) copy16(_seed_ptr((hs).seeds, idx), src)
#define SEED_READ(hs, idx, dst)  copy16(dst, _seed_ptr((hs).seeds, idx))

#define LANE_IDX(x) ((uint32_t)(x) & 7u)

// ---------------------------------------------------------------------------
// AES S-box (FIPS 197, Section 5.1.1)
// ---------------------------------------------------------------------------

// clang-format off
static const uint8_t aes_sbox[256] = {
  0x63,0x7c,0x77,0x7b,0xf2,0x6b,0x6f,0xc5,0x30,0x01,0x67,0x2b,0xfe,0xd7,0xab,0x76,
  0xca,0x82,0xc9,0x7d,0xfa,0x59,0x47,0xf0,0xad,0xd4,0xa2,0xaf,0x9c,0xa4,0x72,0xc0,
  0xb7,0xfd,0x93,0x26,0x36,0x3f,0xf7,0xcc,0x34,0xa5,0xe5,0xf1,0x71,0xd8,0x31,0x15,
  0x04,0xc7,0x23,0xc3,0x18,0x96,0x05,0x9a,0x07,0x12,0x80,0xe2,0xeb,0x27,0xb2,0x75,
  0x09,0x83,0x2c,0x1a,0x1b,0x6e,0x5a,0xa0,0x52,0x3b,0xd6,0xb3,0x29,0xe3,0x2f,0x84,
  0x53,0xd1,0x00,0xed,0x20,0xfc,0xb1,0x5b,0x6a,0xcb,0xbe,0x39,0x4a,0x4c,0x58,0xcf,
  0xd0,0xef,0xaa,0xfb,0x43,0x4d,0x33,0x85,0x45,0xf9,0x02,0x7f,0x50,0x3c,0x9f,0xa8,
  0x51,0xa3,0x40,0x8f,0x92,0x9d,0x38,0xf5,0xbc,0xb6,0xda,0x21,0x10,0xff,0xf3,0xd2,
  0xcd,0x0c,0x13,0xec,0x5f,0x97,0x44,0x17,0xc4,0xa7,0x7e,0x3d,0x64,0x5d,0x19,0x73,
  0x60,0x81,0x4f,0xdc,0x22,0x2a,0x90,0x88,0x46,0xee,0xb8,0x14,0xde,0x5e,0x0b,0xdb,
  0xe0,0x32,0x3a,0x0a,0x49,0x06,0x24,0x5c,0xc2,0xd3,0xac,0x62,0x91,0x95,0xe4,0x79,
  0xe7,0xc8,0x37,0x6d,0x8d,0xd5,0x4e,0xa9,0x6c,0x56,0xf4,0xea,0x65,0x7a,0xae,0x08,
  0xba,0x78,0x25,0x2e,0x1c,0xa6,0xb4,0xc6,0xe8,0xdd,0x74,0x1f,0x4b,0xbd,0x8b,0x8a,
  0x70,0x3e,0xb5,0x66,0x48,0x03,0xf6,0x0e,0x61,0x35,0x57,0xb9,0x86,0xc1,0x1d,0x9e,
  0xe1,0xf8,0x98,0x11,0x69,0xd9,0x8e,0x94,0x9b,0x1e,0x87,0xe9,0xce,0x55,0x28,0xdf,
  0x8c,0xa1,0x89,0x0d,0xbf,0xe6,0x42,0x68,0x41,0x99,0x2d,0x0f,0xb0,0x54,0xbb,0x16,
};
// clang-format on

// ---------------------------------------------------------------------------
// AES helpers
// ---------------------------------------------------------------------------

static __always_inline uint8_t xtime(uint8_t b) {
  return (uint8_t)((b << 1) ^ (((b >> 7) & 1) * 0x1b));
}

static __always_inline void xor16(uint8_t dst[16], const uint8_t src[16]) {
  *(uint64_t*)&dst[0] ^= *(const uint64_t*)&src[0];
  *(uint64_t*)&dst[8] ^= *(const uint64_t*)&src[8];
}

static __always_inline void copy16(uint8_t dst[16], const uint8_t src[16]) {
  *(uint64_t*)&dst[0] = *(const uint64_t*)&src[0];
  *(uint64_t*)&dst[8] = *(const uint64_t*)&src[8];
}

static __always_inline void zero16(uint8_t dst[16]) {
  *(uint64_t*)&dst[0] = 0;
  *(uint64_t*)&dst[8] = 0;
}

#endif // __SWISS_MAP_HASH_H__
