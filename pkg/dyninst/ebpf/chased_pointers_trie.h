#ifndef __CHASED_POINTERS_TRIE_H__
#define __CHASED_POINTERS_TRIE_H__

// The critbit trie stores sets of typed pointers: 64-bit address + 32-bit
// type_id. Based on the patricia trie or djb's critbit trie but binpacked
// to support up to 2048 entries at a maximum with 4 additional bytes of
// overhead per 12-byte entry.
//
// Example trie storing (0x1000,42), (0x1008,42), (0x2000,17):
//
//                         root=node[0]
//                    +---------------------+
//                    | critbit=12          | (addr differs at bit 12)
//                    | left=node[1]        |
//                    | right=leaf[2]       |
//                    +----------+----------+
//                         bit12=0|1
//               +----------------+---------------------------------+
//               v                                                   v
//        +-------------+                                      +--------------+
//        | node[1]     |                                      | leaf[2]      |
//        | critbit=3   | (addr differs at bit 3)              | (0x2000, 17) |
//        | left/right  |                                      +--------------+
//        +------+------+
//           bit3=0|1
//        +--------+--------+
//        v                 v
//   +--------------+  +--------------+
//   | leaf[0]      |  | leaf[1]      |
//   | (0x1000, 42) |  | (0x1008, 42) |
//   +--------------+  +--------------+
//
// Node encoding (12-bit): bit 11 = leaf flag, bits 0-10 = array index
// Critical bits: bits 0-63 from addr, bits 64-95 from type_id

#ifdef CGO_TEST // used by the Go test suite
#include <stdint.h>
#include <stddef.h>
#include <stdbool.h>
#include <assert.h>
#define barrier_var(x)
#else
#include "bpf_tracing.h"
#include "compiler.h"
#endif

typedef struct chased_pointers_trie_internal_node {
  uint32_t critbit : 8; // bit position (0-95) where children differ
  uint32_t left : 12; // left child node index (+ LEAF_BIT if leaf)
  uint32_t right : 12; // right child node index (+ LEAF_BIT if leaf)
} chased_pointers_trie_internal_node_t;

// Align to 4 bytes.
typedef struct
    __attribute__((aligned(4)))
    __attribute__((packed))
    chased_pointers_trie_leaf_node {
  uint64_t addr; // 64-bit pointer/address
  uint32_t type_id; // 32-bit type identifier
} chased_pointers_trie_leaf_node_t;

// As of writing, this structure is a member in the stack machine, which is
// itself limited to 16KiB, so this needs to be less than that.
//
// The memory usage of this structure is 12 bytes per leaf, plues 4 bytes
// per internal node, plus 4 bytes for the root metadata. There's always
// N-1 internal nodes so the structure uses exactly 16 bytes per entry.
#define CPT_MEMORY_SIZE (16 << 10) // 16KiB
#define CPT_OVERHEAD_PER_ENTRY (sizeof(chased_pointers_trie_internal_node_t) + sizeof(chased_pointers_trie_leaf_node_t))
#define CPT_NUM_NODES (CPT_MEMORY_SIZE / CPT_OVERHEAD_PER_ENTRY)
#define CPT_NUM_INTERNAL_NODES (CPT_NUM_NODES - 1)

// CHASED_POINTERS_TRIE_NULL_NODE is a magic value that indicates that the root
// node is not set.
#define CPT_NULL_NODE 0xFFFF

typedef struct chased_pointers_trie {
  uint16_t len; // number of entries currently stored
  uint16_t root; // root node index (or NULL_NODE if empty)
  chased_pointers_trie_internal_node_t nodes[CPT_NUM_INTERNAL_NODES];
  chased_pointers_trie_leaf_node_t leaves[CPT_NUM_NODES];
} chased_pointers_trie_t;

// Unfortunately we can't use the static_assert macro in eBPF because
// it isn't supported by the compiler we're using there so we instead create
// a program that we know will fail to verify if the properties are not met.
#ifndef CGO_TEST
#define static_assert(cond, msg) \
  if (!(cond)) {                 \
    bpf_printk(msg);             \
    while (1) {                  \
    }                            \
  }
