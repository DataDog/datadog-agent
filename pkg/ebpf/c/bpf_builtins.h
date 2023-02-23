/* SPDX-License-Identifier: (GPL-2.0-only OR BSD-2-Clause) */
/* Copyright Authors of Cilium */

#ifndef __BPF_BUILTINS__
#define __BPF_BUILTINS__

#include "compiler.h"

/* Memory iterators used below. */
#define __it_bwd(x, op) (x -= sizeof(__u##op))
#define __it_fwd(x, op) (x += sizeof(__u##op))

/* Memory operators used below. */
#define __it_set(a, op) (*(__u##op *)__it_bwd(a, op)) = 0
#define __it_xor(a, b, r, op) r |= (*(__u##op *)__it_bwd(a, op)) ^ (*(__u##op *)__it_bwd(b, op))
#define __it_mob(a, b, op) (*(__u##op *)__it_bwd(a, op)) = (*(__u##op *)__it_bwd(b, op))
#define __it_mof(a, b, op)				\
	do {						\
		*(__u##op *)a = *(__u##op *)b;		\
		__it_fwd(a, op); __it_fwd(b, op);	\
	} while (0)

static __always_inline __maybe_unused void
__bpf_memset_builtin(void *d, __u8 c, __u64 len)
{
	/* Everything non-zero or non-const (currently unsupported) as c
	 * gets handled here.
	 */
	__builtin_memset(d, c, len);
}

static __always_inline void __bpf_memzero(void *d, __u64 len)
{
#if __clang_major__ >= 10
	if (!__builtin_constant_p(len))
		__throw_build_bug();

	d += len;

    if (len > 1 && len % 2 == 1) {
        __it_set(d, 8);
    	len -= 1;
    }

    switch (len) {
    case 512:          __it_set(d, 64);
    case 504: jmp_504: __it_set(d, 64);
    case 496: jmp_496: __it_set(d, 64);
    case 488: jmp_488: __it_set(d, 64);
    case 480: jmp_480: __it_set(d, 64);
    case 472: jmp_472: __it_set(d, 64);
    case 464: jmp_464: __it_set(d, 64);
    case 456: jmp_456: __it_set(d, 64);
    case 448: jmp_448: __it_set(d, 64);
    case 440: jmp_440: __it_set(d, 64);
    case 432: jmp_432: __it_set(d, 64);
    case 424: jmp_424: __it_set(d, 64);
    case 416: jmp_416: __it_set(d, 64);
    case 408: jmp_408: __it_set(d, 64);
    case 400: jmp_400: __it_set(d, 64);
    case 392: jmp_392: __it_set(d, 64);
    case 384: jmp_384: __it_set(d, 64);
    case 376: jmp_376: __it_set(d, 64);
    case 368: jmp_368: __it_set(d, 64);
    case 360: jmp_360: __it_set(d, 64);
    case 352: jmp_352: __it_set(d, 64);
    case 344: jmp_344: __it_set(d, 64);
    case 336: jmp_336: __it_set(d, 64);
    case 328: jmp_328: __it_set(d, 64);
    case 320: jmp_320: __it_set(d, 64);
    case 312: jmp_312: __it_set(d, 64);
    case 304: jmp_304: __it_set(d, 64);
    case 296: jmp_296: __it_set(d, 64);
    case 288: jmp_288: __it_set(d, 64);
    case 280: jmp_280: __it_set(d, 64);
    case 272: jmp_272: __it_set(d, 64);
    case 264: jmp_264: __it_set(d, 64);
    case 256: jmp_256: __it_set(d, 64);
    case 248: jmp_248: __it_set(d, 64);
    case 240: jmp_240: __it_set(d, 64);
    case 232: jmp_232: __it_set(d, 64);
    case 224: jmp_224: __it_set(d, 64);
    case 216: jmp_216: __it_set(d, 64);
    case 208: jmp_208: __it_set(d, 64);
    case 200: jmp_200: __it_set(d, 64);
    case 192: jmp_192: __it_set(d, 64);
    case 184: jmp_184: __it_set(d, 64);
    case 176: jmp_176: __it_set(d, 64);
    case 168: jmp_168: __it_set(d, 64);
    case 160: jmp_160: __it_set(d, 64);
    case 152: jmp_152: __it_set(d, 64);
    case 144: jmp_144: __it_set(d, 64);
    case 136: jmp_136: __it_set(d, 64);
    case 128: jmp_128: __it_set(d, 64);
    case 120: jmp_120: __it_set(d, 64);
    case 112: jmp_112: __it_set(d, 64);
    case 104: jmp_104: __it_set(d, 64);
    case 96: jmp_96: __it_set(d, 64);
    case 88: jmp_88: __it_set(d, 64);
    case 80: jmp_80: __it_set(d, 64);
    case 72: jmp_72: __it_set(d, 64);
    case 64: jmp_64: __it_set(d, 64);
    case 56: jmp_56: __it_set(d, 64);
    case 48: jmp_48: __it_set(d, 64);
    case 40: jmp_40: __it_set(d, 64);
    case 32: jmp_32: __it_set(d, 64);
    case 24: jmp_24: __it_set(d, 64);
    case 16: jmp_16: __it_set(d, 64);
    case 8: jmp_8: __it_set(d, 64); break;
    
    case 510: __it_set(d, 16); __it_set(d, 32); goto jmp_504;
    case 502: __it_set(d, 16); __it_set(d, 32); goto jmp_496;
    case 494: __it_set(d, 16); __it_set(d, 32); goto jmp_488;
    case 486: __it_set(d, 16); __it_set(d, 32); goto jmp_480;
    case 478: __it_set(d, 16); __it_set(d, 32); goto jmp_472;
    case 470: __it_set(d, 16); __it_set(d, 32); goto jmp_464;
    case 462: __it_set(d, 16); __it_set(d, 32); goto jmp_456;
    case 454: __it_set(d, 16); __it_set(d, 32); goto jmp_448;
    case 446: __it_set(d, 16); __it_set(d, 32); goto jmp_440;
    case 438: __it_set(d, 16); __it_set(d, 32); goto jmp_432;
    case 430: __it_set(d, 16); __it_set(d, 32); goto jmp_424;
    case 422: __it_set(d, 16); __it_set(d, 32); goto jmp_416;
    case 414: __it_set(d, 16); __it_set(d, 32); goto jmp_408;
    case 406: __it_set(d, 16); __it_set(d, 32); goto jmp_400;
    case 398: __it_set(d, 16); __it_set(d, 32); goto jmp_392;
    case 390: __it_set(d, 16); __it_set(d, 32); goto jmp_384;
    case 382: __it_set(d, 16); __it_set(d, 32); goto jmp_376;
    case 374: __it_set(d, 16); __it_set(d, 32); goto jmp_368;
    case 366: __it_set(d, 16); __it_set(d, 32); goto jmp_360;
    case 358: __it_set(d, 16); __it_set(d, 32); goto jmp_352;
    case 350: __it_set(d, 16); __it_set(d, 32); goto jmp_344;
    case 342: __it_set(d, 16); __it_set(d, 32); goto jmp_336;
    case 334: __it_set(d, 16); __it_set(d, 32); goto jmp_328;
    case 326: __it_set(d, 16); __it_set(d, 32); goto jmp_320;
    case 318: __it_set(d, 16); __it_set(d, 32); goto jmp_312;
    case 310: __it_set(d, 16); __it_set(d, 32); goto jmp_304;
    case 302: __it_set(d, 16); __it_set(d, 32); goto jmp_296;
    case 294: __it_set(d, 16); __it_set(d, 32); goto jmp_288;
    case 286: __it_set(d, 16); __it_set(d, 32); goto jmp_280;
    case 278: __it_set(d, 16); __it_set(d, 32); goto jmp_272;
    case 270: __it_set(d, 16); __it_set(d, 32); goto jmp_264;
    case 262: __it_set(d, 16); __it_set(d, 32); goto jmp_256;
    case 254: __it_set(d, 16); __it_set(d, 32); goto jmp_248;
    case 246: __it_set(d, 16); __it_set(d, 32); goto jmp_240;
    case 238: __it_set(d, 16); __it_set(d, 32); goto jmp_232;
    case 230: __it_set(d, 16); __it_set(d, 32); goto jmp_224;
    case 222: __it_set(d, 16); __it_set(d, 32); goto jmp_216;
    case 214: __it_set(d, 16); __it_set(d, 32); goto jmp_208;
    case 206: __it_set(d, 16); __it_set(d, 32); goto jmp_200;
    case 198: __it_set(d, 16); __it_set(d, 32); goto jmp_192;
    case 190: __it_set(d, 16); __it_set(d, 32); goto jmp_184;
    case 182: __it_set(d, 16); __it_set(d, 32); goto jmp_176;
    case 174: __it_set(d, 16); __it_set(d, 32); goto jmp_168;
    case 166: __it_set(d, 16); __it_set(d, 32); goto jmp_160;
    case 158: __it_set(d, 16); __it_set(d, 32); goto jmp_152;
    case 150: __it_set(d, 16); __it_set(d, 32); goto jmp_144;
    case 142: __it_set(d, 16); __it_set(d, 32); goto jmp_136;
    case 134: __it_set(d, 16); __it_set(d, 32); goto jmp_128;
    case 126: __it_set(d, 16); __it_set(d, 32); goto jmp_120;
    case 118: __it_set(d, 16); __it_set(d, 32); goto jmp_112;
    case 110: __it_set(d, 16); __it_set(d, 32); goto jmp_104;
    case 102: __it_set(d, 16); __it_set(d, 32); goto jmp_96;
    case 94: __it_set(d, 16); __it_set(d, 32); goto jmp_88;
    case 86: __it_set(d, 16); __it_set(d, 32); goto jmp_80;
    case 78: __it_set(d, 16); __it_set(d, 32); goto jmp_72;
    case 70: __it_set(d, 16); __it_set(d, 32); goto jmp_64;
    case 62: __it_set(d, 16); __it_set(d, 32); goto jmp_56;
    case 54: __it_set(d, 16); __it_set(d, 32); goto jmp_48;
    case 46: __it_set(d, 16); __it_set(d, 32); goto jmp_40;
    case 38: __it_set(d, 16); __it_set(d, 32); goto jmp_32;
    case 30: __it_set(d, 16); __it_set(d, 32); goto jmp_24;
    case 22: __it_set(d, 16); __it_set(d, 32); goto jmp_16;
    case 14: __it_set(d, 16); __it_set(d, 32); goto jmp_8;
    case 6: __it_set(d, 16); __it_set(d, 32); break;
    
    case 508: __it_set(d, 32); goto jmp_504;
    case 500: __it_set(d, 32); goto jmp_496;
    case 492: __it_set(d, 32); goto jmp_488;
    case 484: __it_set(d, 32); goto jmp_480;
    case 476: __it_set(d, 32); goto jmp_472;
    case 468: __it_set(d, 32); goto jmp_464;
    case 460: __it_set(d, 32); goto jmp_456;
    case 452: __it_set(d, 32); goto jmp_448;
    case 444: __it_set(d, 32); goto jmp_440;
    case 436: __it_set(d, 32); goto jmp_432;
    case 428: __it_set(d, 32); goto jmp_424;
    case 420: __it_set(d, 32); goto jmp_416;
    case 412: __it_set(d, 32); goto jmp_408;
    case 404: __it_set(d, 32); goto jmp_400;
    case 396: __it_set(d, 32); goto jmp_392;
    case 388: __it_set(d, 32); goto jmp_384;
    case 380: __it_set(d, 32); goto jmp_376;
    case 372: __it_set(d, 32); goto jmp_368;
    case 364: __it_set(d, 32); goto jmp_360;
    case 356: __it_set(d, 32); goto jmp_352;
    case 348: __it_set(d, 32); goto jmp_344;
    case 340: __it_set(d, 32); goto jmp_336;
    case 332: __it_set(d, 32); goto jmp_328;
    case 324: __it_set(d, 32); goto jmp_320;
    case 316: __it_set(d, 32); goto jmp_312;
    case 308: __it_set(d, 32); goto jmp_304;
    case 300: __it_set(d, 32); goto jmp_296;
    case 292: __it_set(d, 32); goto jmp_288;
    case 284: __it_set(d, 32); goto jmp_280;
    case 276: __it_set(d, 32); goto jmp_272;
    case 268: __it_set(d, 32); goto jmp_264;
    case 260: __it_set(d, 32); goto jmp_256;
    case 252: __it_set(d, 32); goto jmp_248;
    case 244: __it_set(d, 32); goto jmp_240;
    case 236: __it_set(d, 32); goto jmp_232;
    case 228: __it_set(d, 32); goto jmp_224;
    case 220: __it_set(d, 32); goto jmp_216;
    case 212: __it_set(d, 32); goto jmp_208;
    case 204: __it_set(d, 32); goto jmp_200;
    case 196: __it_set(d, 32); goto jmp_192;
    case 188: __it_set(d, 32); goto jmp_184;
    case 180: __it_set(d, 32); goto jmp_176;
    case 172: __it_set(d, 32); goto jmp_168;
    case 164: __it_set(d, 32); goto jmp_160;
    case 156: __it_set(d, 32); goto jmp_152;
    case 148: __it_set(d, 32); goto jmp_144;
    case 140: __it_set(d, 32); goto jmp_136;
    case 132: __it_set(d, 32); goto jmp_128;
    case 124: __it_set(d, 32); goto jmp_120;
    case 116: __it_set(d, 32); goto jmp_112;
    case 108: __it_set(d, 32); goto jmp_104;
    case 100: __it_set(d, 32); goto jmp_96;
    case 92: __it_set(d, 32); goto jmp_88;
    case 84: __it_set(d, 32); goto jmp_80;
    case 76: __it_set(d, 32); goto jmp_72;
    case 68: __it_set(d, 32); goto jmp_64;
    case 60: __it_set(d, 32); goto jmp_56;
    case 52: __it_set(d, 32); goto jmp_48;
    case 44: __it_set(d, 32); goto jmp_40;
    case 36: __it_set(d, 32); goto jmp_32;
    case 28: __it_set(d, 32); goto jmp_24;
    case 20: __it_set(d, 32); goto jmp_16;
    case 12: __it_set(d, 32); goto jmp_8;
    case 4: __it_set(d, 32); break;
    
    case 506: __it_set(d, 16); goto jmp_504;
    case 498: __it_set(d, 16); goto jmp_496;
    case 490: __it_set(d, 16); goto jmp_488;
    case 482: __it_set(d, 16); goto jmp_480;
    case 474: __it_set(d, 16); goto jmp_472;
    case 466: __it_set(d, 16); goto jmp_464;
    case 458: __it_set(d, 16); goto jmp_456;
    case 450: __it_set(d, 16); goto jmp_448;
    case 442: __it_set(d, 16); goto jmp_440;
    case 434: __it_set(d, 16); goto jmp_432;
    case 426: __it_set(d, 16); goto jmp_424;
    case 418: __it_set(d, 16); goto jmp_416;
    case 410: __it_set(d, 16); goto jmp_408;
    case 402: __it_set(d, 16); goto jmp_400;
    case 394: __it_set(d, 16); goto jmp_392;
    case 386: __it_set(d, 16); goto jmp_384;
    case 378: __it_set(d, 16); goto jmp_376;
    case 370: __it_set(d, 16); goto jmp_368;
    case 362: __it_set(d, 16); goto jmp_360;
    case 354: __it_set(d, 16); goto jmp_352;
    case 346: __it_set(d, 16); goto jmp_344;
    case 338: __it_set(d, 16); goto jmp_336;
    case 330: __it_set(d, 16); goto jmp_328;
    case 322: __it_set(d, 16); goto jmp_320;
    case 314: __it_set(d, 16); goto jmp_312;
    case 306: __it_set(d, 16); goto jmp_304;
    case 298: __it_set(d, 16); goto jmp_296;
    case 290: __it_set(d, 16); goto jmp_288;
    case 282: __it_set(d, 16); goto jmp_280;
    case 274: __it_set(d, 16); goto jmp_272;
    case 266: __it_set(d, 16); goto jmp_264;
    case 258: __it_set(d, 16); goto jmp_256;
    case 250: __it_set(d, 16); goto jmp_248;
    case 242: __it_set(d, 16); goto jmp_240;
    case 234: __it_set(d, 16); goto jmp_232;
    case 226: __it_set(d, 16); goto jmp_224;
    case 218: __it_set(d, 16); goto jmp_216;
    case 210: __it_set(d, 16); goto jmp_208;
    case 202: __it_set(d, 16); goto jmp_200;
    case 194: __it_set(d, 16); goto jmp_192;
    case 186: __it_set(d, 16); goto jmp_184;
    case 178: __it_set(d, 16); goto jmp_176;
    case 170: __it_set(d, 16); goto jmp_168;
    case 162: __it_set(d, 16); goto jmp_160;
    case 154: __it_set(d, 16); goto jmp_152;
    case 146: __it_set(d, 16); goto jmp_144;
    case 138: __it_set(d, 16); goto jmp_136;
    case 130: __it_set(d, 16); goto jmp_128;
    case 122: __it_set(d, 16); goto jmp_120;
    case 114: __it_set(d, 16); goto jmp_112;
    case 106: __it_set(d, 16); goto jmp_104;
    case 98: __it_set(d, 16); goto jmp_96;
    case 90: __it_set(d, 16); goto jmp_88;
    case 82: __it_set(d, 16); goto jmp_80;
    case 74: __it_set(d, 16); goto jmp_72;
    case 66: __it_set(d, 16); goto jmp_64;
    case 58: __it_set(d, 16); goto jmp_56;
    case 50: __it_set(d, 16); goto jmp_48;
    case 42: __it_set(d, 16); goto jmp_40;
    case 34: __it_set(d, 16); goto jmp_32;
    case 26: __it_set(d, 16); goto jmp_24;
    case 18: __it_set(d, 16); goto jmp_16;
    case 10: __it_set(d, 16); goto jmp_8;
    case 2: __it_set(d, 16); break;
    
    case 1: __it_set(d, 8); break;

   	default:
		/* __builtin_memset() is crappy slow since it cannot
		 * make any assumptions about alignment & underlying
		 * efficient unaligned access on the target we're
		 * running.
		 */
		__throw_build_bug();
	}
#else
	__bpf_memset_builtin(d, 0, len);
#endif
}

static __always_inline __maybe_unused void*
__bpf_no_builtin_memset(void *d __maybe_unused, __u8 c __maybe_unused,
			__u64 len __maybe_unused)
{
	__throw_build_bug();
}

/* Redirect any direct use in our code to throw an error. */
#define __builtin_memset	__bpf_no_builtin_memset

static __always_inline __maybe_unused __nobuiltin("memset") void bpf_memset(void *d, int c,
							 __u64 len)
{
	if (__builtin_constant_p(len) && __builtin_constant_p(c) && c == 0)
		__bpf_memzero(d, len);
	else
		__bpf_memset_builtin(d, (__u8)c, len);
}

static __always_inline __maybe_unused void
__bpf_memcpy_builtin(void *d, const void *s, __u64 len)
{
	/* Explicit opt-in for __builtin_memcpy(). */
	__builtin_memcpy(d, s, len);
}

static __always_inline void __bpf_memcpy(void *d, const void *s, __u64 len)
{
#if __clang_major__ >= 10
	if (!__builtin_constant_p(len))
		__throw_build_bug();

	d += len;
	s += len;

	if (len > 1 && len % 2 == 1) {
		__it_mob(d, s, 8);
		len -= 1;
	}

	switch (len) {
    case 512:          __it_mob(d, s, 64);
    case 504: jmp_504: __it_mob(d, s, 64);
    case 496: jmp_496: __it_mob(d, s, 64);
    case 488: jmp_488: __it_mob(d, s, 64);
    case 480: jmp_480: __it_mob(d, s, 64);
    case 472: jmp_472: __it_mob(d, s, 64);
    case 464: jmp_464: __it_mob(d, s, 64);
    case 456: jmp_456: __it_mob(d, s, 64);
    case 448: jmp_448: __it_mob(d, s, 64);
    case 440: jmp_440: __it_mob(d, s, 64);
    case 432: jmp_432: __it_mob(d, s, 64);
    case 424: jmp_424: __it_mob(d, s, 64);
    case 416: jmp_416: __it_mob(d, s, 64);
    case 408: jmp_408: __it_mob(d, s, 64);
    case 400: jmp_400: __it_mob(d, s, 64);
    case 392: jmp_392: __it_mob(d, s, 64);
    case 384: jmp_384: __it_mob(d, s, 64);
    case 376: jmp_376: __it_mob(d, s, 64);
    case 368: jmp_368: __it_mob(d, s, 64);
    case 360: jmp_360: __it_mob(d, s, 64);
    case 352: jmp_352: __it_mob(d, s, 64);
    case 344: jmp_344: __it_mob(d, s, 64);
    case 336: jmp_336: __it_mob(d, s, 64);
    case 328: jmp_328: __it_mob(d, s, 64);
    case 320: jmp_320: __it_mob(d, s, 64);
    case 312: jmp_312: __it_mob(d, s, 64);
    case 304: jmp_304: __it_mob(d, s, 64);
    case 296: jmp_296: __it_mob(d, s, 64);
    case 288: jmp_288: __it_mob(d, s, 64);
    case 280: jmp_280: __it_mob(d, s, 64);
    case 272: jmp_272: __it_mob(d, s, 64);
    case 264: jmp_264: __it_mob(d, s, 64);
    case 256: jmp_256: __it_mob(d, s, 64);
    case 248: jmp_248: __it_mob(d, s, 64);
    case 240: jmp_240: __it_mob(d, s, 64);
    case 232: jmp_232: __it_mob(d, s, 64);
    case 224: jmp_224: __it_mob(d, s, 64);
    case 216: jmp_216: __it_mob(d, s, 64);
    case 208: jmp_208: __it_mob(d, s, 64);
    case 200: jmp_200: __it_mob(d, s, 64);
    case 192: jmp_192: __it_mob(d, s, 64);
    case 184: jmp_184: __it_mob(d, s, 64);
    case 176: jmp_176: __it_mob(d, s, 64);
    case 168: jmp_168: __it_mob(d, s, 64);
    case 160: jmp_160: __it_mob(d, s, 64);
    case 152: jmp_152: __it_mob(d, s, 64);
    case 144: jmp_144: __it_mob(d, s, 64);
    case 136: jmp_136: __it_mob(d, s, 64);
    case 128: jmp_128: __it_mob(d, s, 64);
    case 120: jmp_120: __it_mob(d, s, 64);
    case 112: jmp_112: __it_mob(d, s, 64);
    case 104: jmp_104: __it_mob(d, s, 64);
    case 96: jmp_96: __it_mob(d, s, 64);
    case 88: jmp_88: __it_mob(d, s, 64);
    case 80: jmp_80: __it_mob(d, s, 64);
    case 72: jmp_72: __it_mob(d, s, 64);
    case 64: jmp_64: __it_mob(d, s, 64);
    case 56: jmp_56: __it_mob(d, s, 64);
    case 48: jmp_48: __it_mob(d, s, 64);
    case 40: jmp_40: __it_mob(d, s, 64);
    case 32: jmp_32: __it_mob(d, s, 64);
    case 24: jmp_24: __it_mob(d, s, 64);
    case 16: jmp_16: __it_mob(d, s, 64);
    case 8: jmp_8: __it_mob(d, s, 64); break;
    
    case 510: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_504;
    case 502: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_496;
    case 494: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_488;
    case 486: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_480;
    case 478: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_472;
    case 470: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_464;
    case 462: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_456;
    case 454: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_448;
    case 446: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_440;
    case 438: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_432;
    case 430: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_424;
    case 422: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_416;
    case 414: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_408;
    case 406: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_400;
    case 398: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_392;
    case 390: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_384;
    case 382: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_376;
    case 374: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_368;
    case 366: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_360;
    case 358: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_352;
    case 350: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_344;
    case 342: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_336;
    case 334: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_328;
    case 326: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_320;
    case 318: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_312;
    case 310: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_304;
    case 302: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_296;
    case 294: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_288;
    case 286: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_280;
    case 278: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_272;
    case 270: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_264;
    case 262: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_256;
    case 254: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_248;
    case 246: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_240;
    case 238: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_232;
    case 230: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_224;
    case 222: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_216;
    case 214: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_208;
    case 206: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_200;
    case 198: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_192;
    case 190: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_184;
    case 182: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_176;
    case 174: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_168;
    case 166: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_160;
    case 158: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_152;
    case 150: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_144;
    case 142: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_136;
    case 134: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_128;
    case 126: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_120;
    case 118: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_112;
    case 110: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_104;
    case 102: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_96;
    case 94: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_88;
    case 86: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_80;
    case 78: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_72;
    case 70: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_64;
    case 62: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_56;
    case 54: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_48;
    case 46: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_40;
    case 38: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_32;
    case 30: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_24;
    case 22: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_16;
    case 14: __it_mob(d, s, 16); __it_mob(d, s, 32); goto jmp_8;
    case 6: __it_mob(d, s, 16); __it_mob(d, s, 32); break;
    
    case 508: __it_mob(d, s, 32); goto jmp_504;
    case 500: __it_mob(d, s, 32); goto jmp_496;
    case 492: __it_mob(d, s, 32); goto jmp_488;
    case 484: __it_mob(d, s, 32); goto jmp_480;
    case 476: __it_mob(d, s, 32); goto jmp_472;
    case 468: __it_mob(d, s, 32); goto jmp_464;
    case 460: __it_mob(d, s, 32); goto jmp_456;
    case 452: __it_mob(d, s, 32); goto jmp_448;
    case 444: __it_mob(d, s, 32); goto jmp_440;
    case 436: __it_mob(d, s, 32); goto jmp_432;
    case 428: __it_mob(d, s, 32); goto jmp_424;
    case 420: __it_mob(d, s, 32); goto jmp_416;
    case 412: __it_mob(d, s, 32); goto jmp_408;
    case 404: __it_mob(d, s, 32); goto jmp_400;
    case 396: __it_mob(d, s, 32); goto jmp_392;
    case 388: __it_mob(d, s, 32); goto jmp_384;
    case 380: __it_mob(d, s, 32); goto jmp_376;
    case 372: __it_mob(d, s, 32); goto jmp_368;
    case 364: __it_mob(d, s, 32); goto jmp_360;
    case 356: __it_mob(d, s, 32); goto jmp_352;
    case 348: __it_mob(d, s, 32); goto jmp_344;
    case 340: __it_mob(d, s, 32); goto jmp_336;
    case 332: __it_mob(d, s, 32); goto jmp_328;
    case 324: __it_mob(d, s, 32); goto jmp_320;
    case 316: __it_mob(d, s, 32); goto jmp_312;
    case 308: __it_mob(d, s, 32); goto jmp_304;
    case 300: __it_mob(d, s, 32); goto jmp_296;
    case 292: __it_mob(d, s, 32); goto jmp_288;
    case 284: __it_mob(d, s, 32); goto jmp_280;
    case 276: __it_mob(d, s, 32); goto jmp_272;
    case 268: __it_mob(d, s, 32); goto jmp_264;
    case 260: __it_mob(d, s, 32); goto jmp_256;
    case 252: __it_mob(d, s, 32); goto jmp_248;
    case 244: __it_mob(d, s, 32); goto jmp_240;
    case 236: __it_mob(d, s, 32); goto jmp_232;
    case 228: __it_mob(d, s, 32); goto jmp_224;
    case 220: __it_mob(d, s, 32); goto jmp_216;
    case 212: __it_mob(d, s, 32); goto jmp_208;
    case 204: __it_mob(d, s, 32); goto jmp_200;
    case 196: __it_mob(d, s, 32); goto jmp_192;
    case 188: __it_mob(d, s, 32); goto jmp_184;
    case 180: __it_mob(d, s, 32); goto jmp_176;
    case 172: __it_mob(d, s, 32); goto jmp_168;
    case 164: __it_mob(d, s, 32); goto jmp_160;
    case 156: __it_mob(d, s, 32); goto jmp_152;
    case 148: __it_mob(d, s, 32); goto jmp_144;
    case 140: __it_mob(d, s, 32); goto jmp_136;
    case 132: __it_mob(d, s, 32); goto jmp_128;
    case 124: __it_mob(d, s, 32); goto jmp_120;
    case 116: __it_mob(d, s, 32); goto jmp_112;
    case 108: __it_mob(d, s, 32); goto jmp_104;
    case 100: __it_mob(d, s, 32); goto jmp_96;
    case 92: __it_mob(d, s, 32); goto jmp_88;
    case 84: __it_mob(d, s, 32); goto jmp_80;
    case 76: __it_mob(d, s, 32); goto jmp_72;
    case 68: __it_mob(d, s, 32); goto jmp_64;
    case 60: __it_mob(d, s, 32); goto jmp_56;
    case 52: __it_mob(d, s, 32); goto jmp_48;
    case 44: __it_mob(d, s, 32); goto jmp_40;
    case 36: __it_mob(d, s, 32); goto jmp_32;
    case 28: __it_mob(d, s, 32); goto jmp_24;
    case 20: __it_mob(d, s, 32); goto jmp_16;
    case 12: __it_mob(d, s, 32); goto jmp_8;
    case 4: __it_mob(d, s, 32); break;
    
    case 506: __it_mob(d, s, 16); goto jmp_504;
    case 498: __it_mob(d, s, 16); goto jmp_496;
    case 490: __it_mob(d, s, 16); goto jmp_488;
    case 482: __it_mob(d, s, 16); goto jmp_480;
    case 474: __it_mob(d, s, 16); goto jmp_472;
    case 466: __it_mob(d, s, 16); goto jmp_464;
    case 458: __it_mob(d, s, 16); goto jmp_456;
    case 450: __it_mob(d, s, 16); goto jmp_448;
    case 442: __it_mob(d, s, 16); goto jmp_440;
    case 434: __it_mob(d, s, 16); goto jmp_432;
    case 426: __it_mob(d, s, 16); goto jmp_424;
    case 418: __it_mob(d, s, 16); goto jmp_416;
    case 410: __it_mob(d, s, 16); goto jmp_408;
    case 402: __it_mob(d, s, 16); goto jmp_400;
    case 394: __it_mob(d, s, 16); goto jmp_392;
    case 386: __it_mob(d, s, 16); goto jmp_384;
    case 378: __it_mob(d, s, 16); goto jmp_376;
    case 370: __it_mob(d, s, 16); goto jmp_368;
    case 362: __it_mob(d, s, 16); goto jmp_360;
    case 354: __it_mob(d, s, 16); goto jmp_352;
    case 346: __it_mob(d, s, 16); goto jmp_344;
    case 338: __it_mob(d, s, 16); goto jmp_336;
    case 330: __it_mob(d, s, 16); goto jmp_328;
    case 322: __it_mob(d, s, 16); goto jmp_320;
    case 314: __it_mob(d, s, 16); goto jmp_312;
    case 306: __it_mob(d, s, 16); goto jmp_304;
    case 298: __it_mob(d, s, 16); goto jmp_296;
    case 290: __it_mob(d, s, 16); goto jmp_288;
    case 282: __it_mob(d, s, 16); goto jmp_280;
    case 274: __it_mob(d, s, 16); goto jmp_272;
    case 266: __it_mob(d, s, 16); goto jmp_264;
    case 258: __it_mob(d, s, 16); goto jmp_256;
    case 250: __it_mob(d, s, 16); goto jmp_248;
    case 242: __it_mob(d, s, 16); goto jmp_240;
    case 234: __it_mob(d, s, 16); goto jmp_232;
    case 226: __it_mob(d, s, 16); goto jmp_224;
    case 218: __it_mob(d, s, 16); goto jmp_216;
    case 210: __it_mob(d, s, 16); goto jmp_208;
    case 202: __it_mob(d, s, 16); goto jmp_200;
    case 194: __it_mob(d, s, 16); goto jmp_192;
    case 186: __it_mob(d, s, 16); goto jmp_184;
    case 178: __it_mob(d, s, 16); goto jmp_176;
    case 170: __it_mob(d, s, 16); goto jmp_168;
    case 162: __it_mob(d, s, 16); goto jmp_160;
    case 154: __it_mob(d, s, 16); goto jmp_152;
    case 146: __it_mob(d, s, 16); goto jmp_144;
    case 138: __it_mob(d, s, 16); goto jmp_136;
    case 130: __it_mob(d, s, 16); goto jmp_128;
    case 122: __it_mob(d, s, 16); goto jmp_120;
    case 114: __it_mob(d, s, 16); goto jmp_112;
    case 106: __it_mob(d, s, 16); goto jmp_104;
    case 98: __it_mob(d, s, 16); goto jmp_96;
    case 90: __it_mob(d, s, 16); goto jmp_88;
    case 82: __it_mob(d, s, 16); goto jmp_80;
    case 74: __it_mob(d, s, 16); goto jmp_72;
    case 66: __it_mob(d, s, 16); goto jmp_64;
    case 58: __it_mob(d, s, 16); goto jmp_56;
    case 50: __it_mob(d, s, 16); goto jmp_48;
    case 42: __it_mob(d, s, 16); goto jmp_40;
    case 34: __it_mob(d, s, 16); goto jmp_32;
    case 26: __it_mob(d, s, 16); goto jmp_24;
    case 18: __it_mob(d, s, 16); goto jmp_16;
    case 10: __it_mob(d, s, 16); goto jmp_8;
    case 2: __it_mob(d, s, 16); break;
    
    case 1: __it_mob(d, s, 8); break;

	default:
		/* __builtin_memcpy() is crappy slow since it cannot
		 * make any assumptions about alignment & underlying
		 * efficient unaligned access on the target we're
		 * running.
		 */
		__throw_build_bug();
	}
#else
	__bpf_memcpy_builtin(d, s, len);
#endif
}

static __always_inline __maybe_unused void*
__bpf_no_builtin_memcpy(void *d __maybe_unused, const void *s __maybe_unused,
			__u64 len __maybe_unused)
{
	__throw_build_bug();
}

/* Redirect any direct use in our code to throw an error. */
#define __builtin_memcpy	__bpf_no_builtin_memcpy

static __always_inline __maybe_unused __nobuiltin("memcpy") void bpf_memcpy(void *d, const void *s,
							 __u64 len)
{
	return __bpf_memcpy(d, s, len);
}

static __always_inline __maybe_unused __u64
__bpf_memcmp_builtin(const void *x, const void *y, __u64 len)
{
	/* Explicit opt-in for __builtin_memcmp(). We use the bcmp builtin
	 * here for two reasons: i) we only need to know equal or non-equal
	 * similar as in __bpf_memcmp(), and ii) if __bpf_memcmp() ends up
	 * selecting __bpf_memcmp_builtin(), clang generats a memcmp loop.
	 * That is, (*) -> __bpf_memcmp() -> __bpf_memcmp_builtin() ->
	 * __builtin_memcmp() -> memcmp() -> (*), meaning it will end up
	 * selecting our memcmp() from here. Remapping to __builtin_bcmp()
	 * breaks this loop and resolves both needs at once.
	 */
	return __builtin_bcmp(x, y, len);
}

static __always_inline __u64 __bpf_memcmp(const void *x, const void *y,
					  __u64 len)
{
#if __clang_major__ >= 10
	__u64 r = 0;

	if (!__builtin_constant_p(len))
		__throw_build_bug();

	x += len;
	y += len;

	if (len > 1 && len % 2 == 1) {
		__it_xor(x, y, r, 8);
		len -= 1;
	}

	switch (len) {
    case 512:          __it_xor(x, y, r, 64);
    case 504: jmp_504: __it_xor(x, y, r, 64);
    case 496: jmp_496: __it_xor(x, y, r, 64);
    case 488: jmp_488: __it_xor(x, y, r, 64);
    case 480: jmp_480: __it_xor(x, y, r, 64);
    case 472: jmp_472: __it_xor(x, y, r, 64);
    case 464: jmp_464: __it_xor(x, y, r, 64);
    case 456: jmp_456: __it_xor(x, y, r, 64);
    case 448: jmp_448: __it_xor(x, y, r, 64);
    case 440: jmp_440: __it_xor(x, y, r, 64);
    case 432: jmp_432: __it_xor(x, y, r, 64);
    case 424: jmp_424: __it_xor(x, y, r, 64);
    case 416: jmp_416: __it_xor(x, y, r, 64);
    case 408: jmp_408: __it_xor(x, y, r, 64);
    case 400: jmp_400: __it_xor(x, y, r, 64);
    case 392: jmp_392: __it_xor(x, y, r, 64);
    case 384: jmp_384: __it_xor(x, y, r, 64);
    case 376: jmp_376: __it_xor(x, y, r, 64);
    case 368: jmp_368: __it_xor(x, y, r, 64);
    case 360: jmp_360: __it_xor(x, y, r, 64);
    case 352: jmp_352: __it_xor(x, y, r, 64);
    case 344: jmp_344: __it_xor(x, y, r, 64);
    case 336: jmp_336: __it_xor(x, y, r, 64);
    case 328: jmp_328: __it_xor(x, y, r, 64);
    case 320: jmp_320: __it_xor(x, y, r, 64);
    case 312: jmp_312: __it_xor(x, y, r, 64);
    case 304: jmp_304: __it_xor(x, y, r, 64);
    case 296: jmp_296: __it_xor(x, y, r, 64);
    case 288: jmp_288: __it_xor(x, y, r, 64);
    case 280: jmp_280: __it_xor(x, y, r, 64);
    case 272: jmp_272: __it_xor(x, y, r, 64);
    case 264: jmp_264: __it_xor(x, y, r, 64);
    case 256: jmp_256: __it_xor(x, y, r, 64);
    case 248: jmp_248: __it_xor(x, y, r, 64);
    case 240: jmp_240: __it_xor(x, y, r, 64);
    case 232: jmp_232: __it_xor(x, y, r, 64);
    case 224: jmp_224: __it_xor(x, y, r, 64);
    case 216: jmp_216: __it_xor(x, y, r, 64);
    case 208: jmp_208: __it_xor(x, y, r, 64);
    case 200: jmp_200: __it_xor(x, y, r, 64);
    case 192: jmp_192: __it_xor(x, y, r, 64);
    case 184: jmp_184: __it_xor(x, y, r, 64);
    case 176: jmp_176: __it_xor(x, y, r, 64);
    case 168: jmp_168: __it_xor(x, y, r, 64);
    case 160: jmp_160: __it_xor(x, y, r, 64);
    case 152: jmp_152: __it_xor(x, y, r, 64);
    case 144: jmp_144: __it_xor(x, y, r, 64);
    case 136: jmp_136: __it_xor(x, y, r, 64);
    case 128: jmp_128: __it_xor(x, y, r, 64);
    case 120: jmp_120: __it_xor(x, y, r, 64);
    case 112: jmp_112: __it_xor(x, y, r, 64);
    case 104: jmp_104: __it_xor(x, y, r, 64);
    case 96: jmp_96: __it_xor(x, y, r, 64);
    case 88: jmp_88: __it_xor(x, y, r, 64);
    case 80: jmp_80: __it_xor(x, y, r, 64);
    case 72: jmp_72: __it_xor(x, y, r, 64);
    case 64: jmp_64: __it_xor(x, y, r, 64);
    case 56: jmp_56: __it_xor(x, y, r, 64);
    case 48: jmp_48: __it_xor(x, y, r, 64);
    case 40: jmp_40: __it_xor(x, y, r, 64);
    case 32: jmp_32: __it_xor(x, y, r, 64);
    case 24: jmp_24: __it_xor(x, y, r, 64);
    case 16: jmp_16: __it_xor(x, y, r, 64);
    case 8: jmp_8: __it_xor(x, y, r, 64); break;
    
    case 510: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_504;
    case 502: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_496;
    case 494: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_488;
    case 486: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_480;
    case 478: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_472;
    case 470: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_464;
    case 462: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_456;
    case 454: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_448;
    case 446: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_440;
    case 438: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_432;
    case 430: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_424;
    case 422: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_416;
    case 414: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_408;
    case 406: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_400;
    case 398: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_392;
    case 390: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_384;
    case 382: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_376;
    case 374: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_368;
    case 366: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_360;
    case 358: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_352;
    case 350: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_344;
    case 342: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_336;
    case 334: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_328;
    case 326: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_320;
    case 318: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_312;
    case 310: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_304;
    case 302: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_296;
    case 294: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_288;
    case 286: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_280;
    case 278: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_272;
    case 270: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_264;
    case 262: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_256;
    case 254: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_248;
    case 246: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_240;
    case 238: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_232;
    case 230: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_224;
    case 222: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_216;
    case 214: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_208;
    case 206: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_200;
    case 198: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_192;
    case 190: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_184;
    case 182: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_176;
    case 174: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_168;
    case 166: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_160;
    case 158: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_152;
    case 150: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_144;
    case 142: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_136;
    case 134: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_128;
    case 126: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_120;
    case 118: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_112;
    case 110: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_104;
    case 102: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_96;
    case 94: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_88;
    case 86: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_80;
    case 78: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_72;
    case 70: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_64;
    case 62: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_56;
    case 54: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_48;
    case 46: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_40;
    case 38: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_32;
    case 30: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_24;
    case 22: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_16;
    case 14: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); goto jmp_8;
    case 6: __it_xor(x, y, r, 16); __it_xor(x, y, r, 32); break;
    
    case 508: __it_xor(x, y, r, 32); goto jmp_504;
    case 500: __it_xor(x, y, r, 32); goto jmp_496;
    case 492: __it_xor(x, y, r, 32); goto jmp_488;
    case 484: __it_xor(x, y, r, 32); goto jmp_480;
    case 476: __it_xor(x, y, r, 32); goto jmp_472;
    case 468: __it_xor(x, y, r, 32); goto jmp_464;
    case 460: __it_xor(x, y, r, 32); goto jmp_456;
    case 452: __it_xor(x, y, r, 32); goto jmp_448;
    case 444: __it_xor(x, y, r, 32); goto jmp_440;
    case 436: __it_xor(x, y, r, 32); goto jmp_432;
    case 428: __it_xor(x, y, r, 32); goto jmp_424;
    case 420: __it_xor(x, y, r, 32); goto jmp_416;
    case 412: __it_xor(x, y, r, 32); goto jmp_408;
    case 404: __it_xor(x, y, r, 32); goto jmp_400;
    case 396: __it_xor(x, y, r, 32); goto jmp_392;
    case 388: __it_xor(x, y, r, 32); goto jmp_384;
    case 380: __it_xor(x, y, r, 32); goto jmp_376;
    case 372: __it_xor(x, y, r, 32); goto jmp_368;
    case 364: __it_xor(x, y, r, 32); goto jmp_360;
    case 356: __it_xor(x, y, r, 32); goto jmp_352;
    case 348: __it_xor(x, y, r, 32); goto jmp_344;
    case 340: __it_xor(x, y, r, 32); goto jmp_336;
    case 332: __it_xor(x, y, r, 32); goto jmp_328;
    case 324: __it_xor(x, y, r, 32); goto jmp_320;
    case 316: __it_xor(x, y, r, 32); goto jmp_312;
    case 308: __it_xor(x, y, r, 32); goto jmp_304;
    case 300: __it_xor(x, y, r, 32); goto jmp_296;
    case 292: __it_xor(x, y, r, 32); goto jmp_288;
    case 284: __it_xor(x, y, r, 32); goto jmp_280;
    case 276: __it_xor(x, y, r, 32); goto jmp_272;
    case 268: __it_xor(x, y, r, 32); goto jmp_264;
    case 260: __it_xor(x, y, r, 32); goto jmp_256;
    case 252: __it_xor(x, y, r, 32); goto jmp_248;
    case 244: __it_xor(x, y, r, 32); goto jmp_240;
    case 236: __it_xor(x, y, r, 32); goto jmp_232;
    case 228: __it_xor(x, y, r, 32); goto jmp_224;
    case 220: __it_xor(x, y, r, 32); goto jmp_216;
    case 212: __it_xor(x, y, r, 32); goto jmp_208;
    case 204: __it_xor(x, y, r, 32); goto jmp_200;
    case 196: __it_xor(x, y, r, 32); goto jmp_192;
    case 188: __it_xor(x, y, r, 32); goto jmp_184;
    case 180: __it_xor(x, y, r, 32); goto jmp_176;
    case 172: __it_xor(x, y, r, 32); goto jmp_168;
    case 164: __it_xor(x, y, r, 32); goto jmp_160;
    case 156: __it_xor(x, y, r, 32); goto jmp_152;
    case 148: __it_xor(x, y, r, 32); goto jmp_144;
    case 140: __it_xor(x, y, r, 32); goto jmp_136;
    case 132: __it_xor(x, y, r, 32); goto jmp_128;
    case 124: __it_xor(x, y, r, 32); goto jmp_120;
    case 116: __it_xor(x, y, r, 32); goto jmp_112;
    case 108: __it_xor(x, y, r, 32); goto jmp_104;
    case 100: __it_xor(x, y, r, 32); goto jmp_96;
    case 92: __it_xor(x, y, r, 32); goto jmp_88;
    case 84: __it_xor(x, y, r, 32); goto jmp_80;
    case 76: __it_xor(x, y, r, 32); goto jmp_72;
    case 68: __it_xor(x, y, r, 32); goto jmp_64;
    case 60: __it_xor(x, y, r, 32); goto jmp_56;
    case 52: __it_xor(x, y, r, 32); goto jmp_48;
    case 44: __it_xor(x, y, r, 32); goto jmp_40;
    case 36: __it_xor(x, y, r, 32); goto jmp_32;
    case 28: __it_xor(x, y, r, 32); goto jmp_24;
    case 20: __it_xor(x, y, r, 32); goto jmp_16;
    case 12: __it_xor(x, y, r, 32); goto jmp_8;
    case 4: __it_xor(x, y, r, 32); break;
    
    case 506: __it_xor(x, y, r, 16); goto jmp_504;
    case 498: __it_xor(x, y, r, 16); goto jmp_496;
    case 490: __it_xor(x, y, r, 16); goto jmp_488;
    case 482: __it_xor(x, y, r, 16); goto jmp_480;
    case 474: __it_xor(x, y, r, 16); goto jmp_472;
    case 466: __it_xor(x, y, r, 16); goto jmp_464;
    case 458: __it_xor(x, y, r, 16); goto jmp_456;
    case 450: __it_xor(x, y, r, 16); goto jmp_448;
    case 442: __it_xor(x, y, r, 16); goto jmp_440;
    case 434: __it_xor(x, y, r, 16); goto jmp_432;
    case 426: __it_xor(x, y, r, 16); goto jmp_424;
    case 418: __it_xor(x, y, r, 16); goto jmp_416;
    case 410: __it_xor(x, y, r, 16); goto jmp_408;
    case 402: __it_xor(x, y, r, 16); goto jmp_400;
    case 394: __it_xor(x, y, r, 16); goto jmp_392;
    case 386: __it_xor(x, y, r, 16); goto jmp_384;
    case 378: __it_xor(x, y, r, 16); goto jmp_376;
    case 370: __it_xor(x, y, r, 16); goto jmp_368;
    case 362: __it_xor(x, y, r, 16); goto jmp_360;
    case 354: __it_xor(x, y, r, 16); goto jmp_352;
    case 346: __it_xor(x, y, r, 16); goto jmp_344;
    case 338: __it_xor(x, y, r, 16); goto jmp_336;
    case 330: __it_xor(x, y, r, 16); goto jmp_328;
    case 322: __it_xor(x, y, r, 16); goto jmp_320;
    case 314: __it_xor(x, y, r, 16); goto jmp_312;
    case 306: __it_xor(x, y, r, 16); goto jmp_304;
    case 298: __it_xor(x, y, r, 16); goto jmp_296;
    case 290: __it_xor(x, y, r, 16); goto jmp_288;
    case 282: __it_xor(x, y, r, 16); goto jmp_280;
    case 274: __it_xor(x, y, r, 16); goto jmp_272;
    case 266: __it_xor(x, y, r, 16); goto jmp_264;
    case 258: __it_xor(x, y, r, 16); goto jmp_256;
    case 250: __it_xor(x, y, r, 16); goto jmp_248;
    case 242: __it_xor(x, y, r, 16); goto jmp_240;
    case 234: __it_xor(x, y, r, 16); goto jmp_232;
    case 226: __it_xor(x, y, r, 16); goto jmp_224;
    case 218: __it_xor(x, y, r, 16); goto jmp_216;
    case 210: __it_xor(x, y, r, 16); goto jmp_208;
    case 202: __it_xor(x, y, r, 16); goto jmp_200;
    case 194: __it_xor(x, y, r, 16); goto jmp_192;
    case 186: __it_xor(x, y, r, 16); goto jmp_184;
    case 178: __it_xor(x, y, r, 16); goto jmp_176;
    case 170: __it_xor(x, y, r, 16); goto jmp_168;
    case 162: __it_xor(x, y, r, 16); goto jmp_160;
    case 154: __it_xor(x, y, r, 16); goto jmp_152;
    case 146: __it_xor(x, y, r, 16); goto jmp_144;
    case 138: __it_xor(x, y, r, 16); goto jmp_136;
    case 130: __it_xor(x, y, r, 16); goto jmp_128;
    case 122: __it_xor(x, y, r, 16); goto jmp_120;
    case 114: __it_xor(x, y, r, 16); goto jmp_112;
    case 106: __it_xor(x, y, r, 16); goto jmp_104;
    case 98: __it_xor(x, y, r, 16); goto jmp_96;
    case 90: __it_xor(x, y, r, 16); goto jmp_88;
    case 82: __it_xor(x, y, r, 16); goto jmp_80;
    case 74: __it_xor(x, y, r, 16); goto jmp_72;
    case 66: __it_xor(x, y, r, 16); goto jmp_64;
    case 58: __it_xor(x, y, r, 16); goto jmp_56;
    case 50: __it_xor(x, y, r, 16); goto jmp_48;
    case 42: __it_xor(x, y, r, 16); goto jmp_40;
    case 34: __it_xor(x, y, r, 16); goto jmp_32;
    case 26: __it_xor(x, y, r, 16); goto jmp_24;
    case 18: __it_xor(x, y, r, 16); goto jmp_16;
    case 10: __it_xor(x, y, r, 16); goto jmp_8;
    case 2: __it_xor(x, y, r, 16); break;
    
    case 1: __it_xor(x, y, r, 8); break;

	default:
		__throw_build_bug();
	}

	return r;
#else
	return __bpf_memcmp_builtin(x, y, len);
#endif
}

