#ifndef _FILE_CAPABILITIES_TEST_H_
#define _FILE_CAPABILITIES_TEST_H_

#include "baloum.h"

// The Go harness allocates exactly the historical structure's size in the VM
// heap. A read past the 20-byte Linux 4.14 fixture fails, unlike a stack fixture
// whose neighboring bytes could hide an overread.
SEC("test/exec_file_capabilities")
int test_exec_file_capabilities(struct baloum_ctx *ctx) {
    struct file_capabilities_t caps = {.permitted = ~0ULL, .inheritable = ~0ULL, .effective = 1, .available = 1};
    read_exec_file_capabilities((void *)ctx->arg0, ctx->arg1, ctx->arg2, &caps);
    baloum_memcpy((void *)ctx->arg3, &caps, sizeof(caps));
    return 1;
}

#endif /* _FILE_CAPABILITIES_TEST_H_ */
