// This program prints the UUIDs of all CUDA-visible GPUs.
// It uses the real CUDA runtime library to query device properties.
// This binary should be run using the pkg/gpu/testutil/samplebins.go:RunSample* methods.

#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <cuda_runtime.h>

// Format a CUDA UUID as a string in the standard GPU-xxxx format
void format_uuid(cudaUUID_t *uuid, char *output) {
    unsigned char *bytes = (unsigned char *)uuid->bytes;
    sprintf(output, "GPU-%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
            bytes[0], bytes[1], bytes[2], bytes[3],
            bytes[4], bytes[5],
            bytes[6], bytes[7],
            bytes[8], bytes[9],
            bytes[10], bytes[11], bytes[12], bytes[13], bytes[14], bytes[15]);
}

int main(int argc, char **argv) {
    int deviceCount;
    cudaError_t err;

    // This string is used by PatternScanner to validate a proper start of this sample program
    fprintf(stderr, "Starting GPU UUID printer\n");

    err = cudaGetDeviceCount(&deviceCount);
    if (err != cudaSuccess) {
        fprintf(stderr, "cudaGetDeviceCount failed: %s\n", cudaGetErrorString(err));
        return 1;
    }

    fprintf(stderr, "Found %d CUDA device(s)\n", deviceCount);

    for (int i = 0; i < deviceCount; i++) {
        cudaDeviceProp props;
        err = cudaGetDeviceProperties(&props, i);
        if (err != cudaSuccess) {
            fprintf(stderr, "cudaGetDeviceProperties failed for device %d: %s\n", i, cudaGetErrorString(err));
            continue;
        }

        char uuid_str[64];
        format_uuid(&props.uuid, uuid_str);

        // Print to stdout for parsing, stderr for logging
        printf("%s\n", uuid_str);
        fprintf(stderr, "Device %d: %s (%s)\n", i, props.name, uuid_str);
    }

    // Flush stdout to ensure UUIDs are available for reading
    fflush(stdout);

    // This line is used as a marker by patternScanner to indicate the end of the program
    fprintf(stderr, "GPU UUIDs printed.\n");

    // Keep running so the container stays alive for tests that need to inspect it
    pause();

    return 0;
}
