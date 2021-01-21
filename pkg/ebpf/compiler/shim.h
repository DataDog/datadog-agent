#ifndef __COMPILER_SHIM_H
#define __COMPILER_SHIM_H

#ifdef __cplusplus
extern "C" {
#endif

typedef struct bpf_compiler bpf_compiler;

bpf_compiler *new_bpf_compiler(void);
int bpf_compile_to_object_file(bpf_compiler *, const char *, const char *, const char **, char verbose, char in_memory);
const char * bpf_compiler_get_errors(bpf_compiler *compiler);
void delete_bpf_compiler(bpf_compiler *);

#ifdef __cplusplus
}
#endif

#endif
