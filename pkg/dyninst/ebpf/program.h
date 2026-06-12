#ifndef __PROGRAM_H__
#define __PROGRAM_H__

#include "bpf_helpers.h"
#include "types.h"

// Data that programs the stack machine and event processing.

volatile const uint64_t VARIABLE_runtime_dot_firstmoduledata = 0;
volatile const uint32_t OFFSET_runtime_dot_moduledata__types = 0;
volatile const uint32_t OFFSET_runtime_dot_g__goid = 0;
volatile const uint32_t OFFSET_runtime_dot_g__stack = 0;
volatile const uint32_t OFFSET_runtime_dot_g__m = 0;
volatile const uint32_t OFFSET_runtime_dot_m__curg = 0;
volatile const uint32_t OFFSET_runtime_dot_stack__hi = 0;

// runtime._panic field offsets, used by the runtime.recovery uprobe to
// identify the SP range being unwound and capture the panic value. Zero
// if the binary's DWARF lacks runtime._panic; the loader skips
// attaching the recovery probe in that case.
volatile const uint32_t OFFSET_runtime_dot_g___panic = 0;
volatile const uint32_t OFFSET_runtime_dot__panic__arg = 0;
volatile const uint32_t OFFSET_runtime_dot__panic__startSP = 0;
volatile const uint32_t OFFSET_runtime_dot__panic__sp = 0;
volatile const uint32_t OFFSET_runtime_dot__panic__recovered = 0;
volatile const uint32_t OFFSET_runtime_dot__panic__goexit = 0;

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

// Cumulative per-probe stats. ARRAY (not PERCPU_ARRAY) so we can size
// it per IR probe count and key by probe_id; updates use __sync atomics
// to remain race-free across CPUs. max_entries is set by the loader.
// Declared here (rather than in event.c) so stack_machine.h can update
// the recovery counters without a forward-declaration dance.
struct {
  __uint(type, BPF_MAP_TYPE_ARRAY);
  __uint(max_entries, 0);
  __type(key, uint32_t);
  __type(value, stats_t);
} stats_buf SEC(".maps");

// zero_uint32 is a reusable zero key. Used to address PERCPU_ARRAY maps
// that hold a single per-CPU value (in_progress_calls_buf, the per-CPU
// events scratch buffer) and also to address slot 0 of the shared
// stats_buf ARRAY — the runtime.recovery probe writes its process-wide
// counters into slot 0 regardless of which probe_id the recovery
// firing nominally belongs to. Declared in the shared header so any
// .c/.h file can address such maps without a forward declaration.
static const uint32_t zero_uint32 = 0;

struct {
  __uint(type, BPF_MAP_TYPE_ARRAY);
  __uint(max_entries, 0);
  __type(key, uint32_t);
  __type(value, uint32_t);
} go_runtime_types SEC(".maps");

struct {
  __uint(type, BPF_MAP_TYPE_ARRAY);
  __uint(max_entries, 0);
  __type(key, uint32_t);
  __type(value, uint32_t);
} go_runtime_type_ids SEC(".maps");

// Like go_runtime_type_ids but without pointer-to-pointee dereferencing.
// Used by dict resolution where we need the actual type, not the interface-
// adjusted pointee type.
struct {
  __uint(type, BPF_MAP_TYPE_ARRAY);
  __uint(max_entries, 0);
  __type(key, uint32_t);
  __type(value, uint32_t);
} go_runtime_type_direct_ids SEC(".maps");

volatile const uint32_t num_go_runtime_types = 0;

// IR type id of the synthetic TraceContextType. Set at attach time by the
// loader. Used by SM_OP_GO_CONTEXT_CHAIN_INIT to rewrite the synthetic data
// item header's type field. If 0 (unset) when INIT runs, the data item
// becomes unrecognizable to the decoder; the loader is required to set it
// before attach.
volatile const uint32_t trace_context_type_id = 0;

// Swiss map hash support: addresses of runtime hash globals.
// These are read from userspace via bpf_probe_read_user at probe time.
volatile const uint64_t VARIABLE_runtime_dot_useAeshash = 0;
volatile const uint64_t VARIABLE_runtime_dot_aeskeysched = 0;

// Target binary architecture. Determines which AES instruction semantics
// (x86 AESENC vs arm64 AESE+AESMC) the BPF hash emulation uses.
volatile const uint32_t is_arm64 = 0;

#endif // __PROGRAM_H__