static __always_inline __maybe_unused __u64
__bpf_no_builtin_memcmp(const void *x __maybe_unused,
			const void *y __maybe_unused, __u64 len __maybe_unused)
{
	__throw_build_bug();
	return 0;
}

/* Redirect any direct use in our code to throw an error. */
#define __builtin_memcmp	__bpf_no_builtin_memcmp

/* Modified for our needs in that we only return either zero (x and y
 * are equal) or non-zero (x and y are non-equal).
 */
static __always_inline __maybe_unused __nobuiltin("memcmp") __u64 bpf_memcmp(const void *x,
							  const void *y,
							  __u64 len)
{
	return __bpf_memcmp(x, y, len);
}

static __always_inline __maybe_unused void
__bpf_memmove_builtin(void *d, const void *s, __u64 len)
{
	/* Explicit opt-in for __builtin_memmove(). */
	__builtin_memmove(d, s, len);
}

static __always_inline void __bpf_memmove_bwd(void *d, const void *s, __u64 len)
{
	/* Our internal memcpy implementation walks backwards by default. */
	__bpf_memcpy(d, s, len);
}

static __always_inline void __bpf_memmove_fwd(void *d, const void *s, __u64 len)
{
#if __clang_major__ >= 10
	if (!__builtin_constant_p(len))
		__throw_build_bug();

	switch (len) {
    case 512:          __it_mof(d, s, 64);
    case 504: jmp_504: __it_mof(d, s, 64);
    case 496: jmp_496: __it_mof(d, s, 64);
    case 488: jmp_488: __it_mof(d, s, 64);
    case 480: jmp_480: __it_mof(d, s, 64);
    case 472: jmp_472: __it_mof(d, s, 64);
    case 464: jmp_464: __it_mof(d, s, 64);
    case 456: jmp_456: __it_mof(d, s, 64);
    case 448: jmp_448: __it_mof(d, s, 64);
    case 440: jmp_440: __it_mof(d, s, 64);
    case 432: jmp_432: __it_mof(d, s, 64);
    case 424: jmp_424: __it_mof(d, s, 64);
    case 416: jmp_416: __it_mof(d, s, 64);
    case 408: jmp_408: __it_mof(d, s, 64);
    case 400: jmp_400: __it_mof(d, s, 64);
    case 392: jmp_392: __it_mof(d, s, 64);
    case 384: jmp_384: __it_mof(d, s, 64);
    case 376: jmp_376: __it_mof(d, s, 64);
    case 368: jmp_368: __it_mof(d, s, 64);
    case 360: jmp_360: __it_mof(d, s, 64);
    case 352: jmp_352: __it_mof(d, s, 64);
    case 344: jmp_344: __it_mof(d, s, 64);
    case 336: jmp_336: __it_mof(d, s, 64);
    case 328: jmp_328: __it_mof(d, s, 64);
    case 320: jmp_320: __it_mof(d, s, 64);
    case 312: jmp_312: __it_mof(d, s, 64);
    case 304: jmp_304: __it_mof(d, s, 64);
    case 296: jmp_296: __it_mof(d, s, 64);
    case 288: jmp_288: __it_mof(d, s, 64);
    case 280: jmp_280: __it_mof(d, s, 64);
    case 272: jmp_272: __it_mof(d, s, 64);
    case 264: jmp_264: __it_mof(d, s, 64);
    case 256: jmp_256: __it_mof(d, s, 64);
    case 248: jmp_248: __it_mof(d, s, 64);
    case 240: jmp_240: __it_mof(d, s, 64);
    case 232: jmp_232: __it_mof(d, s, 64);
    case 224: jmp_224: __it_mof(d, s, 64);
    case 216: jmp_216: __it_mof(d, s, 64);
    case 208: jmp_208: __it_mof(d, s, 64);
    case 200: jmp_200: __it_mof(d, s, 64);
    case 192: jmp_192: __it_mof(d, s, 64);
    case 184: jmp_184: __it_mof(d, s, 64);
    case 176: jmp_176: __it_mof(d, s, 64);
    case 168: jmp_168: __it_mof(d, s, 64);
    case 160: jmp_160: __it_mof(d, s, 64);
    case 152: jmp_152: __it_mof(d, s, 64);
    case 144: jmp_144: __it_mof(d, s, 64);
    case 136: jmp_136: __it_mof(d, s, 64);
    case 128: jmp_128: __it_mof(d, s, 64);
    case 120: jmp_120: __it_mof(d, s, 64);
    case 112: jmp_112: __it_mof(d, s, 64);
    case 104: jmp_104: __it_mof(d, s, 64);
    case 96: jmp_96: __it_mof(d, s, 64);
    case 88: jmp_88: __it_mof(d, s, 64);
    case 80: jmp_80: __it_mof(d, s, 64);
    case 72: jmp_72: __it_mof(d, s, 64);
    case 64: jmp_64: __it_mof(d, s, 64);
    case 56: jmp_56: __it_mof(d, s, 64);
    case 48: jmp_48: __it_mof(d, s, 64);
    case 40: jmp_40: __it_mof(d, s, 64);
    case 32: jmp_32: __it_mof(d, s, 64);
    case 24: jmp_24: __it_mof(d, s, 64);
    case 16: jmp_16: __it_mof(d, s, 64);
    case 8: jmp_8: __it_mof(d, s, 64); break;
    
    case 510: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_504;
    case 502: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_496;
    case 494: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_488;
    case 486: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_480;
    case 478: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_472;
    case 470: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_464;
    case 462: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_456;
    case 454: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_448;
    case 446: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_440;
    case 438: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_432;
    case 430: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_424;
    case 422: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_416;
    case 414: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_408;
    case 406: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_400;
    case 398: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_392;
    case 390: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_384;
    case 382: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_376;
    case 374: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_368;
    case 366: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_360;
    case 358: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_352;
    case 350: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_344;
    case 342: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_336;
    case 334: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_328;
    case 326: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_320;
    case 318: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_312;
    case 310: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_304;
    case 302: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_296;
    case 294: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_288;
    case 286: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_280;
    case 278: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_272;
    case 270: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_264;
    case 262: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_256;
    case 254: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_248;
    case 246: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_240;
    case 238: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_232;
    case 230: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_224;
    case 222: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_216;
    case 214: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_208;
    case 206: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_200;
    case 198: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_192;
    case 190: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_184;
    case 182: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_176;
    case 174: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_168;
    case 166: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_160;
    case 158: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_152;
    case 150: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_144;
    case 142: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_136;
    case 134: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_128;
    case 126: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_120;
    case 118: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_112;
    case 110: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_104;
    case 102: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_96;
    case 94: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_88;
    case 86: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_80;
    case 78: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_72;
    case 70: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_64;
    case 62: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_56;
    case 54: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_48;
    case 46: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_40;
    case 38: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_32;
    case 30: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_24;
    case 22: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_16;
    case 14: __it_mof(d, s, 16); __it_mof(d, s, 32); goto jmp_8;
    case 6: __it_mof(d, s, 16); __it_mof(d, s, 32); break;
    
    case 508: __it_mof(d, s, 32); goto jmp_504;
    case 500: __it_mof(d, s, 32); goto jmp_496;
    case 492: __it_mof(d, s, 32); goto jmp_488;
    case 484: __it_mof(d, s, 32); goto jmp_480;
    case 476: __it_mof(d, s, 32); goto jmp_472;
    case 468: __it_mof(d, s, 32); goto jmp_464;
    case 460: __it_mof(d, s, 32); goto jmp_456;
    case 452: __it_mof(d, s, 32); goto jmp_448;
    case 444: __it_mof(d, s, 32); goto jmp_440;
    case 436: __it_mof(d, s, 32); goto jmp_432;
    case 428: __it_mof(d, s, 32); goto jmp_424;
    case 420: __it_mof(d, s, 32); goto jmp_416;
    case 412: __it_mof(d, s, 32); goto jmp_408;
    case 404: __it_mof(d, s, 32); goto jmp_400;
    case 396: __it_mof(d, s, 32); goto jmp_392;
    case 388: __it_mof(d, s, 32); goto jmp_384;
    case 380: __it_mof(d, s, 32); goto jmp_376;
    case 372: __it_mof(d, s, 32); goto jmp_368;
    case 364: __it_mof(d, s, 32); goto jmp_360;
    case 356: __it_mof(d, s, 32); goto jmp_352;
    case 348: __it_mof(d, s, 32); goto jmp_344;
    case 340: __it_mof(d, s, 32); goto jmp_336;
    case 332: __it_mof(d, s, 32); goto jmp_328;
    case 324: __it_mof(d, s, 32); goto jmp_320;
    case 316: __it_mof(d, s, 32); goto jmp_312;
    case 308: __it_mof(d, s, 32); goto jmp_304;
    case 300: __it_mof(d, s, 32); goto jmp_296;
    case 292: __it_mof(d, s, 32); goto jmp_288;
    case 284: __it_mof(d, s, 32); goto jmp_280;
    case 276: __it_mof(d, s, 32); goto jmp_272;
    case 268: __it_mof(d, s, 32); goto jmp_264;
    case 260: __it_mof(d, s, 32); goto jmp_256;
    case 252: __it_mof(d, s, 32); goto jmp_248;
    case 244: __it_mof(d, s, 32); goto jmp_240;
    case 236: __it_mof(d, s, 32); goto jmp_232;
    case 228: __it_mof(d, s, 32); goto jmp_224;
    case 220: __it_mof(d, s, 32); goto jmp_216;
    case 212: __it_mof(d, s, 32); goto jmp_208;
    case 204: __it_mof(d, s, 32); goto jmp_200;
    case 196: __it_mof(d, s, 32); goto jmp_192;
    case 188: __it_mof(d, s, 32); goto jmp_184;
    case 180: __it_mof(d, s, 32); goto jmp_176;
    case 172: __it_mof(d, s, 32); goto jmp_168;
    case 164: __it_mof(d, s, 32); goto jmp_160;
    case 156: __it_mof(d, s, 32); goto jmp_152;
    case 148: __it_mof(d, s, 32); goto jmp_144;
    case 140: __it_mof(d, s, 32); goto jmp_136;
    case 132: __it_mof(d, s, 32); goto jmp_128;
    case 124: __it_mof(d, s, 32); goto jmp_120;
    case 116: __it_mof(d, s, 32); goto jmp_112;
    case 108: __it_mof(d, s, 32); goto jmp_104;
    case 100: __it_mof(d, s, 32); goto jmp_96;
    case 92: __it_mof(d, s, 32); goto jmp_88;
    case 84: __it_mof(d, s, 32); goto jmp_80;
    case 76: __it_mof(d, s, 32); goto jmp_72;
    case 68: __it_mof(d, s, 32); goto jmp_64;
    case 60: __it_mof(d, s, 32); goto jmp_56;
    case 52: __it_mof(d, s, 32); goto jmp_48;
    case 44: __it_mof(d, s, 32); goto jmp_40;
    case 36: __it_mof(d, s, 32); goto jmp_32;
    case 28: __it_mof(d, s, 32); goto jmp_24;
    case 20: __it_mof(d, s, 32); goto jmp_16;
    case 12: __it_mof(d, s, 32); goto jmp_8;
    case 4: __it_mof(d, s, 32); break;
    
    case 506: __it_mof(d, s, 16); goto jmp_504;
    case 498: __it_mof(d, s, 16); goto jmp_496;
    case 490: __it_mof(d, s, 16); goto jmp_488;
    case 482: __it_mof(d, s, 16); goto jmp_480;
    case 474: __it_mof(d, s, 16); goto jmp_472;
    case 466: __it_mof(d, s, 16); goto jmp_464;
    case 458: __it_mof(d, s, 16); goto jmp_456;
    case 450: __it_mof(d, s, 16); goto jmp_448;
    case 442: __it_mof(d, s, 16); goto jmp_440;
    case 434: __it_mof(d, s, 16); goto jmp_432;
    case 426: __it_mof(d, s, 16); goto jmp_424;
    case 418: __it_mof(d, s, 16); goto jmp_416;
    case 410: __it_mof(d, s, 16); goto jmp_408;
    case 402: __it_mof(d, s, 16); goto jmp_400;
    case 394: __it_mof(d, s, 16); goto jmp_392;
    case 386: __it_mof(d, s, 16); goto jmp_384;
    case 378: __it_mof(d, s, 16); goto jmp_376;
    case 370: __it_mof(d, s, 16); goto jmp_368;
    case 362: __it_mof(d, s, 16); goto jmp_360;
    case 354: __it_mof(d, s, 16); goto jmp_352;
    case 346: __it_mof(d, s, 16); goto jmp_344;
    case 338: __it_mof(d, s, 16); goto jmp_336;
    case 330: __it_mof(d, s, 16); goto jmp_328;
    case 322: __it_mof(d, s, 16); goto jmp_320;
    case 314: __it_mof(d, s, 16); goto jmp_312;
    case 306: __it_mof(d, s, 16); goto jmp_304;
    case 298: __it_mof(d, s, 16); goto jmp_296;
    case 290: __it_mof(d, s, 16); goto jmp_288;
    case 282: __it_mof(d, s, 16); goto jmp_280;
    case 274: __it_mof(d, s, 16); goto jmp_272;
    case 266: __it_mof(d, s, 16); goto jmp_264;
    case 258: __it_mof(d, s, 16); goto jmp_256;
    case 250: __it_mof(d, s, 16); goto jmp_248;
    case 242: __it_mof(d, s, 16); goto jmp_240;
    case 234: __it_mof(d, s, 16); goto jmp_232;
    case 226: __it_mof(d, s, 16); goto jmp_224;
    case 218: __it_mof(d, s, 16); goto jmp_216;
    case 210: __it_mof(d, s, 16); goto jmp_208;
    case 202: __it_mof(d, s, 16); goto jmp_200;
    case 194: __it_mof(d, s, 16); goto jmp_192;
    case 186: __it_mof(d, s, 16); goto jmp_184;
    case 178: __it_mof(d, s, 16); goto jmp_176;
    case 170: __it_mof(d, s, 16); goto jmp_168;
    case 162: __it_mof(d, s, 16); goto jmp_160;
    case 154: __it_mof(d, s, 16); goto jmp_152;
    case 146: __it_mof(d, s, 16); goto jmp_144;
    case 138: __it_mof(d, s, 16); goto jmp_136;
    case 130: __it_mof(d, s, 16); goto jmp_128;
    case 122: __it_mof(d, s, 16); goto jmp_120;
    case 114: __it_mof(d, s, 16); goto jmp_112;
    case 106: __it_mof(d, s, 16); goto jmp_104;
    case 98: __it_mof(d, s, 16); goto jmp_96;
    case 90: __it_mof(d, s, 16); goto jmp_88;
    case 82: __it_mof(d, s, 16); goto jmp_80;
    case 74: __it_mof(d, s, 16); goto jmp_72;
    case 66: __it_mof(d, s, 16); goto jmp_64;
    case 58: __it_mof(d, s, 16); goto jmp_56;
    case 50: __it_mof(d, s, 16); goto jmp_48;
    case 42: __it_mof(d, s, 16); goto jmp_40;
    case 34: __it_mof(d, s, 16); goto jmp_32;
    case 26: __it_mof(d, s, 16); goto jmp_24;
    case 18: __it_mof(d, s, 16); goto jmp_16;
    case 10: __it_mof(d, s, 16); goto jmp_8;
    case 2: __it_mof(d, s, 16); break;
    
    case 1: __it_mof(d, s, 8); break;

	default:
		/* __builtin_memmove() is crappy slow since it cannot
		 * make any assumptions about alignment & underlying
		 * efficient unaligned access on the target we're
		 * running.
		 */
		__throw_build_bug();
	}
#else
	__bpf_memmove_builtin(d, s, len);
#endif
}

