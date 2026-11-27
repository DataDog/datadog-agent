// This is a dummy CUDA runtime library that can be used to test the GPU monitoring code without
// having a real CUDA runtime library installed.
// This binary should be run using the pkg/gpu/testutil/samplebins.go:RunSample* methods, which
// call the binary with the correct arguments and environment variables to test the agent.
// This version calls cudaLaunchKernel at a specified rate per second.

#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <unistd.h>
#include <time.h>
#include "common_functions.h"

int setenv(const char *__name, const char *__value, int __replace) {
    (void)__name;
    (void)__value;
    (void)__replace;
    return 0;
}

int main(int argc, char **argv) {
    cudaStream_t stream = 30;

    if (argc != 5) {
        fprintf(stderr, "Usage: %s <wait-to-start-sec> <device-index> <calls-per-second> <execution-time-sec>\n", argv[0]);
        return 1;
    }

    int waitStart = atoi(argv[1]);
    int device = atoi(argv[2]);
    int callsPerSecond = atoi(argv[3]);
    int executionTimeSec = atoi(argv[4]);

    if (callsPerSecond <= 0) {
        fprintf(stderr, "Error: calls-per-second must be positive\n");
        return 1;
    }

    if (executionTimeSec <= 0) {
        fprintf(stderr, "Error: execution-time-sec must be positive\n");
        return 1;
    }

    // This string is used by PatternScanner to validate a proper start of this sample program inside the container
    fprintf(stderr, "Starting CudaRateSample program\n");
    fprintf(stderr, "Waiting for %d seconds before starting\n", waitStart);
    fprintf(stderr, "Will make %d cudaLaunchKernel calls per second for %d seconds\n", callsPerSecond, executionTimeSec);

    // Give time for the eBPF program to load
    sleep(waitStart);

    fprintf(stderr, "Starting calls, will use device index %d\n", device);

    // Call all
    cudaSetDevice(device);

    // Calculate interval between calls in nanoseconds for better precision
    long interval_ns = 1000000000L / callsPerSecond; // 1 second = 1,000,000,000 nanoseconds

    struct timespec start_time, current_time, last_log_time;
    clock_gettime(CLOCK_MONOTONIC, &start_time);
    last_log_time = start_time;

    long call_count = 0;
    long next_call_time_ns = 0;
    long execution_time_ns = (long)executionTimeSec * 1000000000L; // Convert to nanoseconds

    while (1) {
        clock_gettime(CLOCK_MONOTONIC, &current_time);
        long current_time_ns = (current_time.tv_sec - start_time.tv_sec) * 1000000000L +
                               (current_time.tv_nsec - start_time.tv_nsec);

        // Exit if execution time has been reached
        if (current_time_ns >= execution_time_ns) {
            fprintf(stderr, "Execution time of %d seconds reached\n", executionTimeSec);
            break;
        }

        if (current_time_ns >= next_call_time_ns) {
            cudaLaunchKernel((void *)0x1234, (dim3){ 1, 2, 3 }, (dim3){ 4, 5, 6 }, NULL, 10, stream);
            call_count++;
            next_call_time_ns = current_time_ns + interval_ns;
        } else {
            // For very high rates (>10k), use busy loop for maximum performance
            if (callsPerSecond > 10000) {
                // Busy loop - no sleep for maximum precision
            } else if (callsPerSecond > 1000) {
                struct timespec sleep_time = { 0, 1000 }; // 1 microsecond
                nanosleep(&sleep_time, NULL);
            } else {
                // For lower rates, sleep longer to avoid busy waiting
                usleep(100);
            }
        }

        // Log every second
        long time_since_last_log_ns = (current_time.tv_sec - last_log_time.tv_sec) * 1000000000L +
                                      (current_time.tv_nsec - last_log_time.tv_nsec);
        if (time_since_last_log_ns >= 1000000000L) { // 1 second in nanoseconds
            long calls_per_second_actual = call_count * 1000000000L / time_since_last_log_ns;
            fprintf(stderr, "Made %ld calls in %ld.%03lds (rate: %ld calls/sec)\n",
                call_count,
                time_since_last_log_ns / 1000000000L,
                (time_since_last_log_ns % 1000000000L) / 1000000L,
                calls_per_second_actual);
            last_log_time = current_time;
            call_count = 0; // Reset counter for next second
        }
    }

    fprintf(stderr, "CUDA calls made.\n");

    return 0;
}
