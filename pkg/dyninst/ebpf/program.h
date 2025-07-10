#ifndef __PROGRAM_H__
#define __PROGRAM_H__

#include "bpf_helpers.h"
#include "types.h"

// Data that programs the stack machine and event processing.

volatile const uint32_t prog_id = 0;

struct {
  __uint(type, BPF_MAP_TYPE_ARRAY);
  __uint(max_entries, 0);
  __type(key, uint32_t);
  __type(value, uint8_t[1]);
} stack_machine_code SEC(".maps");
volatile const uint32_t stack_machine_code_len = 0;
volatile const uint32_t stack_machine_code_max_op = 0;
volatile const uint32_t chase_pointers_entrypoint = 0;

struct {
  __uint(type, BPF_MAP_TYPE_ARRAY);
  __uint(max_entries, 0);
  __type(key, uint32_t);
  __type(value, uint32_t);
} type_ids SEC(".maps");
volatile const uint32_t num_types = 0;

struct {
  __uint(type, BPF_MAP_TYPE_ARRAY);
  __uint(max_entries, 0);
  __type(key, uint32_t);
  __type(value, type_info_t);
} type_info SEC(".maps");

struct {
  __uint(type, BPF_MAP_TYPE_ARRAY);
  __uint(max_entries, 0);
  __type(key, uint32_t);
  __type(value, throttler_params_t);
} throttler_params SEC(".maps");
volatile const uint32_t num_throttlers = 0;

struct {
  __uint(type, BPF_MAP_TYPE_ARRAY);
  __uint(max_entries, 0);
  __type(key, uint32_t);
  __type(value, probe_params_t);
} probe_params SEC(".maps");
volatile const uint32_t num_probe_params = 0;

#endif // __PROGRAM_H__