static __always_inline __maybe_unused void*
__bpf_no_builtin_memmove(void *d __maybe_unused, const void *s __maybe_unused,
			 __u64 len __maybe_unused)
{
	__throw_build_bug();
}

/* Redirect any direct use in our code to throw an error. */
#define __builtin_memmove	__bpf_no_builtin_memmove

static __always_inline void __bpf_memmove(void *d, const void *s, __u64 len)
{
	/* Note, the forward walking memmove() might not work with on-stack data
	 * since we'll end up walking the memory unaligned even when __align_stack_8
	 * is set. Should not matter much since we'll use memmove() mostly or only
	 * on pkt data.
	 *
	 * Example with d, s, len = 12 bytes:
	 *   * __bpf_memmove_fwd() emits: mov_32 d[0],s[0]; mov_64 d[4],s[4]
	 *   * __bpf_memmove_bwd() emits: mov_32 d[8],s[8]; mov_64 d[0],s[0]
	 */
	if (d <= s)
		return __bpf_memmove_fwd(d, s, len);
	else
		return __bpf_memmove_bwd(d, s, len);
}

static __always_inline __maybe_unused __nobuiltin("memmove") void bpf_memmove(void *d,
							   const void *s,
							   __u64 len)
{
	return __bpf_memmove(d, s, len);
}

#endif /* __BPF_BUILTINS__ */