void static_assert_properties(void) {
#endif
  static_assert(
      sizeof(chased_pointers_trie_internal_node_t) == 4,
      "chased_pointers_trie_internal_node_t must be 4 bytes");
  static_assert(
      sizeof(chased_pointers_trie_leaf_node_t) == 12,
      "chased_pointers_trie_leaf_node_t must be 12 bytes");
  static_assert(
      CPT_OVERHEAD_PER_ENTRY == 16,
      "CPT_OVERHEAD_PER_ENTRY must be 16 bytes");
  static_assert(
      sizeof(chased_pointers_trie_t) == CPT_NUM_NODES * CPT_OVERHEAD_PER_ENTRY,
      "chased_pointers_trie_t must be "
      "CPT_NUM_NODES * CPT_OVERHEAD_PER_ENTRY bytes");
#ifndef CGO_TEST
}
#else
void static_assert_properties(void) {
}
#endif

// bit 11: distinguishes leaf (1) from internal (0).
#define CPT_LEAF_BIT 0x800

// the 12th bit is set.
#define CPT_IS_LEAF(node) ((node) & CPT_LEAF_BIT)

// Mask out leaf bit.
#define CPT_NODE_MASK 0x7FF

void chased_pointers_trie_init(chased_pointers_trie_t* trie) {
  trie->len = 0;
  trie->root = CPT_NULL_NODE;

  static_assert_properties();
}

// Count leading zeros in 32-bit value.
__attribute__((always_inline)) static inline uint8_t clz32(uint32_t x) {
  uint8_t n = 32;
  if (x) {
    n = 0;
    if (!(x & 0xFFFF0000)) {
      n += 16;
      x <<= 16;
    }
    if (!(x & 0xFF000000)) {
      n += 8;
      x <<= 8;
    }
    if (!(x & 0xF0000000)) {
      n += 4;
      x <<= 4;
    }
    if (!(x & 0xC0000000)) {
      n += 2;
      x <<= 2;
    }
    if (!(x & 0x80000000)) {
      n += 1;
    }
  }
  return n;
}

// Count leading zeros in 64-bit value.
__attribute__((always_inline)) static inline uint8_t clz64(uint64_t x) {
  uint8_t n = 64;
  if (x) {
    n = 0;
    if (x & 0xFFFFFFFF00000000) {
      x >>= 32;
    } else {
      n += 32;
    }
    n += clz32((uint32_t)(0xFFFFFFFF & x));
  }
  return n;
}

typedef enum chased_pointers_trie_insert_result {
  CHASED_POINTERS_TRIE_ALREADY_EXISTS = 0,
  CHASED_POINTERS_TRIE_INSERTED = 1,
  CHASED_POINTERS_TRIE_FULL = 2,
  CHASED_POINTERS_TRIE_NULL = 3,
  CHASED_POINTERS_TRIE_ERROR = 4,
} chased_pointers_trie_insert_result_t;

