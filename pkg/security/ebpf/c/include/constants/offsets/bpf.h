#ifndef _CONSTANTS_OFFSETS_BPF_H_
#define _CONSTANTS_OFFSETS_BPF_H_

#include "constants/macros.h"

#define CHECK_HELPER_CALL_FUNC_ID 1
#define CHECK_HELPER_CALL_INSN 2

u64 __attribute__((always_inline)) get_check_helper_call_input(void) {
    u64 input;
    LOAD_CONSTANT("check_helper_call_input", input);
    return input;
}

u64 __attribute__((always_inline)) get_bpf_map_id_offset(void) {
    u64 bpf_map_id_offset;
    LOAD_CONSTANT("bpf_map_id_offset", bpf_map_id_offset);
    return bpf_map_id_offset;
}

u64 __attribute__((always_inline)) get_bpf_map_name_offset(void) {
    u64 bpf_map_name_offset;
    LOAD_CONSTANT("bpf_map_name_offset", bpf_map_name_offset);
    return bpf_map_name_offset;
}

u64 __attribute__((always_inline)) get_bpf_map_type_offset(void) {
    u64 bpf_map_type_offset;
    LOAD_CONSTANT("bpf_map_type_offset", bpf_map_type_offset);
    return bpf_map_type_offset;
}

u64 __attribute__((always_inline)) get_bpf_prog_aux_offset(void) {
    u64 bpf_prog_aux_offset;
    LOAD_CONSTANT("bpf_prog_aux_offset", bpf_prog_aux_offset);
    return bpf_prog_aux_offset;
}

u64 __attribute__((always_inline)) get_bpf_prog_aux_id_offset(void) {
    u64 bpf_prog_aux_id_offset;
    LOAD_CONSTANT("bpf_prog_aux_id_offset", bpf_prog_aux_id_offset);
    return bpf_prog_aux_id_offset;
}

u64 __attribute__((always_inline)) get_bpf_prog_type_offset(void) {
    u64 bpf_prog_type_offset;
    LOAD_CONSTANT("bpf_prog_type_offset", bpf_prog_type_offset);
    return bpf_prog_type_offset;
}

u64 __attribute__((always_inline)) get_bpf_prog_attach_type_offset(void) {
    u64 bpf_prog_attach_type_offset;
    LOAD_CONSTANT("bpf_prog_attach_type_offset", bpf_prog_attach_type_offset);
    return bpf_prog_attach_type_offset;
}

u64 __attribute__((always_inline)) get_bpf_prog_aux_name_offset(void) {
    u64 bpf_prog_aux_name_offset;
    LOAD_CONSTANT("bpf_prog_aux_name_offset", bpf_prog_aux_name_offset);
    return bpf_prog_aux_name_offset;
}

u64 __attribute__((always_inline)) get_bpf_prog_tag_offset(void) {
    u64 bpf_prog_tag_offset;
    LOAD_CONSTANT("bpf_prog_tag_offset", bpf_prog_tag_offset);
    return bpf_prog_tag_offset;
}

#endif
