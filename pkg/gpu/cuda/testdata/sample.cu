/* Sample program that does nothing, just build a binary that we will inspect */

#include <stdio.h>

#include <cuda_runtime.h>

__global__ void kernel1(float *A, int n) {
    A[0] = 0;
}

__global__ void kernel2(float *A, int n) {
    __shared__ char globalArray[256];

    for (int i = 0; i < n; i++) {
        globalArray[threadIdx.x] = A[i];
    }

    A[0] = globalArray[threadIdx.x];
}

int main(void) {
    float *h_A = (float *)malloc(sizeof(float));

    float *d_A;
    cudaMalloc((void **)&d_A, 1);

    // clang-format off
    kernel1<<<10, 10>>>(d_A, 100);
    kernel2<<<10, 10>>>(d_A, 100);
    // clang-format on

    printf("Done\n");
    return 0;
}