// Insert (addr, type_id) pair into trie.
chased_pointers_trie_insert_result_t
chased_pointers_trie_insert(
    chased_pointers_trie_t* trie, uint64_t addr, uint32_t type_id) {
  if (!trie)
    return CHASED_POINTERS_TRIE_NULL;

  // First insertion creates root leaf.
  if (trie->len == 0) {
    trie->leaves[0].addr = addr;
    trie->leaves[0].type_id = type_id;
    trie->len = 1;
    trie->root = 0 | CPT_LEAF_BIT; // Set root to point to first leaf
    return CHASED_POINTERS_TRIE_INSERTED;
  }

  // Traverse until we reach a leaf.
  uint32_t node = trie->root, parent = CPT_NULL_NODE;
  uint8_t dir = 0;
  // TODO: This 96 could be lowered because we don't really use all the bits
  // in the type_id. In practice this loop doesn't seem to use that many
  // bpf instructions.
  for (int depth = 0; depth < 96; depth++) { // ebpf friendly loop bound
    if (CPT_IS_LEAF(node)) {
      break;
    }
    uint64_t node_idx = node & CPT_NODE_MASK;
    if (node_idx >= CPT_NUM_NODES) {
      return CHASED_POINTERS_TRIE_ERROR;
    }

    chased_pointers_trie_internal_node_t* internal_node = &trie->nodes[node_idx];
    uint32_t crit = internal_node->critbit;

    uint32_t bit = 0;
    if (crit >= 64) { // bit in type_id
      bit = (type_id >> (crit - 64)) & 1;
    } else { // bit in addr
      bit = (addr >> crit) & 1;
    }

    parent = node;
    dir = bit;
    node = bit ? internal_node->right : internal_node->left;
  }

  // Find critical bit between new key and existing leaf.
  uint8_t crit_bit;
  uint64_t node_idx = (uint64_t)(node & CPT_NODE_MASK);
  barrier_var(node_idx);
  if (node_idx >= CPT_NUM_NODES) {
    return CHASED_POINTERS_TRIE_ERROR;
  }
  chased_pointers_trie_leaf_node_t* leaf = &trie->leaves[node_idx];

  // XOR to find first differing bit.
  uint64_t diff_addr = leaf->addr ^ addr;
  uint32_t diff_type_id = leaf->type_id ^ type_id;

  if (diff_addr) {
    crit_bit = 63 - clz64(diff_addr);
  } else if (diff_type_id) {
    crit_bit = 64 + 31 - clz32(diff_type_id);
  } else {
    return CHASED_POINTERS_TRIE_ALREADY_EXISTS; // keys are identical
  }

  // Determine direction for new key at critical bit.
  int newdir = 0;
  if (crit_bit >= 64) {
    newdir = (type_id >> (crit_bit - 64)) & 1;
  } else {
    newdir = (addr >> crit_bit) & 1;
  }

  // Create new internal node.
  uint64_t new_internal = (uint64_t)(trie->len - 1);
  barrier_var(new_internal);
  if (new_internal >= CPT_NUM_NODES - 1) {
    return CHASED_POINTERS_TRIE_FULL;
  }
  chased_pointers_trie_internal_node_t* new_internal_node = &trie->nodes[new_internal];
  uint64_t new_leaf_idx = (uint64_t)(trie->len);
  *new_internal_node = (chased_pointers_trie_internal_node_t){
      .critbit = crit_bit,
      .left = newdir ? node : (new_leaf_idx | CPT_LEAF_BIT),
      .right = newdir ? (new_leaf_idx | CPT_LEAF_BIT) : node,
  };

  // Add new leaf.
  if (new_leaf_idx >= CPT_NUM_NODES) {
    return CHASED_POINTERS_TRIE_ERROR;
  }
  trie->leaves[new_leaf_idx] = (chased_pointers_trie_leaf_node_t){
      .addr = addr,
      .type_id = type_id,
  };
  trie->len++;

  // Update parent to point to new internal node.
  if (parent == CPT_NULL_NODE) {
    trie->root = new_internal;
  } else if (dir) {
    trie->nodes[parent & CPT_NODE_MASK].right = new_internal;
  } else {
    trie->nodes[parent & CPT_NODE_MASK].left = new_internal;
  }
  return CHASED_POINTERS_TRIE_INSERTED;
}

void chased_pointers_trie_clear(chased_pointers_trie_t* trie) {
  trie->len = 0;
  trie->root = 0;
}

#ifdef CGO_TEST
static int chased_pointers_trie_lookup(
    chased_pointers_trie_t* trie, uint64_t addr, uint32_t type_id) {
  if (!trie || trie->len == 0)
    return 0;

  uint32_t node = trie->root;
  for (int depth = 0; depth < 96; depth++) { // ebpf friendly loop bound
    if (CPT_IS_LEAF(node)) {
      break;
    }

    chased_pointers_trie_internal_node_t* internal_node =
        &trie->nodes[node & CPT_NODE_MASK];
    uint32_t crit = internal_node->critbit;

    uint32_t bit = 0;
    if (crit >= 64) { // bit in type_id
      bit = (type_id >> (crit - 64)) & 1;
    } else { // bit in addr
      bit = (addr >> crit) & 1;
    }

    node = bit ? internal_node->right : internal_node->left;
  }

  if (!CPT_IS_LEAF(node)) {
    return 0;
  }

  chased_pointers_trie_leaf_node_t* leaf = &trie->leaves[node & CPT_NODE_MASK];
  bool result = (leaf->addr == addr) && (leaf->type_id == type_id);
  return result;
}

static uint16_t chased_pointers_trie_len(chased_pointers_trie_t* trie) {
  return trie->len;
}
#endif

#endif // __CHASED_POINTERS_TRIE_H__
