#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <unistd.h>

typedef struct {
    uint32_t x, y, z;
} dim3;

typedef int cudaError_t;
typedef uint64_t cudaStream_t;

cudaError_t cudaLaunchKernel(const void *func, dim3 gridDim, dim3 blockDim, void **args, size_t sharedMem, cudaStream_t stream) {
    return 0;
}

cudaError_t cudaMalloc(void **devPtr, size_t size) {
    *devPtr = (void *)0xdeadbeef;
    return 0;
}

cudaError_t cudaFree(void *devPtr) {
    return 0;
}

cudaError_t cudaStreamSynchronize(cudaStream_t stream) {
    return 0;
}

int main(int argc, char **argv) {
    cudaStream_t stream = 30;

    if (argc < 3) {
        fprintf(stderr, "Usage: %s <wait-to-start-sec> <wait-to-end-sec>\n", argv[0]);
        return 1;
    }

    int waitStart = atoi(argv[1]);
    int waitEnd = atoi(argv[2]);

    fprintf(stderr, "Waiting for %d seconds before starting\n", waitStart);

    // Give time for the eBPF program to load
    sleep(waitStart);

    fprintf(stderr, "Starting calls.\n");

    cudaLaunchKernel((void *)0x1234, (dim3){ 1, 2, 3 }, (dim3){ 4, 5, 6 }, NULL, 10, stream);
    void *ptr;
    cudaMalloc(&ptr, 100);
    cudaFree(ptr);
    cudaStreamSynchronize(stream);

    fprintf(stderr, "CUDA calls made. Waiting for %d seconds before exiting\n", waitEnd);

    // Give time for the agent to inspect this process and check environment variables/etc before this exits
    sleep(waitEnd);

    fprintf(stderr, "Exiting\n");

    return 0;
}
