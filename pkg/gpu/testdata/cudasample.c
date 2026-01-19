// This is a dummy CUDA runtime library that can be used to test the GPU monitoring code without
// having a real CUDA runtime library installed.
// This binary should be run using the pkg/gpu/testutil/samplebins.go:RunSample* methods, which
// call the binary with the correct arguments and environment variables to test the agent.

#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <unistd.h>
#include "common_functions.h"

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
    cuLaunchKernel((void *)0x1234, 1, 2, 3, 4, 5, 6, 10, (void*)stream, NULL, NULL);
    CUlaunchConfig launchConfig = { .gridDimX = 1, .gridDimY = 2, .gridDimZ = 3, .blockDimX = 4, .blockDimY = 5, .blockDimZ = 6, .sharedMemBytes = 10, .hStream = (void*)stream };
    cuLaunchKernelEx(&launchConfig, (void *)0x1234, NULL, NULL);
    void *ptr;
    cudaMalloc(&ptr, 100);
    cudaFree(ptr);
    cudaStreamSynchronize(stream);
    cuStreamSynchronize(stream);

    // Sleep for 10ms to ensure that there's time separating the first span and next
    // spans
    usleep(10000);

    cudaMemcpy((void *)0x1234, (void *)0x5678, 100, 0); // kind 0 is cudaMemcpyHostToDevice

    cudaEventRecord(event, stream);
    cudaEventQuery(event);
    cudaEventSynchronize(event);

    cudaEventDestroy(event);

    cudaLaunchKernel((void *)0x1234, (dim3){ 1, 2, 3 }, (dim3){ 4, 5, 6 }, NULL, 10, stream);

    cudaDeviceSynchronize();

    setenv("CUDA_VISIBLE_DEVICES", "42", 1);

    // we don't exit to avoid flakiness when the process is terminated before it was hooked for gpu monitoring
    // the expected usage is to send a kill signal to the process (or stop the container that is running it)

    //this line is used as a marker by patternScanner to indicate the end of the program
    fprintf(stderr, "CUDA calls made.\n");
    pause(); // Wait for signal to finish the process

    return 0;
}
