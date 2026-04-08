# Go SwissTable Maps: Layout, Hashing, and Lookup

This document describes the internal representation of Go's swisstable-based
maps (introduced in Go 1.24, replacing hmaps) and how dynamic instrumentation
performs O(1) key lookups from eBPF. Understanding this is essential for the
map index expression feature.

## Overview

Go maps since 1.24 use a [SwissTable](https://abseil.io/about/design/swisstables)
design. The key ideas are:

1. **Control bytes** enable 8-way parallel slot matching within a group using
   only bitwise operations.
2. **Quadratic probing** visits every group exactly once.
3. **Extendible hashing** splits tables independently for incremental growth.
4. **Per-map random seeds** prevent hash collision attacks.

A map variable in Go is a pointer to a `Map` struct. A nil map pointer means
the map has never been allocated.

## Memory Layout

### Map Header

The `Map` struct (`internal/runtime/maps/map.go`) is the top-level structure:

```
type Map struct {
    used        uint64          // number of filled slots (all tables combined)
    seed        uintptr         // per-map hash seed, set once at creation
    dirPtr      unsafe.Pointer  // directory of tables or single group
    dirLen      int             // directory length; 0 = small map
    globalDepth uint8           // bits used for table selection
    globalShift uint8           // 64 - globalDepth
    writing     uint8           // race detection flag
    clearSeq    uint64          // clear operation counter
}
```

The `dirPtr` field has dual meaning based on `dirLen`:

- **`dirLen == 0`** (small map, ≤8 entries): `dirPtr` points directly to a
  single `group` struct. No tables, no directory, no probing — just one group
  with up to 8 slots.
- **`dirLen > 0`**: `dirPtr` points to an array of `*table` pointers (the
  directory). Multiple directory entries may point to the same table (due to
  split-growth semantics). `dirLen` is always a power of two equal to
  `1 << globalDepth`.

### Table

Each table (`internal/runtime/maps/table.go`) is a self-contained hash table
servicing a subset of the hash space:

```
type table struct {
    used       uint16          // filled slots in this table
    capacity   uint16          // total slots (always 2^N)
    growthLeft uint16          // remaining slots before rehash
    localDepth uint8           // bits used when this table was created
    index      int             // index in the directory (-1 if stale)
    groups     groupsReference // the groups array
}
```

The `groups` field contains:

```
type groupsReference struct {
    data       unsafe.Pointer  // pointer to array of group structs
    lengthMask uint64          // (number of groups) - 1; for modular arithmetic
}
```

The number of groups is always a power of two, so `lengthMask` enables cheap
modular indexing: `group_index & lengthMask`.

### Group

A group (`internal/runtime/maps/group.go`) holds 8 key/element slots plus a
control word:

```
type group struct {
    ctrls  ctrlGroup                    // 8 control bytes packed in uint64
    slots  [8]struct{ key K; elem V }   // 8 key/element pairs
}
```

The control word is 8 bytes at the start of the group. On little-endian systems,
byte `i` of the uint64 corresponds to slot `i`.

The group size depends on the key and element types:
`groupSize = 8 + 8 * slotSize`, where `slotSize = sizeof(key) + sizeof(elem)`
(with alignment padding as dictated by the Go ABI).

### Slot Layout

Each slot within a group stores one key/element pair:

```
type slot struct {
    key  K   // at offset keyInSlotOffset (usually 0)
    elem V   // at offset elemOff (= keySize, rounded up for alignment)
}
```

The key and element offsets within a slot, and the total slot size, are
type-dependent and obtained from DWARF.

### Control Bytes

Each control byte encodes slot status and a partial hash:

```
empty:    1 0 0 0 0 0 0 0  (0x80)
deleted:  1 1 1 1 1 1 1 0  (0xFE)
full:     0 h h h h h h h  (0x00–0x7F, where h = H2 hash)
```

- Bit 7 clear → slot is **full**, bits 0–6 store H2 (lower 7 bits of hash).
- Bit 7 set → slot is **empty** or **deleted** (distinguished by other bits).

The `matchH2` operation checks all 8 control bytes against an H2 value in
parallel using bitwise arithmetic:

```
v = ctrl_word ^ (0x0101010101010101 * h2)
matches = ((v - 0x0101010101010101) & ~v) & 0x8080808080808080
```

Each matching slot sets bit 7 of its byte in the result. False positives are
possible (~0.7% per slot) but harmless — they trigger a key comparison that
fails.

The `matchEmpty` operation finds empty slots:

```
v = ctrl_word
empty = (v & ~(v << 6)) & 0x8080808080808080
```

## Hash Function

Go's map hash functions are per-type and architecture-dependent. The hash takes
two inputs: a pointer to the key data and a seed (the per-map `Map.seed`).

### Hash Splitting

The 64-bit hash is split into two parts:

```
H1 = hash >> 7     // upper 57 bits: selects the initial group via probing
H2 = hash & 0x7F   // lower 7 bits: stored in control byte for fast matching
```

### Hash Function Dispatch

The compiler selects a hash function for each map key type at compile time,
stored as the `Hasher` function pointer in `abi.SwissMapType`. The selection
is based on `AlgType(keyType)`:

| Key type | AlgType | Runtime hasher | Dispatch |
|----------|---------|----------------|----------|
| bool (1B) | AMEM8 | `memhash8` | → `memhash(p, seed, 1)` |
| int8/uint8 (1B) | AMEM8 | `memhash8` | → `memhash(p, seed, 1)` |
| int16/uint16 (2B) | AMEM16 | `memhash16` | → `memhash(p, seed, 2)` |
| int32/uint32 (4B) | AMEM32 | `memhash32` | **specialized** (not through memhash) |
| int64/uint64/uintptr (8B) | AMEM64 | `memhash64` | **specialized** (not through memhash) |
| string | ASTRING | `strhash` | → `memhash(str.ptr, seed, str.len)` |

This matters because `memhash32` and `memhash64` have their own AES code paths
that differ from the general `memhash`. The 1-byte and 2-byte types delegate
to the general `memhash` which has a different AES seed preparation sequence.

### Algorithm Selection: AES vs Wyhash

At process startup, `runtime.alginit()` checks for hardware AES support:

```go
func alginit() {
    if (GOARCH == "amd64") &&
        cpu.X86.HasAES &&    // AESENC instruction
        cpu.X86.HasSSSE3 &&  // PSHUFB instruction
        cpu.X86.HasSSE41 {   // PINSRD/PINSRQ instructions
        initAlgAES()         // sets useAeshash = true, fills aeskeysched
        return
    }
    if GOARCH == "arm64" && cpu.ARM64.HasAES {
        initAlgAES()
        return
    }
    // Fallback: initialize wyhash keys
    for i := range hashkey {
        hashkey[i] = uintptr(bootstrapRand())
    }
}
```

**When AES is used**: Virtually all amd64 CPUs manufactured since ~2010 have
AES-NI (Intel Westmere, AMD Bulldozer onwards). The only amd64 systems without
it are very old hardware or VMs that explicitly disable AES-NI. For arm64, AES
is part of the ARMv8 base profile and present on all modern ARM64 chips.

**When wyhash is used**: Systems where `useAeshash` remains `false`:
- amd64 without AES-NI, SSSE3, or SSE4.1 (pre-2010 hardware or restricted VMs)
- arm64 without ARM crypto extensions (very rare)
- 32-bit architectures (386, arm) — always use wyhash since `hash64.go` is
  only built for 64-bit targets
- Other architectures (mips64, ppc64, riscv64, wasm, etc.)

**How we detect which to use**: The `runtime.useAeshash` global variable is a
bool at a fixed address visible in DWARF. We read it from the traced process
at BPF program load time. Both hash implementations exist in our BPF code; a
branch on the `swiss_map_hash_kind` volatile const (set by the loader)
selects the correct path.

### Per-Process Hash Secrets

Both algorithms require per-process random secrets initialized at startup.
These secrets are the same for all maps in the process — only the per-map
`seed` (in `Map.seed`) differs between maps.

**AES secret** — `runtime.aeskeysched`:
- Type: `uint8[128]` (128 bytes)
- Filled from `/dev/urandom` via `bootstrapRand()` at startup
- Used as round keys in AESENC operations
- We need bytes 0–47 for memhash32/memhash64, and bytes 0–111 for memhash
  with multi-lane hashing of strings up to 128 bytes

**Wyhash secret** — `runtime.hashkey`:
- Type: `uintptr[4]` (32 bytes on 64-bit)
- Each element filled from `bootstrapRand()` at startup
- Used as XOR/mixing constants:
  - `hashkey[0]`: initial seed scramble
  - `hashkey[1]`: mixed with key data in every hash variant
  - `hashkey[2]`, `hashkey[3]`: used only for long keys (>48 bytes)

### AES-based Hash (amd64 with AES-NI)

**`memhash32`** (4-byte keys: int32, uint32):

```
state[0:8]  = seed
state[8:12] = data     // PINSRD $2
state[12:16] = 0
state = AESENC(state, aeskeysched[0:16])
state = AESENC(state, aeskeysched[16:32])
state = AESENC(state, aeskeysched[32:48])
return state[0:8]
```

Note: the seed and data are combined into a single 128-bit register BEFORE
any AESENC round. The round keys come from `aeskeysched`. This is different
from the general `memhash` path where the seed is scrambled first.

**`memhash64`** (8-byte keys: int64, uint64, uintptr):

```
state[0:8]  = seed
state[8:16] = data     // PINSRQ $1
state = AESENC(state, aeskeysched[0:16])
state = AESENC(state, aeskeysched[16:32])
state = AESENC(state, aeskeysched[32:48])
return state[0:8]
```

Same structure as memhash32 but data fills the upper 64 bits.

**`memhash`** (general, for 1-byte, 2-byte, and 16-byte keys):

The seed preparation is different from memhash32/64:

```
state[0:8]  = seed
state[8:10] = uint16(len)    // PINSRW $4
state[10:16] = replicated len // PSHUFHW $0 repeats the 16-bit len
unscrambled = state           // saved copy for multi-lane seeding
state ^= aeskeysched[0:16]   // XOR (not AESENC!)
state = AESENC(state, state)  // scramble seed (self-keyed round)
```

Then data is mixed in, depending on length:

- **len == 0**: one more `AESENC(state, state)`, return `state[0:8]`.
- **1 ≤ len ≤ 15**: Load 16 bytes at `[data+len-16, data+len)`, mask to `len`
  bytes using a lookup table. `data ^= state; data = AESENC(data, data)` × 3.
  Return `data[0:8]`.
- **len == 16**: `data = load 16 bytes; data ^= state; AESENC(data, data)` × 3.
- **17 ≤ len ≤ 32**: Two 16-byte lanes. Second seed:
  `seed1 = unscrambled ^ aeskeysched[16:32]; seed1 = AESENC(seed1, seed1)`.
  Load first 16 and last 16 bytes (may overlap). Each lane: XOR with its seed,
  `AESENC × 3`. Combine: `result = lane0 ^ lane1`.
- **33 ≤ len ≤ 64**: Four lanes using `aeskeysched[16:48]` for 3 additional seeds.
- **65 ≤ len ≤ 128**: Eight lanes using `aeskeysched[16:112]`.
- **129+ len**: Eight lanes, loop over 128-byte blocks with interleaved
  AESENC scramble + data XOR.

**`strhash`** (string keys):

Extracts `(ptr, len)` from the string header (16 bytes: `{ptr uintptr; len int}`),
then calls the general `memhash(ptr, seed, len)` path above.

For our use case (base type keys 1–8 bytes, string keys up to 512 bytes), we
implement all length tiers:
- `memhash32` AES path: 3 AESENC rounds with `aeskeysched[0:48]`
- `memhash64` AES path: 3 AESENC rounds with `aeskeysched[0:48]`
- `memhash` AES path for 1–2 byte keys: seed scramble + 3 data rounds with
  `aeskeysched[0:16]`
- `strhash` AES path for strings ≤16B: seed scramble + 3 data rounds
- `strhash` AES path for strings 17–32B: 2-lane, using `aeskeysched[0:32]`
- `strhash` AES path for strings 33–64B: 4-lane, using `aeskeysched[0:48]`
- `strhash` AES path for strings 65–128B: 8-lane, using `aeskeysched[0:112]`
- `strhash` AES path for strings 129–512B: 8-lane with block loop

### Wyhash Fallback

When `useAeshash == false`, Go uses a wyhash-inspired algorithm
(`runtime/hash64.go`). The core primitive is:

```
const m5 = 0x1d8e4e27c47d124f

func mix(a, b uint64) uint64 {
    hi, lo = bits.Mul64(a, b)   // full 128-bit multiply
    return hi ^ lo
}
```

The `mix` operation requires 128-bit multiplication. In BPF, this can be done
with `__uint128_t` (supported by clang for BPF targets) or decomposed into
four 64-bit multiplies with carry propagation.

**`memhash32Fallback`** (4-byte keys):

```
a = uint64(readUnaligned32(p))
return mix(m5 ^ 4, mix(a ^ hashkey[1], a ^ seed ^ hashkey[0]))
```

Two nested `mix` calls = two 128-bit multiplies.

**`memhash64Fallback`** (8-byte keys):

```
a = readUnaligned64(p)
return mix(m5 ^ 8, mix(a ^ hashkey[1], a ^ seed ^ hashkey[0]))
```

Same structure as memhash32 but with the full 8 bytes of data.

**`memhashFallback`** (general, for 1-byte, 2-byte keys and strings):

```
seed ^= hashkey[0]
switch {
case len == 0:
    return seed
case len < 4:
    a = uint64(data[0]) | uint64(data[len>>1])<<8 | uint64(data[len-1])<<16
    b = 0  // (falls through to final mix)
case len == 4:
    a = uint64(readUnaligned32(data))
    b = a
case len < 8:
    a = uint64(readUnaligned32(data))
    b = uint64(readUnaligned32(data + len - 4))
case len == 8:
    a = readUnaligned64(data)
    b = a
case len <= 16:
    a = readUnaligned64(data)
    b = readUnaligned64(data + len - 8)
default:
    // Process 16-byte chunks (for strings > 16 bytes)
    for len > 48:
        seed  = mix(r8(p)    ^ hashkey[1], r8(p+8)  ^ seed)
        seed1 = mix(r8(p+16) ^ hashkey[2], r8(p+24) ^ seed1)
        seed2 = mix(r8(p+32) ^ hashkey[3], r8(p+40) ^ seed2)
        p += 48; len -= 48
    seed ^= seed1 ^ seed2
    for len > 16:
        seed = mix(r8(p) ^ hashkey[1], r8(p+8) ^ seed)
        p += 16; len -= 16
    a = r8(p + len - 16)
    b = r8(p + len - 8)
}
return mix(m5 ^ uint64(originalLen), mix(a ^ hashkey[1], b ^ seed))
```

**`strhashFallback`** (string keys): Extracts `(ptr, len)` from the Go string
header, then calls `memhashFallback(ptr, seed, len)`.

For our use case:
- 1-byte keys (bool, int8, uint8): `memhashFallback` with `len < 4` path
- 2-byte keys (int16, uint16): `memhashFallback` with `len < 4` path
- 4-byte keys (int32, uint32): `memhash32Fallback` (specialized)
- 8-byte keys (int64, uint64, uintptr): `memhash64Fallback` (specialized)
- Strings ≤ 8 bytes: `strhashFallback` → `memhashFallback` with appropriate branch
- Strings 9–16 bytes: `memhashFallback` `len <= 16` path
- Strings 17–48 bytes: `memhashFallback` loop with `hashkey[1]` mixing
- Strings 49–255 bytes: `memhashFallback` triple-mixing loop with
  `hashkey[1..3]`

### Memoization of Hash Secrets in BPF

The hash secrets (`aeskeysched`, `hashkey`) and algorithm selector (`useAeshash`)
are per-process constants that never change after `runtime.alginit()`. Rather
than reading them from userspace on every map lookup (which would add 128+ bytes
of `bpf_probe_read_user` per invocation), we memoize them in BPF global variables
on first use.

```c
static uint64_t g_swiss_hash_flags = 0;  // bit 0 = initialized, bit 1 = use_aes
static uint64_t g_swiss_hashkey[4] = {};
static uint8_t g_swiss_aeskeysched[128] = {};
```

The first map index lookup calls `swiss_hash_ensure_initialized()` which reads
from the traced process via `bpf_probe_read_user` and sets the flag. All
subsequent lookups (in the same BPF program lifetime) use the cached values
with zero additional userspace reads.

This also avoids the BPF 512-byte stack limit — storing 128 bytes of
`aeskeysched` on the stack would consume a quarter of the budget.

### DWARF Visibility of Hash Secrets

The hash secrets are visible as DWARF global variables:

```
DW_TAG_variable
    DW_AT_name      "runtime.useAeshash"
    DW_AT_location  DW_OP_addr <addr>
    DW_AT_type      "bool"

DW_TAG_variable
    DW_AT_name      "runtime.aeskeysched"
    DW_AT_location  DW_OP_addr <addr>
    DW_AT_type      "uint8[128]"

DW_TAG_variable
    DW_AT_name      "runtime.hashkey"
    DW_AT_location  DW_OP_addr <addr>
    DW_AT_type      "uintptr[4]"
```

These addresses are extracted during DWARF processing (same pass as
`runtime.firstmoduledata`) and stored in `ir.GoMapHashInfo`. At BPF program
load time, the loader reads the actual secret bytes from the traced process and
populates a BPF map for the hash functions to use.

## Lookup Algorithm

### Table Selection (large maps)

For maps with `dirLen > 0`, the table is selected using the upper bits of the
hash:

```
tableIdx = hash >> globalShift    // globalShift = 64 - globalDepth
tablePtr = directory[tableIdx]    // dirPtr[tableIdx * 8]
```

Multiple directory entries may reference the same table (when `localDepth <
globalDepth`). This is a consequence of split-growth — it doesn't affect
lookup correctness.

### Quadratic Probing

Within a table, the probe sequence is:

```
offset₀ = H1 & lengthMask
offsetₙ = (offsetₙ₋₁ + n) & lengthMask    // n = 1, 2, 3, ...
```

This is triangular-number probing: `p(i) = (i² + i)/2 + H1 (mod groups)`.
It visits every group exactly once when the number of groups is a power of two,
because `(i² + i)/2` is a bijection in `Z/(2^m)`.

The probe sequence terminates when a group contains an empty control byte (0x80),
indicating the key was never inserted past this point.

### Full Lookup Sequence

```
hash = hashFunc(key, map.seed)
h1 = hash >> 7
h2 = hash & 0x7F

if dirLen == 0:
    # Small map: single group at dirPtr
    search one group at dirPtr
else:
    # Large map: directory lookup + probing
    tablePtr = directory[hash >> globalShift]
    groups = tablePtr.groups
    for offset in probeSequence(h1, groups.lengthMask):
        group = groups.data[offset]
        matches = group.ctrls.matchH2(h2)
        for each matching slot:
            if slotKey == key: return slotElem
        if group.ctrls.matchEmpty() != 0: break   # end of probe sequence
    return "not found"
```

## Software AESENC in BPF

BPF cannot execute AES-NI instructions. To compute the same hash as the Go
runtime, we implement `AESENC` in software. Each `AESENC(state, roundKey)`
performs four transformations on the 128-bit state:

### 1. SubBytes

Each of the 16 state bytes is substituted through the AES S-box (a fixed
256-byte lookup table). The S-box is the multiplicative inverse in GF(2⁸)
followed by an affine transformation.

```c
for (int i = 0; i < 16; i++)
    state[i] = AES_SBOX[state[i]];
```

The S-box is stored as a `static const` array in the BPF program.

### 2. ShiftRows

Bytes within each row of the 4×4 state matrix are cyclically shifted:

```
Row 0: no shift        state[0],  state[4],  state[8],  state[12]
Row 1: shift left 1    state[5],  state[9],  state[13], state[1]
Row 2: shift left 2    state[10], state[14], state[2],  state[6]
Row 3: shift left 3    state[15], state[3],  state[7],  state[11]
```

This is a fixed byte permutation — zero runtime cost beyond the index mapping.

### 3. MixColumns

Each 4-byte column is multiplied by a fixed matrix in GF(2⁸):

```
[2 3 1 1] [s0]
[1 2 3 1] [s1]
[1 1 2 3] [s2]
[3 1 1 2] [s3]
```

Multiplication by 2 in GF(2⁸) (`xtime`) is: `(b << 1) ^ (0x1b if b & 0x80)`.
Multiplication by 3 is: `xtime(b) ^ b`.

### 4. AddRoundKey

XOR the 16-byte state with the 16-byte round key.

### Instruction Cost

Each AESENC round costs approximately:
- SubBytes: 16 byte loads from the S-box array
- ShiftRows: 16 byte moves (or fused with SubBytes via index remapping)
- MixColumns: ~64 arithmetic instructions (4 columns × ~16 ops each)
- AddRoundKey: 2 XOR instructions (operating on uint64 pairs)

Total: ~100 BPF instructions per round. For `memhash64` (3 rounds): ~300
instructions. For `strhash` on short strings (4 rounds): ~400 instructions.
This is well within BPF program limits.

## BPF Verification Constraints

### Call Stack Depth Limit

The BPF verifier enforces a maximum call stack depth of **8 frames**. Each
function call (including `bpf_loop` internals and its callback) consumes
frames. The call chain to reach a stack machine opcode handler is already
deep:

```
Frame 1: BPF program entry (uprobe handler)
Frame 2: bpf_loop helper (the outer sm_loop driver)
Frame 3: sm_loop callback
Frame 4: (opcode handler code, typically inlined into sm_loop)
```

We are already at depth 3–4 inside an opcode handler. A `bpf_loop` call from
here consumes 2 more frames (the helper + its callback), bringing us to 5–6.
The callback could call `__always_inline` helpers, but it **cannot** call
another `bpf_loop` — that would need frames 7+8 and leave zero room for any
helper calls inside the inner callback.

**Bottom line**: Inside an opcode handler, we can have at most **one**
`bpf_loop` at the leaf of the call chain. Everything else must be inlined.

### Global Functions for Verification Isolation

Global (non-static) noinline functions are verified **independently** by the
BPF verifier, each with its own verification context. This reduces verifier
complexity and avoids state explosion. However, global functions do **not**
reset the runtime call depth — the actual call stack still accumulates at
runtime.

The benefit of using a global function for the map lookup is purely about
verification: the verifier analyzes `sm_swiss_map_lookup` separately from
`sm_loop`, keeping the verification tractable. At runtime, calling it from
`sm_loop` still uses a call stack frame.

### Practical Structure for Map Lookup

The map lookup is decomposed into 5 sm_loop opcodes that the existing bytecode
interpreter drives via PC replay. No nested `bpf_loop` is used:

```
[SM_OP_SWISS_MAP_SETUP]       reads bytecode params, copies key data, inits hash secrets
[SM_OP_SWISS_MAP_AESENC]      one AESENC round on hash_scratch.state; replays via PC
[SM_OP_SWISS_MAP_HASH_FINISH] AES phase transitions, wyhash, hash finalization
[SM_OP_SWISS_MAP_PROBE]       reads ctrl word, computes H2/empty bitsets
[SM_OP_SWISS_MAP_CHECK_SLOT]  checks one H2-matching slot against the literal key
```

Each opcode is a minimal switch case in `sm_loop`. Complex logic is in global
noinline functions (`sm_swiss_map_setup`, `sm_swiss_map_hash_finish`,
`sm_swiss_map_aesenc`, `swiss_map_check_slot`) that are verified independently.

The AES hash uses multiple phases within HASH_FINISH to handle all length
tiers (1-lane through 8-lane with block loop). Each phase sets up state and
returns to AESENC for rounds, cycling through lanes one at a time.

**String key comparison** uses a bounded `for` loop (up to 512 iterations for
`MaxMapStringKeyLength`), with per-byte `scratch_buf_bounds_check`.

## ARM64 AES Hash (AESE + AESMC)

On arm64, the Go runtime uses ARM crypto instructions (`AESE` + `AESMC`) which
have fundamentally different semantics from x86 `AESENC`:

| Step | x86 AESENC(state, rk) | arm64 AESE(state, rk) + AESMC(state) |
|------|------------------------|---------------------------------------|
| 1 | SubBytes(state) | state XOR rk |
| 2 | ShiftRows | SubBytes |
| 3 | MixColumns | ShiftRows |
| 4 | XOR rk | MixColumns (separate AESMC; omitted on final round) |

The BPF stack machine detects the target architecture via the loader-injected
`is_arm64` volatile const and dispatches to `sm_swiss_map_aese()` instead of
`sm_swiss_map_aesenc()`. The phase state machine in `sm_swiss_map_hash_finish()`
also contains arm64-specific branches.

### Key algorithmic differences (verified from `runtime/asm_arm64.s`)

**memhash32/64**: arm64 uses the SAME round key `aeskeysched[0:16]` for all 3
rounds (x86 uses 3 different keys from `aeskeysched[0:48]`). The final round
on arm64 omits AESMC (no MixColumns).

**Seed scramble**: x86 pre-XORs state with `keysched[0:16]` then does a
self-keyed AESENC. arm64 uses `keysched[0:16]` as the AESE round key directly
(the instruction XORs it before SubBytes internally), followed by AESMC.

**Multi-lane seed derivation**: x86 XORs `unscrambled` with
`keysched[16*n]` then does self-keyed AESENC. arm64 uses the keysched slot
as the AESE state and the scrambled seed as the round key.

**Data rounds (1–128 bytes)**: x86 XORs data with seed then does self-keyed
AESENC. arm64 uses seed as the AESE round key applied to data, with the final
of 3 rounds omitting AESMC.

**129+ block loop**: This is the most structurally different path. On x86, the
accumulators are in `lanes[]` (data XOR'd with seeds initially); the loop does
self-keyed AESENC then data-keyed AESENC. On arm64, the accumulators are in
`seeds[]` (V0–V7); data blocks serve as AESE round keys. The loop does
`AESE(seed, old_data)+AESMC` then `AESE(seed, new_data)+AESMC`. The final
XOR-fold operates on seeds (not lanes), and the final 3 rounds use the
last-loaded data as the round key with the last round omitting AESMC.

## Interaction with Dynamic Instrumentation

### What irgen Extracts from DWARF

The irgen phase extracts all structural information needed for map lookup:

1. **Map header field offsets**: `seed`, `dirPtr`, `dirLen`, `globalShift` —
   obtained by walking the `Map` struct's DWARF members. These offsets can vary
   between Go versions (e.g., if fields are reordered or resized).

2. **Group layout**: control word offset, slots offset, slot size, key/elem
   offsets within each slot — obtained from the group struct type, which is
   reached via: `Map.dirPtr` → `*table` → `table.groups` → `groupsReference.data`
   → `*group`.

3. **Table struct layout**: offset of the `groups` field, and within
   `groupsReference`, offsets of `data` and `lengthMask`.

4. **Key and value types**: extracted from the group's slot struct. The key
   type determines the hash function variant and comparison method.

5. **Hash secret addresses**: `runtime.useAeshash`, `runtime.aeskeysched`,
   `runtime.hashkey` — global variable addresses from DWARF, stored in
   `ir.GoMapHashInfo`.

### What the BPF Program Receives

The compiled stack machine bytecode for a `SwissMapLookupOp` encodes:

- All structural offsets listed above (determined from DWARF)
- The literal key bytes (the compile-time lookup key)
- Key metadata (string vs base type, byte size)
- Value byte size (how many bytes to read on match)

At BPF program load time, the loader additionally provides:

- `swiss_map_hash_kind`: 0 = wyhash, 1 = AES (from `runtime.useAeshash`)
- Hash secret bytes in a BPF array map (from `runtime.aeskeysched` or
  `runtime.hashkey`)

At probe fire time, the BPF code reads the per-map `seed` from the map header
in the traced process's memory, computes the hash, and performs the lookup.

### Supported Key Types

Map index expressions support keys that are:

- **Base types**: `int8`–`int64`, `uint8`–`uint64`, `uintptr`, `bool`.
  Hashed with `memhash32` or `memhash64` depending on size.
- **Strings**: up to `MaxMapStringKeyLength` (512) bytes. Hashed with
  `strhash`. All AES hash length tiers (1-lane through 8-lane with block
  loop) are supported.

Unsupported key types (interfaces, structs, arrays, floats) return a
compile-time error. Float keys are excluded because NaN != NaN semantics
make them unsuitable for literal lookup.

### Error Handling

- **Nil map**: `dirPtr` is 0 → `ExprStatusOOB` (map is nil/empty).
- **Key not found**: probe sequence exhausted → `ExprStatusOOB`.
- **Read failure**: `bpf_probe_read_user` fails (process exited, address
  unmapped) → expression evaluation aborts silently.
- **Old-style hmap**: irgen rejects with a compile-time error ("index not
  supported on old-style maps").

## References

- [SwissTable design (Abseil)](https://abseil.io/about/design/swisstables)
- Go source: `internal/runtime/maps/map.go`, `table.go`, `group.go`
- Go source: `runtime/alg.go` (hash function dispatch)
- Go source: `runtime/hash64.go` (wyhash fallback)
- Go source: `runtime/asm_amd64.s` lines 1205–1565 (AES hash implementation)
- [FIPS 197 — AES specification](https://csrc.nist.gov/pubs/fips/197/final)
  (SubBytes, ShiftRows, MixColumns, AddRoundKey definitions)
