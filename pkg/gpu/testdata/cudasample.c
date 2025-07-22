// This is a dummy CUDA runtime library that can be used to test the GPU monitoring code without
// having a real CUDA runtime library installed.
// This binary should be run using the pkg/gpu/testutil/samplebins.go:RunSample* methods, which
// call the binary with the correct arguments and environment variables to test the agent.

#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <unistd.h>

typedef struct {
    uint32_t x, y, z;
} dim3;

typedef int cudaError_t;
typedef uint64_t cudaStream_t;
typedef uint64_t cudaEvent_t;

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

cudaError_t cudaSetDevice(int device) {
    return 0;
}

cudaError_t cudaEventRecord(cudaEvent_t event, cudaStream_t stream) {
    return 0;
}

cudaError_t cudaEventQuery(cudaEvent_t event) {
    return 0;
}

cudaError_t cudaEventSynchronize(cudaEvent_t event) {
    return 0;
}

cudaError_t cudaEventDestroy(cudaEvent_t event) {
    return 0;
}

cudaError_t cudaMemcpy(void *dst, const void *src, size_t count, int kind) {
    return 0;
}

int main(int argc, char **argv) {
    cudaStream_t stream = 30;
    cudaEvent_t event = 42;

    if (argc != 3) {
        fprintf(stderr, "Usage: %s <wait-to-start-sec> <device-index>\n", argv[0]);
        return 1;
    }

    int waitStart = atoi(argv[1]);
    int device = atoi(argv[2]);

    // This string is used by PatternScanner to validate a proper start of this sample program inside the container
    fprintf(stderr, "Starting CudaSample program\n");
    fprintf(stderr, "Waiting for %d seconds before starting\n", waitStart);

    // Give time for the eBPF program to load
    sleep(waitStart);

    fprintf(stderr, "Starting calls, will use device index %d\n", device);

    cudaSetDevice(device);
    cudaLaunchKernel((void *)0x1234, (dim3){ 1, 2, 3 }, (dim3){ 4, 5, 6 }, NULL, 10, stream);
    void *ptr;
    cudaMalloc(&ptr, 100);
    cudaFree(ptr);
    cudaStreamSynchronize(stream);

    cudaMemcpy((void *)0x1234, (void *)0x5678, 100, 0); // kind 0 is cudaMemcpyHostToDevice

    cudaEventRecord(event, stream);
    cudaEventQuery(event);
    cudaEventSynchronize(event);

    cudaEventDestroy(event);

    setenv("CUDA_VISIBLE_DEVICES", "42", 1);

    // we don't exit to avoid flakiness when the process is terminated before it was hooked for gpu monitoring
    // the expected usage is to send a kill signal to the process (or stop the container that is running it)

    //this line is used as a market by patternScanner to indicate the end of the program
    fprintf(stderr, "CUDA calls made.\n");
    pause(); // Wait for signal to finish the process

    return 0;
}
