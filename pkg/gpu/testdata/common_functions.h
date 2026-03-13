#ifndef COMMON_FUNCTIONS_H
#define COMMON_FUNCTIONS_H

#include <stdint.h>
#include <unistd.h>

// CUDA type definitions
typedef struct {
    uint32_t x, y, z;
} dim3;

typedef int cudaError_t;
typedef uint64_t cudaStream_t;
typedef uint64_t cudaEvent_t;

// CUDA function implementations
cudaError_t cudaLaunchKernel(const void *func, dim3 gridDim, dim3 blockDim, void **args, size_t sharedMem, cudaStream_t stream) {
    (void)func;
    (void)gridDim;
    (void)blockDim;
    (void)args;
    (void)sharedMem;
    (void)stream; // Suppress unused parameter warnings
    return 0;
}

cudaError_t cudaMalloc(void **devPtr, size_t size) {
    (void)size; // Suppress unused parameter warning
    *devPtr = (void *)0xdeadbeef;
    return 0;
}

cudaError_t cudaFree(void *devPtr) {
    (void)devPtr; // Suppress unused parameter warning
    return 0;
}

cudaError_t cudaStreamSynchronize(cudaStream_t stream) {
    (void)stream; // Suppress unused parameter warning
    return 0;
}

cudaError_t cudaSetDevice(int device) {
    (void)device; // Suppress unused parameter warning
    return 0;
}

cudaError_t cudaEventRecord(cudaEvent_t event, cudaStream_t stream) {
    (void)event;
    (void)stream; // Suppress unused parameter warnings
    return 0;
}

cudaError_t cudaEventQuery(cudaEvent_t event) {
    (void)event; // Suppress unused parameter warning
    return 0;
}

cudaError_t cudaEventSynchronize(cudaEvent_t event) {
    (void)event; // Suppress unused parameter warning
    return 0;
}

cudaError_t cudaEventDestroy(cudaEvent_t event) {
    (void)event; // Suppress unused parameter warning
    return 0;
}

cudaError_t cudaMemcpy(void *dst, const void *src, size_t count, int kind) {
    (void)dst;
    (void)src;
    (void)count;
    (void)kind; // Suppress unused parameter warnings
    return 0;
}

cudaError_t cudaDeviceSynchronize() {
    return 0;
}

cudaError_t cuLaunchKernel(const void *func, unsigned int grid_x, unsigned int grid_y, unsigned int grid_z, unsigned int block_x, unsigned int block_y, unsigned int block_z, unsigned int shared_mem, void* stream, void ** kernel_params, void ** extra) {
    (void)func;
    (void)grid_x;
    (void)grid_y;
    (void)grid_z;
    (void)block_x;
    (void)block_y;
    (void)block_z;
    (void)shared_mem;
    (void)stream;
    (void)kernel_params;
    (void)extra;
    return 0;
}

// CUlaunchConfig struct for cuLaunchKernelEx
typedef struct {
    void* attrs;
    unsigned int blockDimX, blockDimY, blockDimZ;
    unsigned int gridDimX, gridDimY, gridDimZ;
    void* hStream;
    unsigned int numAttrs;
    unsigned int sharedMemBytes;
} CUlaunchConfig;

cudaError_t cuLaunchKernelEx(const CUlaunchConfig *config, const void *func, void **kernelParams, void **extra) {
    (void)config;
    (void)func;
    (void)kernelParams;
    (void)extra;
    return 0;
}

cudaError_t cuStreamSynchronize(cudaStream_t stream) {
    (void)stream; // Suppress unused parameter warning
    return 0;
}

#endif // COMMON_FUNCTIONS_H
