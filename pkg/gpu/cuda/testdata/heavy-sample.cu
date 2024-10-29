/* AUTO-GENERATED, DO NOT CHANGE */

// To regenerate, run `make heavy-sample.cu` in this directory

#include <stdio.h>

#include <cuda_runtime.h>


__global__ void kernel_0(float * var_0_0, float * var_0_1, float * var_0_2, float * var_0_3, float * var_0_4, float * var_0_5, float * var_0_6, float * var_0_7, float * var_0_8, float * var_0_9) {
	__shared__ float myVar[1024];
	myVar[0] = 48.682422 * myVar[threadIdx.x];
	myVar[6] = 12.630602 * myVar[threadIdx.x];
	myVar[1] = 28.516594 * myVar[threadIdx.x];
	myVar[8] = 21.327449 * myVar[threadIdx.x];
	myVar[2] = 11.235985 * myVar[threadIdx.x];
	myVar[6] = 9.634747 * myVar[threadIdx.x];
	myVar[0] = 6.104667 * myVar[threadIdx.x];
	myVar[0] = 19.813955 * myVar[threadIdx.x];
	myVar[2] = 47.226601 * myVar[threadIdx.x];
	myVar[6] = 38.647377 * myVar[threadIdx.x];
	var_0_0[0] = myVar[0];
	var_0_1[1] = myVar[1];
	var_0_2[2] = myVar[2];
	var_0_3[3] = myVar[3];
	var_0_4[4] = myVar[4];
	var_0_5[5] = myVar[5];
	var_0_6[6] = myVar[6];
	var_0_7[7] = myVar[7];
	var_0_8[8] = myVar[8];
	var_0_9[9] = myVar[9];
	
}

__global__ void kernel_1(float * var_1_0, float * var_1_1, float * var_1_2, float * var_1_3, float * var_1_4, float * var_1_5, float * var_1_6, float * var_1_7, float * var_1_8, float * var_1_9) {
	__shared__ float myVar[1024];
	myVar[7] = 10.498290 * myVar[threadIdx.x];
	myVar[9] = 48.573285 * myVar[threadIdx.x];
	myVar[7] = 40.453661 * myVar[threadIdx.x];
	myVar[9] = 39.308308 * myVar[threadIdx.x];
	myVar[5] = 45.143141 * myVar[threadIdx.x];
	myVar[9] = 21.797042 * myVar[threadIdx.x];
	myVar[4] = 41.584658 * myVar[threadIdx.x];
	myVar[7] = 20.129875 * myVar[threadIdx.x];
	myVar[0] = 3.211955 * myVar[threadIdx.x];
	myVar[5] = 27.658041 * myVar[threadIdx.x];
	var_1_0[0] = myVar[0];
	var_1_1[1] = myVar[1];
	var_1_2[2] = myVar[2];
	var_1_3[3] = myVar[3];
	var_1_4[4] = myVar[4];
	var_1_5[5] = myVar[5];
	var_1_6[6] = myVar[6];
	var_1_7[7] = myVar[7];
	var_1_8[8] = myVar[8];
	var_1_9[9] = myVar[9];
	
}

__global__ void kernel_2(float * var_2_0, float * var_2_1, float * var_2_2, float * var_2_3, float * var_2_4, float * var_2_5, float * var_2_6, float * var_2_7, float * var_2_8, float * var_2_9) {
	__shared__ float myVar[1024];
	myVar[6] = 0.409518 * myVar[threadIdx.x];
	myVar[4] = 8.690601 * myVar[threadIdx.x];
	myVar[9] = 38.152384 * myVar[threadIdx.x];
	myVar[3] = 47.490931 * myVar[threadIdx.x];
	myVar[0] = 43.900222 * myVar[threadIdx.x];
	myVar[3] = 44.705960 * myVar[threadIdx.x];
	myVar[1] = 32.840704 * myVar[threadIdx.x];
	myVar[1] = 34.670584 * myVar[threadIdx.x];
	myVar[6] = 0.856672 * myVar[threadIdx.x];
	myVar[6] = 13.523540 * myVar[threadIdx.x];
	var_2_0[0] = myVar[0];
	var_2_1[1] = myVar[1];
	var_2_2[2] = myVar[2];
	var_2_3[3] = myVar[3];
	var_2_4[4] = myVar[4];
	var_2_5[5] = myVar[5];
	var_2_6[6] = myVar[6];
	var_2_7[7] = myVar[7];
	var_2_8[8] = myVar[8];
	var_2_9[9] = myVar[9];
	
}

__global__ void kernel_3(float * var_3_0, float * var_3_1, float * var_3_2, float * var_3_3, float * var_3_4, float * var_3_5, float * var_3_6, float * var_3_7, float * var_3_8, float * var_3_9) {
	__shared__ float myVar[1024];
	myVar[0] = 25.753676 * myVar[threadIdx.x];
	myVar[7] = 37.243147 * myVar[threadIdx.x];
	myVar[1] = 39.496028 * myVar[threadIdx.x];
	myVar[8] = 40.283404 * myVar[threadIdx.x];
	myVar[8] = 9.798944 * myVar[threadIdx.x];
	myVar[0] = 9.330874 * myVar[threadIdx.x];
	myVar[3] = 12.683786 * myVar[threadIdx.x];
	myVar[7] = 16.171586 * myVar[threadIdx.x];
	myVar[4] = 44.389549 * myVar[threadIdx.x];
	myVar[3] = 13.795409 * myVar[threadIdx.x];
	var_3_0[0] = myVar[0];
	var_3_1[1] = myVar[1];
	var_3_2[2] = myVar[2];
	var_3_3[3] = myVar[3];
	var_3_4[4] = myVar[4];
	var_3_5[5] = myVar[5];
	var_3_6[6] = myVar[6];
	var_3_7[7] = myVar[7];
	var_3_8[8] = myVar[8];
	var_3_9[9] = myVar[9];
	
}

__global__ void kernel_4(float * var_4_0, float * var_4_1, float * var_4_2, float * var_4_3, float * var_4_4, float * var_4_5, float * var_4_6, float * var_4_7, float * var_4_8, float * var_4_9) {
	__shared__ float myVar[1024];
	myVar[6] = 31.317364 * myVar[threadIdx.x];
	myVar[9] = 29.208017 * myVar[threadIdx.x];
	myVar[3] = 49.461131 * myVar[threadIdx.x];
	myVar[5] = 20.083331 * myVar[threadIdx.x];
	myVar[4] = 47.012036 * myVar[threadIdx.x];
	myVar[1] = 20.070240 * myVar[threadIdx.x];
	myVar[3] = 28.171519 * myVar[threadIdx.x];
	myVar[6] = 39.276712 * myVar[threadIdx.x];
	myVar[6] = 11.861790 * myVar[threadIdx.x];
	myVar[2] = 15.870188 * myVar[threadIdx.x];
	var_4_0[0] = myVar[0];
	var_4_1[1] = myVar[1];
	var_4_2[2] = myVar[2];
	var_4_3[3] = myVar[3];
	var_4_4[4] = myVar[4];
	var_4_5[5] = myVar[5];
	var_4_6[6] = myVar[6];
	var_4_7[7] = myVar[7];
	var_4_8[8] = myVar[8];
	var_4_9[9] = myVar[9];
	
}

__global__ void kernel_5(float * var_5_0, float * var_5_1, float * var_5_2, float * var_5_3, float * var_5_4, float * var_5_5, float * var_5_6, float * var_5_7, float * var_5_8, float * var_5_9) {
	__shared__ float myVar[1024];
	myVar[6] = 19.442937 * myVar[threadIdx.x];
	myVar[1] = 22.960740 * myVar[threadIdx.x];
	myVar[3] = 25.491718 * myVar[threadIdx.x];
	myVar[9] = 30.849896 * myVar[threadIdx.x];
	myVar[2] = 49.195293 * myVar[threadIdx.x];
	myVar[5] = 36.407166 * myVar[threadIdx.x];
	myVar[6] = 49.075702 * myVar[threadIdx.x];
	myVar[0] = 22.936021 * myVar[threadIdx.x];
	myVar[4] = 38.690914 * myVar[threadIdx.x];
	myVar[0] = 25.462527 * myVar[threadIdx.x];
	var_5_0[0] = myVar[0];
	var_5_1[1] = myVar[1];
	var_5_2[2] = myVar[2];
	var_5_3[3] = myVar[3];
	var_5_4[4] = myVar[4];
	var_5_5[5] = myVar[5];
	var_5_6[6] = myVar[6];
	var_5_7[7] = myVar[7];
	var_5_8[8] = myVar[8];
	var_5_9[9] = myVar[9];
	
}

__global__ void kernel_6(float * var_6_0, float * var_6_1, float * var_6_2, float * var_6_3, float * var_6_4, float * var_6_5, float * var_6_6, float * var_6_7, float * var_6_8, float * var_6_9) {
	__shared__ float myVar[1024];
	myVar[8] = 40.910830 * myVar[threadIdx.x];
	myVar[3] = 44.110014 * myVar[threadIdx.x];
	myVar[1] = 21.377741 * myVar[threadIdx.x];
	myVar[2] = 22.050224 * myVar[threadIdx.x];
	myVar[3] = 44.650340 * myVar[threadIdx.x];
	myVar[9] = 44.102511 * myVar[threadIdx.x];
	myVar[7] = 5.207397 * myVar[threadIdx.x];
	myVar[7] = 36.209409 * myVar[threadIdx.x];
	myVar[9] = 44.929571 * myVar[threadIdx.x];
	myVar[8] = 49.088663 * myVar[threadIdx.x];
	var_6_0[0] = myVar[0];
	var_6_1[1] = myVar[1];
	var_6_2[2] = myVar[2];
	var_6_3[3] = myVar[3];
	var_6_4[4] = myVar[4];
	var_6_5[5] = myVar[5];
	var_6_6[6] = myVar[6];
	var_6_7[7] = myVar[7];
	var_6_8[8] = myVar[8];
	var_6_9[9] = myVar[9];
	
}

__global__ void kernel_7(float * var_7_0, float * var_7_1, float * var_7_2, float * var_7_3, float * var_7_4, float * var_7_5, float * var_7_6, float * var_7_7, float * var_7_8, float * var_7_9) {
	__shared__ float myVar[1024];
	myVar[8] = 28.669346 * myVar[threadIdx.x];
	myVar[0] = 39.255807 * myVar[threadIdx.x];
	myVar[9] = 29.531385 * myVar[threadIdx.x];
	myVar[5] = 30.978964 * myVar[threadIdx.x];
	myVar[2] = 12.881451 * myVar[threadIdx.x];
	myVar[1] = 31.567788 * myVar[threadIdx.x];
	myVar[0] = 15.197734 * myVar[threadIdx.x];
	myVar[4] = 49.744884 * myVar[threadIdx.x];
	myVar[0] = 36.741280 * myVar[threadIdx.x];
	myVar[3] = 12.612324 * myVar[threadIdx.x];
	var_7_0[0] = myVar[0];
	var_7_1[1] = myVar[1];
	var_7_2[2] = myVar[2];
	var_7_3[3] = myVar[3];
	var_7_4[4] = myVar[4];
	var_7_5[5] = myVar[5];
	var_7_6[6] = myVar[6];
	var_7_7[7] = myVar[7];
	var_7_8[8] = myVar[8];
	var_7_9[9] = myVar[9];
	
}

__global__ void kernel_8(float * var_8_0, float * var_8_1, float * var_8_2, float * var_8_3, float * var_8_4, float * var_8_5, float * var_8_6, float * var_8_7, float * var_8_8, float * var_8_9) {
	__shared__ float myVar[1024];
	myVar[4] = 35.305747 * myVar[threadIdx.x];
	myVar[3] = 22.985684 * myVar[threadIdx.x];
	myVar[3] = 41.990318 * myVar[threadIdx.x];
	myVar[5] = 6.845127 * myVar[threadIdx.x];
	myVar[3] = 28.615930 * myVar[threadIdx.x];
	myVar[5] = 37.194092 * myVar[threadIdx.x];
	myVar[5] = 49.266076 * myVar[threadIdx.x];
	myVar[7] = 33.359126 * myVar[threadIdx.x];
	myVar[9] = 16.949092 * myVar[threadIdx.x];
	myVar[1] = 35.605362 * myVar[threadIdx.x];
	var_8_0[0] = myVar[0];
	var_8_1[1] = myVar[1];
	var_8_2[2] = myVar[2];
	var_8_3[3] = myVar[3];
	var_8_4[4] = myVar[4];
	var_8_5[5] = myVar[5];
	var_8_6[6] = myVar[6];
	var_8_7[7] = myVar[7];
	var_8_8[8] = myVar[8];
	var_8_9[9] = myVar[9];
	
}

__global__ void kernel_9(float * var_9_0, float * var_9_1, float * var_9_2, float * var_9_3, float * var_9_4, float * var_9_5, float * var_9_6, float * var_9_7, float * var_9_8, float * var_9_9) {
	__shared__ float myVar[1024];
	myVar[6] = 6.178335 * myVar[threadIdx.x];
	myVar[8] = 16.840668 * myVar[threadIdx.x];
	myVar[1] = 43.176823 * myVar[threadIdx.x];
	myVar[3] = 31.393618 * myVar[threadIdx.x];
	myVar[4] = 44.561645 * myVar[threadIdx.x];
	myVar[8] = 0.559624 * myVar[threadIdx.x];
	myVar[1] = 14.828130 * myVar[threadIdx.x];
	myVar[9] = 4.181657 * myVar[threadIdx.x];
	myVar[1] = 8.504773 * myVar[threadIdx.x];
	myVar[5] = 13.515462 * myVar[threadIdx.x];
	var_9_0[0] = myVar[0];
	var_9_1[1] = myVar[1];
	var_9_2[2] = myVar[2];
	var_9_3[3] = myVar[3];
	var_9_4[4] = myVar[4];
	var_9_5[5] = myVar[5];
	var_9_6[6] = myVar[6];
	var_9_7[7] = myVar[7];
	var_9_8[8] = myVar[8];
	var_9_9[9] = myVar[9];
	
}

__global__ void kernel_10(float * var_10_0, float * var_10_1, float * var_10_2, float * var_10_3, float * var_10_4, float * var_10_5, float * var_10_6, float * var_10_7, float * var_10_8, float * var_10_9) {
	__shared__ float myVar[1024];
	myVar[8] = 7.882892 * myVar[threadIdx.x];
	myVar[2] = 34.916764 * myVar[threadIdx.x];
	myVar[5] = 0.152698 * myVar[threadIdx.x];
	myVar[7] = 29.688767 * myVar[threadIdx.x];
	myVar[1] = 41.704095 * myVar[threadIdx.x];
	myVar[7] = 36.150542 * myVar[threadIdx.x];
	myVar[6] = 5.765721 * myVar[threadIdx.x];
	myVar[2] = 48.071225 * myVar[threadIdx.x];
	myVar[5] = 48.565510 * myVar[threadIdx.x];
	myVar[0] = 3.512245 * myVar[threadIdx.x];
	var_10_0[0] = myVar[0];
	var_10_1[1] = myVar[1];
	var_10_2[2] = myVar[2];
	var_10_3[3] = myVar[3];
	var_10_4[4] = myVar[4];
	var_10_5[5] = myVar[5];
	var_10_6[6] = myVar[6];
	var_10_7[7] = myVar[7];
	var_10_8[8] = myVar[8];
	var_10_9[9] = myVar[9];
	
}

__global__ void kernel_11(float * var_11_0, float * var_11_1, float * var_11_2, float * var_11_3, float * var_11_4, float * var_11_5, float * var_11_6, float * var_11_7, float * var_11_8, float * var_11_9) {
	__shared__ float myVar[1024];
	myVar[9] = 4.789338 * myVar[threadIdx.x];
	myVar[4] = 36.930728 * myVar[threadIdx.x];
	myVar[9] = 16.710979 * myVar[threadIdx.x];
	myVar[9] = 20.257562 * myVar[threadIdx.x];
	myVar[7] = 40.874004 * myVar[threadIdx.x];
	myVar[7] = 19.896021 * myVar[threadIdx.x];
	myVar[4] = 42.319657 * myVar[threadIdx.x];
	myVar[0] = 44.471235 * myVar[threadIdx.x];
	myVar[1] = 15.504836 * myVar[threadIdx.x];
	myVar[2] = 2.378930 * myVar[threadIdx.x];
	var_11_0[0] = myVar[0];
	var_11_1[1] = myVar[1];
	var_11_2[2] = myVar[2];
	var_11_3[3] = myVar[3];
	var_11_4[4] = myVar[4];
	var_11_5[5] = myVar[5];
	var_11_6[6] = myVar[6];
	var_11_7[7] = myVar[7];
	var_11_8[8] = myVar[8];
	var_11_9[9] = myVar[9];
	
}

__global__ void kernel_12(float * var_12_0, float * var_12_1, float * var_12_2, float * var_12_3, float * var_12_4, float * var_12_5, float * var_12_6, float * var_12_7, float * var_12_8, float * var_12_9) {
	__shared__ float myVar[1024];
	myVar[3] = 33.044254 * myVar[threadIdx.x];
	myVar[8] = 36.172918 * myVar[threadIdx.x];
	myVar[9] = 22.500201 * myVar[threadIdx.x];
	myVar[9] = 42.259863 * myVar[threadIdx.x];
	myVar[0] = 25.286195 * myVar[threadIdx.x];
	myVar[2] = 18.583546 * myVar[threadIdx.x];
	myVar[4] = 37.845654 * myVar[threadIdx.x];
	myVar[6] = 23.263653 * myVar[threadIdx.x];
	myVar[6] = 0.334531 * myVar[threadIdx.x];
	myVar[8] = 48.571792 * myVar[threadIdx.x];
	var_12_0[0] = myVar[0];
	var_12_1[1] = myVar[1];
	var_12_2[2] = myVar[2];
	var_12_3[3] = myVar[3];
	var_12_4[4] = myVar[4];
	var_12_5[5] = myVar[5];
	var_12_6[6] = myVar[6];
	var_12_7[7] = myVar[7];
	var_12_8[8] = myVar[8];
	var_12_9[9] = myVar[9];
	
}

__global__ void kernel_13(float * var_13_0, float * var_13_1, float * var_13_2, float * var_13_3, float * var_13_4, float * var_13_5, float * var_13_6, float * var_13_7, float * var_13_8, float * var_13_9) {
	__shared__ float myVar[1024];
	myVar[5] = 2.329164 * myVar[threadIdx.x];
	myVar[2] = 27.468162 * myVar[threadIdx.x];
	myVar[2] = 31.282914 * myVar[threadIdx.x];
	myVar[2] = 25.963372 * myVar[threadIdx.x];
	myVar[6] = 18.401266 * myVar[threadIdx.x];
	myVar[9] = 19.282669 * myVar[threadIdx.x];
	myVar[9] = 25.353553 * myVar[threadIdx.x];
	myVar[5] = 3.507961 * myVar[threadIdx.x];
	myVar[5] = 8.984620 * myVar[threadIdx.x];
	myVar[1] = 3.687348 * myVar[threadIdx.x];
	var_13_0[0] = myVar[0];
	var_13_1[1] = myVar[1];
	var_13_2[2] = myVar[2];
	var_13_3[3] = myVar[3];
	var_13_4[4] = myVar[4];
	var_13_5[5] = myVar[5];
	var_13_6[6] = myVar[6];
	var_13_7[7] = myVar[7];
	var_13_8[8] = myVar[8];
	var_13_9[9] = myVar[9];
	
}

__global__ void kernel_14(float * var_14_0, float * var_14_1, float * var_14_2, float * var_14_3, float * var_14_4, float * var_14_5, float * var_14_6, float * var_14_7, float * var_14_8, float * var_14_9) {
	__shared__ float myVar[1024];
	myVar[1] = 31.522549 * myVar[threadIdx.x];
	myVar[6] = 47.020477 * myVar[threadIdx.x];
	myVar[9] = 41.012974 * myVar[threadIdx.x];
	myVar[8] = 13.093407 * myVar[threadIdx.x];
	myVar[8] = 31.646809 * myVar[threadIdx.x];
	myVar[6] = 48.843660 * myVar[threadIdx.x];
	myVar[6] = 22.924309 * myVar[threadIdx.x];
	myVar[7] = 26.845918 * myVar[threadIdx.x];
	myVar[3] = 9.920997 * myVar[threadIdx.x];
	myVar[6] = 33.245486 * myVar[threadIdx.x];
	var_14_0[0] = myVar[0];
	var_14_1[1] = myVar[1];
	var_14_2[2] = myVar[2];
	var_14_3[3] = myVar[3];
	var_14_4[4] = myVar[4];
	var_14_5[5] = myVar[5];
	var_14_6[6] = myVar[6];
	var_14_7[7] = myVar[7];
	var_14_8[8] = myVar[8];
	var_14_9[9] = myVar[9];
	
}

__global__ void kernel_15(float * var_15_0, float * var_15_1, float * var_15_2, float * var_15_3, float * var_15_4, float * var_15_5, float * var_15_6, float * var_15_7, float * var_15_8, float * var_15_9) {
	__shared__ float myVar[1024];
	myVar[7] = 40.622161 * myVar[threadIdx.x];
	myVar[7] = 19.946032 * myVar[threadIdx.x];
	myVar[5] = 21.594293 * myVar[threadIdx.x];
	myVar[6] = 4.439377 * myVar[threadIdx.x];
	myVar[9] = 42.537210 * myVar[threadIdx.x];
	myVar[6] = 49.677309 * myVar[threadIdx.x];
	myVar[5] = 43.046848 * myVar[threadIdx.x];
	myVar[4] = 28.071790 * myVar[threadIdx.x];
	myVar[2] = 16.273522 * myVar[threadIdx.x];
	myVar[1] = 14.347631 * myVar[threadIdx.x];
	var_15_0[0] = myVar[0];
	var_15_1[1] = myVar[1];
	var_15_2[2] = myVar[2];
	var_15_3[3] = myVar[3];
	var_15_4[4] = myVar[4];
	var_15_5[5] = myVar[5];
	var_15_6[6] = myVar[6];
	var_15_7[7] = myVar[7];
	var_15_8[8] = myVar[8];
	var_15_9[9] = myVar[9];
	
}

__global__ void kernel_16(float * var_16_0, float * var_16_1, float * var_16_2, float * var_16_3, float * var_16_4, float * var_16_5, float * var_16_6, float * var_16_7, float * var_16_8, float * var_16_9) {
	__shared__ float myVar[1024];
	myVar[3] = 0.721046 * myVar[threadIdx.x];
	myVar[8] = 29.682274 * myVar[threadIdx.x];
	myVar[7] = 45.508895 * myVar[threadIdx.x];
	myVar[3] = 16.352109 * myVar[threadIdx.x];
	myVar[1] = 7.590435 * myVar[threadIdx.x];
	myVar[8] = 29.519743 * myVar[threadIdx.x];
	myVar[5] = 33.387906 * myVar[threadIdx.x];
	myVar[5] = 29.884450 * myVar[threadIdx.x];
	myVar[8] = 33.424171 * myVar[threadIdx.x];
	myVar[3] = 26.802417 * myVar[threadIdx.x];
	var_16_0[0] = myVar[0];
	var_16_1[1] = myVar[1];
	var_16_2[2] = myVar[2];
	var_16_3[3] = myVar[3];
	var_16_4[4] = myVar[4];
	var_16_5[5] = myVar[5];
	var_16_6[6] = myVar[6];
	var_16_7[7] = myVar[7];
	var_16_8[8] = myVar[8];
	var_16_9[9] = myVar[9];
	
}

__global__ void kernel_17(float * var_17_0, float * var_17_1, float * var_17_2, float * var_17_3, float * var_17_4, float * var_17_5, float * var_17_6, float * var_17_7, float * var_17_8, float * var_17_9) {
	__shared__ float myVar[1024];
	myVar[9] = 24.796966 * myVar[threadIdx.x];
	myVar[9] = 44.205041 * myVar[threadIdx.x];
	myVar[1] = 5.699130 * myVar[threadIdx.x];
	myVar[6] = 0.715713 * myVar[threadIdx.x];
	myVar[4] = 19.104383 * myVar[threadIdx.x];
	myVar[2] = 23.502298 * myVar[threadIdx.x];
	myVar[6] = 4.815490 * myVar[threadIdx.x];
	myVar[0] = 23.821100 * myVar[threadIdx.x];
	myVar[8] = 17.668747 * myVar[threadIdx.x];
	myVar[6] = 17.090312 * myVar[threadIdx.x];
	var_17_0[0] = myVar[0];
	var_17_1[1] = myVar[1];
	var_17_2[2] = myVar[2];
	var_17_3[3] = myVar[3];
	var_17_4[4] = myVar[4];
	var_17_5[5] = myVar[5];
	var_17_6[6] = myVar[6];
	var_17_7[7] = myVar[7];
	var_17_8[8] = myVar[8];
	var_17_9[9] = myVar[9];
	
}

__global__ void kernel_18(float * var_18_0, float * var_18_1, float * var_18_2, float * var_18_3, float * var_18_4, float * var_18_5, float * var_18_6, float * var_18_7, float * var_18_8, float * var_18_9) {
	__shared__ float myVar[1024];
	myVar[4] = 3.345577 * myVar[threadIdx.x];
	myVar[7] = 37.659292 * myVar[threadIdx.x];
	myVar[3] = 21.886083 * myVar[threadIdx.x];
	myVar[2] = 33.293481 * myVar[threadIdx.x];
	myVar[1] = 4.373278 * myVar[threadIdx.x];
	myVar[3] = 36.263312 * myVar[threadIdx.x];
	myVar[5] = 48.536571 * myVar[threadIdx.x];
	myVar[4] = 42.266164 * myVar[threadIdx.x];
	myVar[2] = 38.843900 * myVar[threadIdx.x];
	myVar[0] = 17.701493 * myVar[threadIdx.x];
	var_18_0[0] = myVar[0];
	var_18_1[1] = myVar[1];
	var_18_2[2] = myVar[2];
	var_18_3[3] = myVar[3];
	var_18_4[4] = myVar[4];
	var_18_5[5] = myVar[5];
	var_18_6[6] = myVar[6];
	var_18_7[7] = myVar[7];
	var_18_8[8] = myVar[8];
	var_18_9[9] = myVar[9];
	
}

__global__ void kernel_19(float * var_19_0, float * var_19_1, float * var_19_2, float * var_19_3, float * var_19_4, float * var_19_5, float * var_19_6, float * var_19_7, float * var_19_8, float * var_19_9) {
	__shared__ float myVar[1024];
	myVar[2] = 9.784314 * myVar[threadIdx.x];
	myVar[4] = 31.513838 * myVar[threadIdx.x];
	myVar[1] = 40.367282 * myVar[threadIdx.x];
	myVar[8] = 41.773924 * myVar[threadIdx.x];
	myVar[0] = 30.935291 * myVar[threadIdx.x];
	myVar[6] = 3.365774 * myVar[threadIdx.x];
	myVar[2] = 33.805888 * myVar[threadIdx.x];
	myVar[1] = 11.250243 * myVar[threadIdx.x];
	myVar[1] = 16.077158 * myVar[threadIdx.x];
	myVar[7] = 16.428407 * myVar[threadIdx.x];
	var_19_0[0] = myVar[0];
	var_19_1[1] = myVar[1];
	var_19_2[2] = myVar[2];
	var_19_3[3] = myVar[3];
	var_19_4[4] = myVar[4];
	var_19_5[5] = myVar[5];
	var_19_6[6] = myVar[6];
	var_19_7[7] = myVar[7];
	var_19_8[8] = myVar[8];
	var_19_9[9] = myVar[9];
	
}

__global__ void kernel_20(float * var_20_0, float * var_20_1, float * var_20_2, float * var_20_3, float * var_20_4, float * var_20_5, float * var_20_6, float * var_20_7, float * var_20_8, float * var_20_9) {
	__shared__ float myVar[1024];
	myVar[5] = 12.700576 * myVar[threadIdx.x];
	myVar[3] = 41.213208 * myVar[threadIdx.x];
	myVar[5] = 9.692584 * myVar[threadIdx.x];
	myVar[4] = 32.773133 * myVar[threadIdx.x];
	myVar[4] = 1.923570 * myVar[threadIdx.x];
	myVar[6] = 16.914192 * myVar[threadIdx.x];
	myVar[3] = 36.856016 * myVar[threadIdx.x];
	myVar[9] = 23.362504 * myVar[threadIdx.x];
	myVar[0] = 36.527513 * myVar[threadIdx.x];
	myVar[0] = 35.543537 * myVar[threadIdx.x];
	var_20_0[0] = myVar[0];
	var_20_1[1] = myVar[1];
	var_20_2[2] = myVar[2];
	var_20_3[3] = myVar[3];
	var_20_4[4] = myVar[4];
	var_20_5[5] = myVar[5];
	var_20_6[6] = myVar[6];
	var_20_7[7] = myVar[7];
	var_20_8[8] = myVar[8];
	var_20_9[9] = myVar[9];
	
}

__global__ void kernel_21(float * var_21_0, float * var_21_1, float * var_21_2, float * var_21_3, float * var_21_4, float * var_21_5, float * var_21_6, float * var_21_7, float * var_21_8, float * var_21_9) {
	__shared__ float myVar[1024];
	myVar[4] = 33.888910 * myVar[threadIdx.x];
	myVar[3] = 42.863040 * myVar[threadIdx.x];
	myVar[2] = 21.880141 * myVar[threadIdx.x];
	myVar[8] = 34.265026 * myVar[threadIdx.x];
	myVar[8] = 0.629872 * myVar[threadIdx.x];
	myVar[9] = 3.620120 * myVar[threadIdx.x];
	myVar[1] = 27.851501 * myVar[threadIdx.x];
	myVar[4] = 10.533276 * myVar[threadIdx.x];
	myVar[9] = 11.981865 * myVar[threadIdx.x];
	myVar[2] = 28.404232 * myVar[threadIdx.x];
	var_21_0[0] = myVar[0];
	var_21_1[1] = myVar[1];
	var_21_2[2] = myVar[2];
	var_21_3[3] = myVar[3];
	var_21_4[4] = myVar[4];
	var_21_5[5] = myVar[5];
	var_21_6[6] = myVar[6];
	var_21_7[7] = myVar[7];
	var_21_8[8] = myVar[8];
	var_21_9[9] = myVar[9];
	
}

__global__ void kernel_22(float * var_22_0, float * var_22_1, float * var_22_2, float * var_22_3, float * var_22_4, float * var_22_5, float * var_22_6, float * var_22_7, float * var_22_8, float * var_22_9) {
	__shared__ float myVar[1024];
	myVar[3] = 42.561043 * myVar[threadIdx.x];
	myVar[1] = 5.375239 * myVar[threadIdx.x];
	myVar[0] = 12.570711 * myVar[threadIdx.x];
	myVar[8] = 48.561617 * myVar[threadIdx.x];
	myVar[8] = 46.902203 * myVar[threadIdx.x];
	myVar[3] = 18.228843 * myVar[threadIdx.x];
	myVar[2] = 19.245598 * myVar[threadIdx.x];
	myVar[0] = 13.722794 * myVar[threadIdx.x];
	myVar[5] = 27.891088 * myVar[threadIdx.x];
	myVar[8] = 29.828456 * myVar[threadIdx.x];
	var_22_0[0] = myVar[0];
	var_22_1[1] = myVar[1];
	var_22_2[2] = myVar[2];
	var_22_3[3] = myVar[3];
	var_22_4[4] = myVar[4];
	var_22_5[5] = myVar[5];
	var_22_6[6] = myVar[6];
	var_22_7[7] = myVar[7];
	var_22_8[8] = myVar[8];
	var_22_9[9] = myVar[9];
	
}

__global__ void kernel_23(float * var_23_0, float * var_23_1, float * var_23_2, float * var_23_3, float * var_23_4, float * var_23_5, float * var_23_6, float * var_23_7, float * var_23_8, float * var_23_9) {
	__shared__ float myVar[1024];
	myVar[6] = 3.447684 * myVar[threadIdx.x];
	myVar[9] = 3.640567 * myVar[threadIdx.x];
	myVar[3] = 49.837136 * myVar[threadIdx.x];
	myVar[4] = 10.540101 * myVar[threadIdx.x];
	myVar[1] = 8.881791 * myVar[threadIdx.x];
	myVar[1] = 22.540074 * myVar[threadIdx.x];
	myVar[7] = 38.443411 * myVar[threadIdx.x];
	myVar[6] = 25.384029 * myVar[threadIdx.x];
	myVar[3] = 48.160367 * myVar[threadIdx.x];
	myVar[9] = 13.455500 * myVar[threadIdx.x];
	var_23_0[0] = myVar[0];
	var_23_1[1] = myVar[1];
	var_23_2[2] = myVar[2];
	var_23_3[3] = myVar[3];
	var_23_4[4] = myVar[4];
	var_23_5[5] = myVar[5];
	var_23_6[6] = myVar[6];
	var_23_7[7] = myVar[7];
	var_23_8[8] = myVar[8];
	var_23_9[9] = myVar[9];
	
}

__global__ void kernel_24(float * var_24_0, float * var_24_1, float * var_24_2, float * var_24_3, float * var_24_4, float * var_24_5, float * var_24_6, float * var_24_7, float * var_24_8, float * var_24_9) {
	__shared__ float myVar[1024];
	myVar[4] = 15.322267 * myVar[threadIdx.x];
	myVar[9] = 18.603918 * myVar[threadIdx.x];
	myVar[0] = 35.903317 * myVar[threadIdx.x];
	myVar[9] = 31.488988 * myVar[threadIdx.x];
	myVar[7] = 3.880680 * myVar[threadIdx.x];
	myVar[9] = 17.495960 * myVar[threadIdx.x];
	myVar[5] = 6.304608 * myVar[threadIdx.x];
	myVar[2] = 20.825722 * myVar[threadIdx.x];
	myVar[9] = 10.871806 * myVar[threadIdx.x];
	myVar[1] = 6.955122 * myVar[threadIdx.x];
	var_24_0[0] = myVar[0];
	var_24_1[1] = myVar[1];
	var_24_2[2] = myVar[2];
	var_24_3[3] = myVar[3];
	var_24_4[4] = myVar[4];
	var_24_5[5] = myVar[5];
	var_24_6[6] = myVar[6];
	var_24_7[7] = myVar[7];
	var_24_8[8] = myVar[8];
	var_24_9[9] = myVar[9];
	
}

__global__ void kernel_25(float * var_25_0, float * var_25_1, float * var_25_2, float * var_25_3, float * var_25_4, float * var_25_5, float * var_25_6, float * var_25_7, float * var_25_8, float * var_25_9) {
	__shared__ float myVar[1024];
	myVar[0] = 28.724806 * myVar[threadIdx.x];
	myVar[4] = 4.714734 * myVar[threadIdx.x];
	myVar[5] = 46.098177 * myVar[threadIdx.x];
	myVar[0] = 44.913593 * myVar[threadIdx.x];
	myVar[6] = 47.402892 * myVar[threadIdx.x];
	myVar[4] = 2.686918 * myVar[threadIdx.x];
	myVar[5] = 35.819338 * myVar[threadIdx.x];
	myVar[2] = 7.862932 * myVar[threadIdx.x];
	myVar[2] = 32.582440 * myVar[threadIdx.x];
	myVar[1] = 45.237856 * myVar[threadIdx.x];
	var_25_0[0] = myVar[0];
	var_25_1[1] = myVar[1];
	var_25_2[2] = myVar[2];
	var_25_3[3] = myVar[3];
	var_25_4[4] = myVar[4];
	var_25_5[5] = myVar[5];
	var_25_6[6] = myVar[6];
	var_25_7[7] = myVar[7];
	var_25_8[8] = myVar[8];
	var_25_9[9] = myVar[9];
	
}

__global__ void kernel_26(float * var_26_0, float * var_26_1, float * var_26_2, float * var_26_3, float * var_26_4, float * var_26_5, float * var_26_6, float * var_26_7, float * var_26_8, float * var_26_9) {
	__shared__ float myVar[1024];
	myVar[6] = 13.791539 * myVar[threadIdx.x];
	myVar[1] = 48.158467 * myVar[threadIdx.x];
	myVar[7] = 29.527633 * myVar[threadIdx.x];
	myVar[6] = 19.880055 * myVar[threadIdx.x];
	myVar[8] = 25.291974 * myVar[threadIdx.x];
	myVar[8] = 46.013702 * myVar[threadIdx.x];
	myVar[2] = 25.500842 * myVar[threadIdx.x];
	myVar[9] = 15.082102 * myVar[threadIdx.x];
	myVar[3] = 49.431983 * myVar[threadIdx.x];
	myVar[8] = 37.754153 * myVar[threadIdx.x];
	var_26_0[0] = myVar[0];
	var_26_1[1] = myVar[1];
	var_26_2[2] = myVar[2];
	var_26_3[3] = myVar[3];
	var_26_4[4] = myVar[4];
	var_26_5[5] = myVar[5];
	var_26_6[6] = myVar[6];
	var_26_7[7] = myVar[7];
	var_26_8[8] = myVar[8];
	var_26_9[9] = myVar[9];
	
}

__global__ void kernel_27(float * var_27_0, float * var_27_1, float * var_27_2, float * var_27_3, float * var_27_4, float * var_27_5, float * var_27_6, float * var_27_7, float * var_27_8, float * var_27_9) {
	__shared__ float myVar[1024];
	myVar[2] = 49.098625 * myVar[threadIdx.x];
	myVar[6] = 15.201905 * myVar[threadIdx.x];
	myVar[6] = 8.158726 * myVar[threadIdx.x];
	myVar[1] = 29.073564 * myVar[threadIdx.x];
	myVar[4] = 22.325693 * myVar[threadIdx.x];
	myVar[9] = 15.360091 * myVar[threadIdx.x];
	myVar[9] = 31.617193 * myVar[threadIdx.x];
	myVar[9] = 26.942423 * myVar[threadIdx.x];
	myVar[7] = 4.814359 * myVar[threadIdx.x];
	myVar[9] = 8.658239 * myVar[threadIdx.x];
	var_27_0[0] = myVar[0];
	var_27_1[1] = myVar[1];
	var_27_2[2] = myVar[2];
	var_27_3[3] = myVar[3];
	var_27_4[4] = myVar[4];
	var_27_5[5] = myVar[5];
	var_27_6[6] = myVar[6];
	var_27_7[7] = myVar[7];
	var_27_8[8] = myVar[8];
	var_27_9[9] = myVar[9];
	
}

__global__ void kernel_28(float * var_28_0, float * var_28_1, float * var_28_2, float * var_28_3, float * var_28_4, float * var_28_5, float * var_28_6, float * var_28_7, float * var_28_8, float * var_28_9) {
	__shared__ float myVar[1024];
	myVar[9] = 32.363108 * myVar[threadIdx.x];
	myVar[0] = 26.294276 * myVar[threadIdx.x];
	myVar[3] = 4.833897 * myVar[threadIdx.x];
	myVar[8] = 27.514344 * myVar[threadIdx.x];
	myVar[4] = 10.111906 * myVar[threadIdx.x];
	myVar[6] = 27.445743 * myVar[threadIdx.x];
	myVar[0] = 20.299435 * myVar[threadIdx.x];
	myVar[3] = 32.876743 * myVar[threadIdx.x];
	myVar[4] = 21.097365 * myVar[threadIdx.x];
	myVar[2] = 29.296228 * myVar[threadIdx.x];
	var_28_0[0] = myVar[0];
	var_28_1[1] = myVar[1];
	var_28_2[2] = myVar[2];
	var_28_3[3] = myVar[3];
	var_28_4[4] = myVar[4];
	var_28_5[5] = myVar[5];
	var_28_6[6] = myVar[6];
	var_28_7[7] = myVar[7];
	var_28_8[8] = myVar[8];
	var_28_9[9] = myVar[9];
	
}

__global__ void kernel_29(float * var_29_0, float * var_29_1, float * var_29_2, float * var_29_3, float * var_29_4, float * var_29_5, float * var_29_6, float * var_29_7, float * var_29_8, float * var_29_9) {
	__shared__ float myVar[1024];
	myVar[4] = 3.609021 * myVar[threadIdx.x];
	myVar[9] = 33.072502 * myVar[threadIdx.x];
	myVar[3] = 31.548664 * myVar[threadIdx.x];
	myVar[1] = 49.996068 * myVar[threadIdx.x];
	myVar[5] = 29.369450 * myVar[threadIdx.x];
	myVar[6] = 3.544996 * myVar[threadIdx.x];
	myVar[1] = 2.933794 * myVar[threadIdx.x];
	myVar[3] = 40.278806 * myVar[threadIdx.x];
	myVar[3] = 39.109933 * myVar[threadIdx.x];
	myVar[3] = 49.954560 * myVar[threadIdx.x];
	var_29_0[0] = myVar[0];
	var_29_1[1] = myVar[1];
	var_29_2[2] = myVar[2];
	var_29_3[3] = myVar[3];
	var_29_4[4] = myVar[4];
	var_29_5[5] = myVar[5];
	var_29_6[6] = myVar[6];
	var_29_7[7] = myVar[7];
	var_29_8[8] = myVar[8];
	var_29_9[9] = myVar[9];
	
}

__global__ void kernel_30(float * var_30_0, float * var_30_1, float * var_30_2, float * var_30_3, float * var_30_4, float * var_30_5, float * var_30_6, float * var_30_7, float * var_30_8, float * var_30_9) {
	__shared__ float myVar[1024];
	myVar[5] = 37.387502 * myVar[threadIdx.x];
	myVar[6] = 2.144567 * myVar[threadIdx.x];
	myVar[7] = 23.431729 * myVar[threadIdx.x];
	myVar[5] = 17.514979 * myVar[threadIdx.x];
	myVar[0] = 24.510694 * myVar[threadIdx.x];
	myVar[0] = 6.914233 * myVar[threadIdx.x];
	myVar[1] = 43.205739 * myVar[threadIdx.x];
	myVar[7] = 40.285453 * myVar[threadIdx.x];
	myVar[5] = 19.025275 * myVar[threadIdx.x];
	myVar[3] = 8.837556 * myVar[threadIdx.x];
	var_30_0[0] = myVar[0];
	var_30_1[1] = myVar[1];
	var_30_2[2] = myVar[2];
	var_30_3[3] = myVar[3];
	var_30_4[4] = myVar[4];
	var_30_5[5] = myVar[5];
	var_30_6[6] = myVar[6];
	var_30_7[7] = myVar[7];
	var_30_8[8] = myVar[8];
	var_30_9[9] = myVar[9];
	
}

__global__ void kernel_31(float * var_31_0, float * var_31_1, float * var_31_2, float * var_31_3, float * var_31_4, float * var_31_5, float * var_31_6, float * var_31_7, float * var_31_8, float * var_31_9) {
	__shared__ float myVar[1024];
	myVar[7] = 12.833705 * myVar[threadIdx.x];
	myVar[1] = 28.441507 * myVar[threadIdx.x];
	myVar[7] = 13.415374 * myVar[threadIdx.x];
	myVar[2] = 8.406161 * myVar[threadIdx.x];
	myVar[5] = 17.512429 * myVar[threadIdx.x];
	myVar[6] = 42.466597 * myVar[threadIdx.x];
	myVar[5] = 34.852249 * myVar[threadIdx.x];
	myVar[0] = 39.042674 * myVar[threadIdx.x];
	myVar[5] = 32.211152 * myVar[threadIdx.x];
	myVar[4] = 31.657029 * myVar[threadIdx.x];
	var_31_0[0] = myVar[0];
	var_31_1[1] = myVar[1];
	var_31_2[2] = myVar[2];
	var_31_3[3] = myVar[3];
	var_31_4[4] = myVar[4];
	var_31_5[5] = myVar[5];
	var_31_6[6] = myVar[6];
	var_31_7[7] = myVar[7];
	var_31_8[8] = myVar[8];
	var_31_9[9] = myVar[9];
	
}

__global__ void kernel_32(float * var_32_0, float * var_32_1, float * var_32_2, float * var_32_3, float * var_32_4, float * var_32_5, float * var_32_6, float * var_32_7, float * var_32_8, float * var_32_9) {
	__shared__ float myVar[1024];
	myVar[6] = 33.097889 * myVar[threadIdx.x];
	myVar[0] = 8.568342 * myVar[threadIdx.x];
	myVar[3] = 37.312544 * myVar[threadIdx.x];
	myVar[8] = 2.731467 * myVar[threadIdx.x];
	myVar[0] = 7.537503 * myVar[threadIdx.x];
	myVar[2] = 31.249875 * myVar[threadIdx.x];
	myVar[4] = 15.036837 * myVar[threadIdx.x];
	myVar[3] = 1.455600 * myVar[threadIdx.x];
	myVar[6] = 20.962517 * myVar[threadIdx.x];
	myVar[7] = 11.834914 * myVar[threadIdx.x];
	var_32_0[0] = myVar[0];
	var_32_1[1] = myVar[1];
	var_32_2[2] = myVar[2];
	var_32_3[3] = myVar[3];
	var_32_4[4] = myVar[4];
	var_32_5[5] = myVar[5];
	var_32_6[6] = myVar[6];
	var_32_7[7] = myVar[7];
	var_32_8[8] = myVar[8];
	var_32_9[9] = myVar[9];
	
}

__global__ void kernel_33(float * var_33_0, float * var_33_1, float * var_33_2, float * var_33_3, float * var_33_4, float * var_33_5, float * var_33_6, float * var_33_7, float * var_33_8, float * var_33_9) {
	__shared__ float myVar[1024];
	myVar[1] = 49.841332 * myVar[threadIdx.x];
	myVar[8] = 20.504063 * myVar[threadIdx.x];
	myVar[9] = 41.076575 * myVar[threadIdx.x];
	myVar[6] = 21.032054 * myVar[threadIdx.x];
	myVar[0] = 40.220464 * myVar[threadIdx.x];
	myVar[7] = 9.936741 * myVar[threadIdx.x];
	myVar[8] = 41.653157 * myVar[threadIdx.x];
	myVar[0] = 11.531191 * myVar[threadIdx.x];
	myVar[8] = 17.733310 * myVar[threadIdx.x];
	myVar[3] = 22.221154 * myVar[threadIdx.x];
	var_33_0[0] = myVar[0];
	var_33_1[1] = myVar[1];
	var_33_2[2] = myVar[2];
	var_33_3[3] = myVar[3];
	var_33_4[4] = myVar[4];
	var_33_5[5] = myVar[5];
	var_33_6[6] = myVar[6];
	var_33_7[7] = myVar[7];
	var_33_8[8] = myVar[8];
	var_33_9[9] = myVar[9];
	
}

__global__ void kernel_34(float * var_34_0, float * var_34_1, float * var_34_2, float * var_34_3, float * var_34_4, float * var_34_5, float * var_34_6, float * var_34_7, float * var_34_8, float * var_34_9) {
	__shared__ float myVar[1024];
	myVar[9] = 40.016089 * myVar[threadIdx.x];
	myVar[0] = 7.827281 * myVar[threadIdx.x];
	myVar[5] = 47.266293 * myVar[threadIdx.x];
	myVar[5] = 30.054875 * myVar[threadIdx.x];
	myVar[2] = 39.705856 * myVar[threadIdx.x];
	myVar[0] = 26.049503 * myVar[threadIdx.x];
	myVar[2] = 7.311032 * myVar[threadIdx.x];
	myVar[7] = 26.354148 * myVar[threadIdx.x];
	myVar[5] = 7.888674 * myVar[threadIdx.x];
	myVar[8] = 30.327730 * myVar[threadIdx.x];
	var_34_0[0] = myVar[0];
	var_34_1[1] = myVar[1];
	var_34_2[2] = myVar[2];
	var_34_3[3] = myVar[3];
	var_34_4[4] = myVar[4];
	var_34_5[5] = myVar[5];
	var_34_6[6] = myVar[6];
	var_34_7[7] = myVar[7];
	var_34_8[8] = myVar[8];
	var_34_9[9] = myVar[9];
	
}

__global__ void kernel_35(float * var_35_0, float * var_35_1, float * var_35_2, float * var_35_3, float * var_35_4, float * var_35_5, float * var_35_6, float * var_35_7, float * var_35_8, float * var_35_9) {
	__shared__ float myVar[1024];
	myVar[4] = 4.548860 * myVar[threadIdx.x];
	myVar[0] = 48.596612 * myVar[threadIdx.x];
	myVar[2] = 19.079956 * myVar[threadIdx.x];
	myVar[8] = 31.379061 * myVar[threadIdx.x];
	myVar[3] = 18.408245 * myVar[threadIdx.x];
	myVar[7] = 29.562698 * myVar[threadIdx.x];
	myVar[5] = 11.288055 * myVar[threadIdx.x];
	myVar[3] = 28.110632 * myVar[threadIdx.x];
	myVar[3] = 18.387617 * myVar[threadIdx.x];
	myVar[1] = 36.910730 * myVar[threadIdx.x];
	var_35_0[0] = myVar[0];
	var_35_1[1] = myVar[1];
	var_35_2[2] = myVar[2];
	var_35_3[3] = myVar[3];
	var_35_4[4] = myVar[4];
	var_35_5[5] = myVar[5];
	var_35_6[6] = myVar[6];
	var_35_7[7] = myVar[7];
	var_35_8[8] = myVar[8];
	var_35_9[9] = myVar[9];
	
}

__global__ void kernel_36(float * var_36_0, float * var_36_1, float * var_36_2, float * var_36_3, float * var_36_4, float * var_36_5, float * var_36_6, float * var_36_7, float * var_36_8, float * var_36_9) {
	__shared__ float myVar[1024];
	myVar[4] = 38.709092 * myVar[threadIdx.x];
	myVar[1] = 17.538780 * myVar[threadIdx.x];
	myVar[3] = 27.188513 * myVar[threadIdx.x];
	myVar[2] = 19.507238 * myVar[threadIdx.x];
	myVar[9] = 42.973725 * myVar[threadIdx.x];
	myVar[6] = 30.387322 * myVar[threadIdx.x];
	myVar[9] = 11.370702 * myVar[threadIdx.x];
	myVar[2] = 20.046934 * myVar[threadIdx.x];
	myVar[3] = 23.269483 * myVar[threadIdx.x];
	myVar[7] = 42.634197 * myVar[threadIdx.x];
	var_36_0[0] = myVar[0];
	var_36_1[1] = myVar[1];
	var_36_2[2] = myVar[2];
	var_36_3[3] = myVar[3];
	var_36_4[4] = myVar[4];
	var_36_5[5] = myVar[5];
	var_36_6[6] = myVar[6];
	var_36_7[7] = myVar[7];
	var_36_8[8] = myVar[8];
	var_36_9[9] = myVar[9];
	
}

__global__ void kernel_37(float * var_37_0, float * var_37_1, float * var_37_2, float * var_37_3, float * var_37_4, float * var_37_5, float * var_37_6, float * var_37_7, float * var_37_8, float * var_37_9) {
	__shared__ float myVar[1024];
	myVar[9] = 5.358901 * myVar[threadIdx.x];
	myVar[6] = 37.276176 * myVar[threadIdx.x];
	myVar[7] = 38.499256 * myVar[threadIdx.x];
	myVar[5] = 0.677148 * myVar[threadIdx.x];
	myVar[2] = 17.141034 * myVar[threadIdx.x];
	myVar[5] = 5.427960 * myVar[threadIdx.x];
	myVar[9] = 5.819996 * myVar[threadIdx.x];
	myVar[4] = 24.209951 * myVar[threadIdx.x];
	myVar[2] = 45.153299 * myVar[threadIdx.x];
	myVar[6] = 13.056218 * myVar[threadIdx.x];
	var_37_0[0] = myVar[0];
	var_37_1[1] = myVar[1];
	var_37_2[2] = myVar[2];
	var_37_3[3] = myVar[3];
	var_37_4[4] = myVar[4];
	var_37_5[5] = myVar[5];
	var_37_6[6] = myVar[6];
	var_37_7[7] = myVar[7];
	var_37_8[8] = myVar[8];
	var_37_9[9] = myVar[9];
	
}

__global__ void kernel_38(float * var_38_0, float * var_38_1, float * var_38_2, float * var_38_3, float * var_38_4, float * var_38_5, float * var_38_6, float * var_38_7, float * var_38_8, float * var_38_9) {
	__shared__ float myVar[1024];
	myVar[8] = 5.460291 * myVar[threadIdx.x];
	myVar[1] = 25.222137 * myVar[threadIdx.x];
	myVar[5] = 17.176304 * myVar[threadIdx.x];
	myVar[7] = 28.634038 * myVar[threadIdx.x];
	myVar[6] = 23.609900 * myVar[threadIdx.x];
	myVar[3] = 41.332861 * myVar[threadIdx.x];
	myVar[8] = 29.642004 * myVar[threadIdx.x];
	myVar[7] = 19.468654 * myVar[threadIdx.x];
	myVar[1] = 38.410628 * myVar[threadIdx.x];
	myVar[8] = 24.252108 * myVar[threadIdx.x];
	var_38_0[0] = myVar[0];
	var_38_1[1] = myVar[1];
	var_38_2[2] = myVar[2];
	var_38_3[3] = myVar[3];
	var_38_4[4] = myVar[4];
	var_38_5[5] = myVar[5];
	var_38_6[6] = myVar[6];
	var_38_7[7] = myVar[7];
	var_38_8[8] = myVar[8];
	var_38_9[9] = myVar[9];
	
}

__global__ void kernel_39(float * var_39_0, float * var_39_1, float * var_39_2, float * var_39_3, float * var_39_4, float * var_39_5, float * var_39_6, float * var_39_7, float * var_39_8, float * var_39_9) {
	__shared__ float myVar[1024];
	myVar[7] = 4.699386 * myVar[threadIdx.x];
	myVar[4] = 42.780262 * myVar[threadIdx.x];
	myVar[2] = 46.730611 * myVar[threadIdx.x];
	myVar[1] = 17.028525 * myVar[threadIdx.x];
	myVar[2] = 26.071464 * myVar[threadIdx.x];
	myVar[3] = 1.573222 * myVar[threadIdx.x];
	myVar[6] = 43.866070 * myVar[threadIdx.x];
	myVar[3] = 39.808741 * myVar[threadIdx.x];
	myVar[0] = 10.624138 * myVar[threadIdx.x];
	myVar[6] = 46.929066 * myVar[threadIdx.x];
	var_39_0[0] = myVar[0];
	var_39_1[1] = myVar[1];
	var_39_2[2] = myVar[2];
	var_39_3[3] = myVar[3];
	var_39_4[4] = myVar[4];
	var_39_5[5] = myVar[5];
	var_39_6[6] = myVar[6];
	var_39_7[7] = myVar[7];
	var_39_8[8] = myVar[8];
	var_39_9[9] = myVar[9];
	
}

__global__ void kernel_40(float * var_40_0, float * var_40_1, float * var_40_2, float * var_40_3, float * var_40_4, float * var_40_5, float * var_40_6, float * var_40_7, float * var_40_8, float * var_40_9) {
	__shared__ float myVar[1024];
	myVar[5] = 28.462100 * myVar[threadIdx.x];
	myVar[5] = 16.902711 * myVar[threadIdx.x];
	myVar[2] = 24.259712 * myVar[threadIdx.x];
	myVar[0] = 34.166913 * myVar[threadIdx.x];
	myVar[4] = 49.967410 * myVar[threadIdx.x];
	myVar[7] = 49.559763 * myVar[threadIdx.x];
	myVar[9] = 25.396087 * myVar[threadIdx.x];
	myVar[4] = 19.431114 * myVar[threadIdx.x];
	myVar[7] = 27.760430 * myVar[threadIdx.x];
	myVar[4] = 5.094379 * myVar[threadIdx.x];
	var_40_0[0] = myVar[0];
	var_40_1[1] = myVar[1];
	var_40_2[2] = myVar[2];
	var_40_3[3] = myVar[3];
	var_40_4[4] = myVar[4];
	var_40_5[5] = myVar[5];
	var_40_6[6] = myVar[6];
	var_40_7[7] = myVar[7];
	var_40_8[8] = myVar[8];
	var_40_9[9] = myVar[9];
	
}

__global__ void kernel_41(float * var_41_0, float * var_41_1, float * var_41_2, float * var_41_3, float * var_41_4, float * var_41_5, float * var_41_6, float * var_41_7, float * var_41_8, float * var_41_9) {
	__shared__ float myVar[1024];
	myVar[2] = 22.832298 * myVar[threadIdx.x];
	myVar[1] = 32.084364 * myVar[threadIdx.x];
	myVar[7] = 26.671853 * myVar[threadIdx.x];
	myVar[8] = 7.974848 * myVar[threadIdx.x];
	myVar[2] = 29.369853 * myVar[threadIdx.x];
	myVar[8] = 32.925229 * myVar[threadIdx.x];
	myVar[5] = 28.874093 * myVar[threadIdx.x];
	myVar[1] = 29.357745 * myVar[threadIdx.x];
	myVar[2] = 30.595407 * myVar[threadIdx.x];
	myVar[0] = 46.058006 * myVar[threadIdx.x];
	var_41_0[0] = myVar[0];
	var_41_1[1] = myVar[1];
	var_41_2[2] = myVar[2];
	var_41_3[3] = myVar[3];
	var_41_4[4] = myVar[4];
	var_41_5[5] = myVar[5];
	var_41_6[6] = myVar[6];
	var_41_7[7] = myVar[7];
	var_41_8[8] = myVar[8];
	var_41_9[9] = myVar[9];
	
}

__global__ void kernel_42(float * var_42_0, float * var_42_1, float * var_42_2, float * var_42_3, float * var_42_4, float * var_42_5, float * var_42_6, float * var_42_7, float * var_42_8, float * var_42_9) {
	__shared__ float myVar[1024];
	myVar[2] = 3.880864 * myVar[threadIdx.x];
	myVar[4] = 35.747074 * myVar[threadIdx.x];
	myVar[2] = 15.077994 * myVar[threadIdx.x];
	myVar[4] = 7.648367 * myVar[threadIdx.x];
	myVar[3] = 48.654527 * myVar[threadIdx.x];
	myVar[2] = 22.623383 * myVar[threadIdx.x];
	myVar[2] = 47.879960 * myVar[threadIdx.x];
	myVar[2] = 5.522035 * myVar[threadIdx.x];
	myVar[5] = 1.406982 * myVar[threadIdx.x];
	myVar[3] = 32.108976 * myVar[threadIdx.x];
	var_42_0[0] = myVar[0];
	var_42_1[1] = myVar[1];
	var_42_2[2] = myVar[2];
	var_42_3[3] = myVar[3];
	var_42_4[4] = myVar[4];
	var_42_5[5] = myVar[5];
	var_42_6[6] = myVar[6];
	var_42_7[7] = myVar[7];
	var_42_8[8] = myVar[8];
	var_42_9[9] = myVar[9];
	
}

__global__ void kernel_43(float * var_43_0, float * var_43_1, float * var_43_2, float * var_43_3, float * var_43_4, float * var_43_5, float * var_43_6, float * var_43_7, float * var_43_8, float * var_43_9) {
	__shared__ float myVar[1024];
	myVar[9] = 48.492659 * myVar[threadIdx.x];
	myVar[8] = 23.671270 * myVar[threadIdx.x];
	myVar[8] = 38.490300 * myVar[threadIdx.x];
	myVar[4] = 2.131732 * myVar[threadIdx.x];
	myVar[1] = 36.505205 * myVar[threadIdx.x];
	myVar[8] = 39.658574 * myVar[threadIdx.x];
	myVar[5] = 6.777877 * myVar[threadIdx.x];
	myVar[1] = 27.597590 * myVar[threadIdx.x];
	myVar[4] = 10.845351 * myVar[threadIdx.x];
	myVar[5] = 24.901491 * myVar[threadIdx.x];
	var_43_0[0] = myVar[0];
	var_43_1[1] = myVar[1];
	var_43_2[2] = myVar[2];
	var_43_3[3] = myVar[3];
	var_43_4[4] = myVar[4];
	var_43_5[5] = myVar[5];
	var_43_6[6] = myVar[6];
	var_43_7[7] = myVar[7];
	var_43_8[8] = myVar[8];
	var_43_9[9] = myVar[9];
	
}

__global__ void kernel_44(float * var_44_0, float * var_44_1, float * var_44_2, float * var_44_3, float * var_44_4, float * var_44_5, float * var_44_6, float * var_44_7, float * var_44_8, float * var_44_9) {
	__shared__ float myVar[1024];
	myVar[8] = 28.626900 * myVar[threadIdx.x];
	myVar[1] = 15.559386 * myVar[threadIdx.x];
	myVar[9] = 13.209298 * myVar[threadIdx.x];
	myVar[6] = 37.720059 * myVar[threadIdx.x];
	myVar[7] = 17.716526 * myVar[threadIdx.x];
	myVar[3] = 4.130992 * myVar[threadIdx.x];
	myVar[5] = 22.501120 * myVar[threadIdx.x];
	myVar[0] = 26.947997 * myVar[threadIdx.x];
	myVar[3] = 23.235711 * myVar[threadIdx.x];
	myVar[0] = 1.034861 * myVar[threadIdx.x];
	var_44_0[0] = myVar[0];
	var_44_1[1] = myVar[1];
	var_44_2[2] = myVar[2];
	var_44_3[3] = myVar[3];
	var_44_4[4] = myVar[4];
	var_44_5[5] = myVar[5];
	var_44_6[6] = myVar[6];
	var_44_7[7] = myVar[7];
	var_44_8[8] = myVar[8];
	var_44_9[9] = myVar[9];
	
}

__global__ void kernel_45(float * var_45_0, float * var_45_1, float * var_45_2, float * var_45_3, float * var_45_4, float * var_45_5, float * var_45_6, float * var_45_7, float * var_45_8, float * var_45_9) {
	__shared__ float myVar[1024];
	myVar[6] = 16.404295 * myVar[threadIdx.x];
	myVar[2] = 4.287836 * myVar[threadIdx.x];
	myVar[0] = 6.790351 * myVar[threadIdx.x];
	myVar[5] = 8.390972 * myVar[threadIdx.x];
	myVar[5] = 35.574646 * myVar[threadIdx.x];
	myVar[9] = 29.376300 * myVar[threadIdx.x];
	myVar[5] = 24.313347 * myVar[threadIdx.x];
	myVar[1] = 46.508907 * myVar[threadIdx.x];
	myVar[4] = 10.751607 * myVar[threadIdx.x];
	myVar[5] = 13.335187 * myVar[threadIdx.x];
	var_45_0[0] = myVar[0];
	var_45_1[1] = myVar[1];
	var_45_2[2] = myVar[2];
	var_45_3[3] = myVar[3];
	var_45_4[4] = myVar[4];
	var_45_5[5] = myVar[5];
	var_45_6[6] = myVar[6];
	var_45_7[7] = myVar[7];
	var_45_8[8] = myVar[8];
	var_45_9[9] = myVar[9];
	
}

__global__ void kernel_46(float * var_46_0, float * var_46_1, float * var_46_2, float * var_46_3, float * var_46_4, float * var_46_5, float * var_46_6, float * var_46_7, float * var_46_8, float * var_46_9) {
	__shared__ float myVar[1024];
	myVar[6] = 6.164319 * myVar[threadIdx.x];
	myVar[3] = 39.749101 * myVar[threadIdx.x];
	myVar[1] = 32.019275 * myVar[threadIdx.x];
	myVar[4] = 22.489652 * myVar[threadIdx.x];
	myVar[4] = 24.629295 * myVar[threadIdx.x];
	myVar[6] = 6.320353 * myVar[threadIdx.x];
	myVar[3] = 22.544241 * myVar[threadIdx.x];
	myVar[4] = 26.402154 * myVar[threadIdx.x];
	myVar[8] = 20.717110 * myVar[threadIdx.x];
	myVar[4] = 36.832258 * myVar[threadIdx.x];
	var_46_0[0] = myVar[0];
	var_46_1[1] = myVar[1];
	var_46_2[2] = myVar[2];
	var_46_3[3] = myVar[3];
	var_46_4[4] = myVar[4];
	var_46_5[5] = myVar[5];
	var_46_6[6] = myVar[6];
	var_46_7[7] = myVar[7];
	var_46_8[8] = myVar[8];
	var_46_9[9] = myVar[9];
	
}

__global__ void kernel_47(float * var_47_0, float * var_47_1, float * var_47_2, float * var_47_3, float * var_47_4, float * var_47_5, float * var_47_6, float * var_47_7, float * var_47_8, float * var_47_9) {
	__shared__ float myVar[1024];
	myVar[3] = 27.788791 * myVar[threadIdx.x];
	myVar[6] = 29.835578 * myVar[threadIdx.x];
	myVar[1] = 10.718828 * myVar[threadIdx.x];
	myVar[4] = 8.423091 * myVar[threadIdx.x];
	myVar[0] = 18.408419 * myVar[threadIdx.x];
	myVar[3] = 34.166867 * myVar[threadIdx.x];
	myVar[1] = 33.818438 * myVar[threadIdx.x];
	myVar[9] = 38.649392 * myVar[threadIdx.x];
	myVar[2] = 38.995460 * myVar[threadIdx.x];
	myVar[1] = 7.026142 * myVar[threadIdx.x];
	var_47_0[0] = myVar[0];
	var_47_1[1] = myVar[1];
	var_47_2[2] = myVar[2];
	var_47_3[3] = myVar[3];
	var_47_4[4] = myVar[4];
	var_47_5[5] = myVar[5];
	var_47_6[6] = myVar[6];
	var_47_7[7] = myVar[7];
	var_47_8[8] = myVar[8];
	var_47_9[9] = myVar[9];
	
}

__global__ void kernel_48(float * var_48_0, float * var_48_1, float * var_48_2, float * var_48_3, float * var_48_4, float * var_48_5, float * var_48_6, float * var_48_7, float * var_48_8, float * var_48_9) {
	__shared__ float myVar[1024];
	myVar[8] = 34.125071 * myVar[threadIdx.x];
	myVar[7] = 5.222487 * myVar[threadIdx.x];
	myVar[2] = 36.672181 * myVar[threadIdx.x];
	myVar[9] = 12.274317 * myVar[threadIdx.x];
	myVar[6] = 9.177071 * myVar[threadIdx.x];
	myVar[7] = 5.821057 * myVar[threadIdx.x];
	myVar[8] = 1.231224 * myVar[threadIdx.x];
	myVar[3] = 49.790522 * myVar[threadIdx.x];
	myVar[8] = 39.761171 * myVar[threadIdx.x];
	myVar[4] = 22.404854 * myVar[threadIdx.x];
	var_48_0[0] = myVar[0];
	var_48_1[1] = myVar[1];
	var_48_2[2] = myVar[2];
	var_48_3[3] = myVar[3];
	var_48_4[4] = myVar[4];
	var_48_5[5] = myVar[5];
	var_48_6[6] = myVar[6];
	var_48_7[7] = myVar[7];
	var_48_8[8] = myVar[8];
	var_48_9[9] = myVar[9];
	
}

__global__ void kernel_49(float * var_49_0, float * var_49_1, float * var_49_2, float * var_49_3, float * var_49_4, float * var_49_5, float * var_49_6, float * var_49_7, float * var_49_8, float * var_49_9) {
	__shared__ float myVar[1024];
	myVar[9] = 16.530505 * myVar[threadIdx.x];
	myVar[2] = 15.127651 * myVar[threadIdx.x];
	myVar[0] = 30.241751 * myVar[threadIdx.x];
	myVar[4] = 32.781389 * myVar[threadIdx.x];
	myVar[7] = 39.703450 * myVar[threadIdx.x];
	myVar[3] = 20.524503 * myVar[threadIdx.x];
	myVar[2] = 9.988706 * myVar[threadIdx.x];
	myVar[0] = 31.878672 * myVar[threadIdx.x];
	myVar[8] = 23.459937 * myVar[threadIdx.x];
	myVar[8] = 46.195898 * myVar[threadIdx.x];
	var_49_0[0] = myVar[0];
	var_49_1[1] = myVar[1];
	var_49_2[2] = myVar[2];
	var_49_3[3] = myVar[3];
	var_49_4[4] = myVar[4];
	var_49_5[5] = myVar[5];
	var_49_6[6] = myVar[6];
	var_49_7[7] = myVar[7];
	var_49_8[8] = myVar[8];
	var_49_9[9] = myVar[9];
	
}

__global__ void kernel_50(float * var_50_0, float * var_50_1, float * var_50_2, float * var_50_3, float * var_50_4, float * var_50_5, float * var_50_6, float * var_50_7, float * var_50_8, float * var_50_9) {
	__shared__ float myVar[1024];
	myVar[4] = 5.325346 * myVar[threadIdx.x];
	myVar[6] = 15.725661 * myVar[threadIdx.x];
	myVar[0] = 13.795713 * myVar[threadIdx.x];
	myVar[4] = 37.816785 * myVar[threadIdx.x];
	myVar[0] = 24.448054 * myVar[threadIdx.x];
	myVar[8] = 13.825842 * myVar[threadIdx.x];
	myVar[6] = 3.172842 * myVar[threadIdx.x];
	myVar[0] = 20.339939 * myVar[threadIdx.x];
	myVar[0] = 38.466321 * myVar[threadIdx.x];
	myVar[4] = 1.731809 * myVar[threadIdx.x];
	var_50_0[0] = myVar[0];
	var_50_1[1] = myVar[1];
	var_50_2[2] = myVar[2];
	var_50_3[3] = myVar[3];
	var_50_4[4] = myVar[4];
	var_50_5[5] = myVar[5];
	var_50_6[6] = myVar[6];
	var_50_7[7] = myVar[7];
	var_50_8[8] = myVar[8];
	var_50_9[9] = myVar[9];
	
}

__global__ void kernel_51(float * var_51_0, float * var_51_1, float * var_51_2, float * var_51_3, float * var_51_4, float * var_51_5, float * var_51_6, float * var_51_7, float * var_51_8, float * var_51_9) {
	__shared__ float myVar[1024];
	myVar[6] = 19.079411 * myVar[threadIdx.x];
	myVar[9] = 12.770786 * myVar[threadIdx.x];
	myVar[6] = 45.832591 * myVar[threadIdx.x];
	myVar[2] = 23.565949 * myVar[threadIdx.x];
	myVar[1] = 13.269062 * myVar[threadIdx.x];
	myVar[4] = 29.815152 * myVar[threadIdx.x];
	myVar[2] = 47.923472 * myVar[threadIdx.x];
	myVar[2] = 25.084106 * myVar[threadIdx.x];
	myVar[1] = 9.889331 * myVar[threadIdx.x];
	myVar[1] = 25.405339 * myVar[threadIdx.x];
	var_51_0[0] = myVar[0];
	var_51_1[1] = myVar[1];
	var_51_2[2] = myVar[2];
	var_51_3[3] = myVar[3];
	var_51_4[4] = myVar[4];
	var_51_5[5] = myVar[5];
	var_51_6[6] = myVar[6];
	var_51_7[7] = myVar[7];
	var_51_8[8] = myVar[8];
	var_51_9[9] = myVar[9];
	
}

__global__ void kernel_52(float * var_52_0, float * var_52_1, float * var_52_2, float * var_52_3, float * var_52_4, float * var_52_5, float * var_52_6, float * var_52_7, float * var_52_8, float * var_52_9) {
	__shared__ float myVar[1024];
	myVar[1] = 14.796697 * myVar[threadIdx.x];
	myVar[5] = 29.243528 * myVar[threadIdx.x];
	myVar[1] = 11.022113 * myVar[threadIdx.x];
	myVar[2] = 42.219422 * myVar[threadIdx.x];
	myVar[3] = 8.393879 * myVar[threadIdx.x];
	myVar[6] = 36.936989 * myVar[threadIdx.x];
	myVar[2] = 19.929292 * myVar[threadIdx.x];
	myVar[6] = 37.384822 * myVar[threadIdx.x];
	myVar[3] = 46.113482 * myVar[threadIdx.x];
	myVar[9] = 32.640692 * myVar[threadIdx.x];
	var_52_0[0] = myVar[0];
	var_52_1[1] = myVar[1];
	var_52_2[2] = myVar[2];
	var_52_3[3] = myVar[3];
	var_52_4[4] = myVar[4];
	var_52_5[5] = myVar[5];
	var_52_6[6] = myVar[6];
	var_52_7[7] = myVar[7];
	var_52_8[8] = myVar[8];
	var_52_9[9] = myVar[9];
	
}

__global__ void kernel_53(float * var_53_0, float * var_53_1, float * var_53_2, float * var_53_3, float * var_53_4, float * var_53_5, float * var_53_6, float * var_53_7, float * var_53_8, float * var_53_9) {
	__shared__ float myVar[1024];
	myVar[2] = 10.261608 * myVar[threadIdx.x];
	myVar[5] = 28.951232 * myVar[threadIdx.x];
	myVar[6] = 24.001826 * myVar[threadIdx.x];
	myVar[0] = 43.966242 * myVar[threadIdx.x];
	myVar[7] = 46.266413 * myVar[threadIdx.x];
	myVar[7] = 19.628547 * myVar[threadIdx.x];
	myVar[5] = 3.449005 * myVar[threadIdx.x];
	myVar[7] = 13.980082 * myVar[threadIdx.x];
	myVar[6] = 47.656687 * myVar[threadIdx.x];
	myVar[3] = 14.673002 * myVar[threadIdx.x];
	var_53_0[0] = myVar[0];
	var_53_1[1] = myVar[1];
	var_53_2[2] = myVar[2];
	var_53_3[3] = myVar[3];
	var_53_4[4] = myVar[4];
	var_53_5[5] = myVar[5];
	var_53_6[6] = myVar[6];
	var_53_7[7] = myVar[7];
	var_53_8[8] = myVar[8];
	var_53_9[9] = myVar[9];
	
}

__global__ void kernel_54(float * var_54_0, float * var_54_1, float * var_54_2, float * var_54_3, float * var_54_4, float * var_54_5, float * var_54_6, float * var_54_7, float * var_54_8, float * var_54_9) {
	__shared__ float myVar[1024];
	myVar[7] = 35.271961 * myVar[threadIdx.x];
	myVar[3] = 46.033162 * myVar[threadIdx.x];
	myVar[2] = 19.404058 * myVar[threadIdx.x];
	myVar[0] = 19.280636 * myVar[threadIdx.x];
	myVar[7] = 13.225660 * myVar[threadIdx.x];
	myVar[9] = 23.648565 * myVar[threadIdx.x];
	myVar[4] = 1.204307 * myVar[threadIdx.x];
	myVar[7] = 20.344610 * myVar[threadIdx.x];
	myVar[5] = 43.198196 * myVar[threadIdx.x];
	myVar[2] = 10.681342 * myVar[threadIdx.x];
	var_54_0[0] = myVar[0];
	var_54_1[1] = myVar[1];
	var_54_2[2] = myVar[2];
	var_54_3[3] = myVar[3];
	var_54_4[4] = myVar[4];
	var_54_5[5] = myVar[5];
	var_54_6[6] = myVar[6];
	var_54_7[7] = myVar[7];
	var_54_8[8] = myVar[8];
	var_54_9[9] = myVar[9];
	
}

__global__ void kernel_55(float * var_55_0, float * var_55_1, float * var_55_2, float * var_55_3, float * var_55_4, float * var_55_5, float * var_55_6, float * var_55_7, float * var_55_8, float * var_55_9) {
	__shared__ float myVar[1024];
	myVar[4] = 19.916123 * myVar[threadIdx.x];
	myVar[4] = 22.751341 * myVar[threadIdx.x];
	myVar[5] = 44.696533 * myVar[threadIdx.x];
	myVar[5] = 24.933806 * myVar[threadIdx.x];
	myVar[9] = 25.149382 * myVar[threadIdx.x];
	myVar[5] = 9.417759 * myVar[threadIdx.x];
	myVar[1] = 17.649512 * myVar[threadIdx.x];
	myVar[6] = 19.933094 * myVar[threadIdx.x];
	myVar[6] = 7.024863 * myVar[threadIdx.x];
	myVar[1] = 27.755281 * myVar[threadIdx.x];
	var_55_0[0] = myVar[0];
	var_55_1[1] = myVar[1];
	var_55_2[2] = myVar[2];
	var_55_3[3] = myVar[3];
	var_55_4[4] = myVar[4];
	var_55_5[5] = myVar[5];
	var_55_6[6] = myVar[6];
	var_55_7[7] = myVar[7];
	var_55_8[8] = myVar[8];
	var_55_9[9] = myVar[9];
	
}

__global__ void kernel_56(float * var_56_0, float * var_56_1, float * var_56_2, float * var_56_3, float * var_56_4, float * var_56_5, float * var_56_6, float * var_56_7, float * var_56_8, float * var_56_9) {
	__shared__ float myVar[1024];
	myVar[9] = 6.687973 * myVar[threadIdx.x];
	myVar[7] = 31.218800 * myVar[threadIdx.x];
	myVar[8] = 14.682340 * myVar[threadIdx.x];
	myVar[6] = 32.591882 * myVar[threadIdx.x];
	myVar[5] = 0.628655 * myVar[threadIdx.x];
	myVar[5] = 29.086831 * myVar[threadIdx.x];
	myVar[5] = 38.344642 * myVar[threadIdx.x];
	myVar[5] = 48.892267 * myVar[threadIdx.x];
	myVar[1] = 5.500571 * myVar[threadIdx.x];
	myVar[9] = 31.552227 * myVar[threadIdx.x];
	var_56_0[0] = myVar[0];
	var_56_1[1] = myVar[1];
	var_56_2[2] = myVar[2];
	var_56_3[3] = myVar[3];
	var_56_4[4] = myVar[4];
	var_56_5[5] = myVar[5];
	var_56_6[6] = myVar[6];
	var_56_7[7] = myVar[7];
	var_56_8[8] = myVar[8];
	var_56_9[9] = myVar[9];
	
}

__global__ void kernel_57(float * var_57_0, float * var_57_1, float * var_57_2, float * var_57_3, float * var_57_4, float * var_57_5, float * var_57_6, float * var_57_7, float * var_57_8, float * var_57_9) {
	__shared__ float myVar[1024];
	myVar[0] = 42.942852 * myVar[threadIdx.x];
	myVar[7] = 16.922594 * myVar[threadIdx.x];
	myVar[5] = 25.290475 * myVar[threadIdx.x];
	myVar[3] = 42.944496 * myVar[threadIdx.x];
	myVar[8] = 36.049624 * myVar[threadIdx.x];
	myVar[3] = 21.299058 * myVar[threadIdx.x];
	myVar[7] = 35.597965 * myVar[threadIdx.x];
	myVar[2] = 10.644784 * myVar[threadIdx.x];
	myVar[6] = 48.275254 * myVar[threadIdx.x];
	myVar[2] = 24.570567 * myVar[threadIdx.x];
	var_57_0[0] = myVar[0];
	var_57_1[1] = myVar[1];
	var_57_2[2] = myVar[2];
	var_57_3[3] = myVar[3];
	var_57_4[4] = myVar[4];
	var_57_5[5] = myVar[5];
	var_57_6[6] = myVar[6];
	var_57_7[7] = myVar[7];
	var_57_8[8] = myVar[8];
	var_57_9[9] = myVar[9];
	
}

__global__ void kernel_58(float * var_58_0, float * var_58_1, float * var_58_2, float * var_58_3, float * var_58_4, float * var_58_5, float * var_58_6, float * var_58_7, float * var_58_8, float * var_58_9) {
	__shared__ float myVar[1024];
	myVar[0] = 32.557784 * myVar[threadIdx.x];
	myVar[4] = 31.142459 * myVar[threadIdx.x];
	myVar[1] = 44.341997 * myVar[threadIdx.x];
	myVar[2] = 39.714522 * myVar[threadIdx.x];
	myVar[4] = 42.604394 * myVar[threadIdx.x];
	myVar[7] = 15.058580 * myVar[threadIdx.x];
	myVar[2] = 25.976174 * myVar[threadIdx.x];
	myVar[1] = 30.940931 * myVar[threadIdx.x];
	myVar[1] = 16.873948 * myVar[threadIdx.x];
	myVar[2] = 10.993214 * myVar[threadIdx.x];
	var_58_0[0] = myVar[0];
	var_58_1[1] = myVar[1];
	var_58_2[2] = myVar[2];
	var_58_3[3] = myVar[3];
	var_58_4[4] = myVar[4];
	var_58_5[5] = myVar[5];
	var_58_6[6] = myVar[6];
	var_58_7[7] = myVar[7];
	var_58_8[8] = myVar[8];
	var_58_9[9] = myVar[9];
	
}

__global__ void kernel_59(float * var_59_0, float * var_59_1, float * var_59_2, float * var_59_3, float * var_59_4, float * var_59_5, float * var_59_6, float * var_59_7, float * var_59_8, float * var_59_9) {
	__shared__ float myVar[1024];
	myVar[3] = 1.311810 * myVar[threadIdx.x];
	myVar[5] = 28.465090 * myVar[threadIdx.x];
	myVar[0] = 15.562939 * myVar[threadIdx.x];
	myVar[5] = 18.741216 * myVar[threadIdx.x];
	myVar[4] = 33.144149 * myVar[threadIdx.x];
	myVar[7] = 33.103929 * myVar[threadIdx.x];
	myVar[1] = 22.436713 * myVar[threadIdx.x];
	myVar[6] = 5.993131 * myVar[threadIdx.x];
	myVar[4] = 10.973600 * myVar[threadIdx.x];
	myVar[2] = 17.460804 * myVar[threadIdx.x];
	var_59_0[0] = myVar[0];
	var_59_1[1] = myVar[1];
	var_59_2[2] = myVar[2];
	var_59_3[3] = myVar[3];
	var_59_4[4] = myVar[4];
	var_59_5[5] = myVar[5];
	var_59_6[6] = myVar[6];
	var_59_7[7] = myVar[7];
	var_59_8[8] = myVar[8];
	var_59_9[9] = myVar[9];
	
}

__global__ void kernel_60(float * var_60_0, float * var_60_1, float * var_60_2, float * var_60_3, float * var_60_4, float * var_60_5, float * var_60_6, float * var_60_7, float * var_60_8, float * var_60_9) {
	__shared__ float myVar[1024];
	myVar[7] = 26.745334 * myVar[threadIdx.x];
	myVar[0] = 44.518574 * myVar[threadIdx.x];
	myVar[7] = 1.071710 * myVar[threadIdx.x];
	myVar[4] = 28.570562 * myVar[threadIdx.x];
	myVar[9] = 40.192279 * myVar[threadIdx.x];
	myVar[2] = 8.392118 * myVar[threadIdx.x];
	myVar[7] = 37.779091 * myVar[threadIdx.x];
	myVar[3] = 44.950181 * myVar[threadIdx.x];
	myVar[1] = 29.133288 * myVar[threadIdx.x];
	myVar[1] = 3.291740 * myVar[threadIdx.x];
	var_60_0[0] = myVar[0];
	var_60_1[1] = myVar[1];
	var_60_2[2] = myVar[2];
	var_60_3[3] = myVar[3];
	var_60_4[4] = myVar[4];
	var_60_5[5] = myVar[5];
	var_60_6[6] = myVar[6];
	var_60_7[7] = myVar[7];
	var_60_8[8] = myVar[8];
	var_60_9[9] = myVar[9];
	
}

__global__ void kernel_61(float * var_61_0, float * var_61_1, float * var_61_2, float * var_61_3, float * var_61_4, float * var_61_5, float * var_61_6, float * var_61_7, float * var_61_8, float * var_61_9) {
	__shared__ float myVar[1024];
	myVar[7] = 36.374968 * myVar[threadIdx.x];
	myVar[7] = 47.836531 * myVar[threadIdx.x];
	myVar[3] = 28.497043 * myVar[threadIdx.x];
	myVar[1] = 3.867084 * myVar[threadIdx.x];
	myVar[0] = 33.422697 * myVar[threadIdx.x];
	myVar[4] = 9.390457 * myVar[threadIdx.x];
	myVar[3] = 34.073638 * myVar[threadIdx.x];
	myVar[6] = 31.175615 * myVar[threadIdx.x];
	myVar[0] = 29.532395 * myVar[threadIdx.x];
	myVar[7] = 9.283403 * myVar[threadIdx.x];
	var_61_0[0] = myVar[0];
	var_61_1[1] = myVar[1];
	var_61_2[2] = myVar[2];
	var_61_3[3] = myVar[3];
	var_61_4[4] = myVar[4];
	var_61_5[5] = myVar[5];
	var_61_6[6] = myVar[6];
	var_61_7[7] = myVar[7];
	var_61_8[8] = myVar[8];
	var_61_9[9] = myVar[9];
	
}

__global__ void kernel_62(float * var_62_0, float * var_62_1, float * var_62_2, float * var_62_3, float * var_62_4, float * var_62_5, float * var_62_6, float * var_62_7, float * var_62_8, float * var_62_9) {
	__shared__ float myVar[1024];
	myVar[1] = 14.789948 * myVar[threadIdx.x];
	myVar[5] = 14.691171 * myVar[threadIdx.x];
	myVar[8] = 13.428209 * myVar[threadIdx.x];
	myVar[1] = 43.424723 * myVar[threadIdx.x];
	myVar[8] = 11.275440 * myVar[threadIdx.x];
	myVar[4] = 27.078670 * myVar[threadIdx.x];
	myVar[5] = 39.230396 * myVar[threadIdx.x];
	myVar[0] = 2.988316 * myVar[threadIdx.x];
	myVar[1] = 24.087731 * myVar[threadIdx.x];
	myVar[9] = 30.846373 * myVar[threadIdx.x];
	var_62_0[0] = myVar[0];
	var_62_1[1] = myVar[1];
	var_62_2[2] = myVar[2];
	var_62_3[3] = myVar[3];
	var_62_4[4] = myVar[4];
	var_62_5[5] = myVar[5];
	var_62_6[6] = myVar[6];
	var_62_7[7] = myVar[7];
	var_62_8[8] = myVar[8];
	var_62_9[9] = myVar[9];
	
}

__global__ void kernel_63(float * var_63_0, float * var_63_1, float * var_63_2, float * var_63_3, float * var_63_4, float * var_63_5, float * var_63_6, float * var_63_7, float * var_63_8, float * var_63_9) {
	__shared__ float myVar[1024];
	myVar[5] = 3.936064 * myVar[threadIdx.x];
	myVar[3] = 47.100185 * myVar[threadIdx.x];
	myVar[0] = 37.955791 * myVar[threadIdx.x];
	myVar[8] = 48.851432 * myVar[threadIdx.x];
	myVar[2] = 10.548980 * myVar[threadIdx.x];
	myVar[6] = 22.418456 * myVar[threadIdx.x];
	myVar[6] = 32.476558 * myVar[threadIdx.x];
	myVar[1] = 12.657882 * myVar[threadIdx.x];
	myVar[3] = 41.171619 * myVar[threadIdx.x];
	myVar[8] = 14.120089 * myVar[threadIdx.x];
	var_63_0[0] = myVar[0];
	var_63_1[1] = myVar[1];
	var_63_2[2] = myVar[2];
	var_63_3[3] = myVar[3];
	var_63_4[4] = myVar[4];
	var_63_5[5] = myVar[5];
	var_63_6[6] = myVar[6];
	var_63_7[7] = myVar[7];
	var_63_8[8] = myVar[8];
	var_63_9[9] = myVar[9];
	
}

__global__ void kernel_64(float * var_64_0, float * var_64_1, float * var_64_2, float * var_64_3, float * var_64_4, float * var_64_5, float * var_64_6, float * var_64_7, float * var_64_8, float * var_64_9) {
	__shared__ float myVar[1024];
	myVar[7] = 45.565801 * myVar[threadIdx.x];
	myVar[6] = 25.062463 * myVar[threadIdx.x];
	myVar[8] = 1.728582 * myVar[threadIdx.x];
	myVar[9] = 40.347319 * myVar[threadIdx.x];
	myVar[2] = 15.007933 * myVar[threadIdx.x];
	myVar[8] = 35.658580 * myVar[threadIdx.x];
	myVar[6] = 45.459833 * myVar[threadIdx.x];
	myVar[5] = 18.062262 * myVar[threadIdx.x];
	myVar[2] = 8.765494 * myVar[threadIdx.x];
	myVar[6] = 11.171619 * myVar[threadIdx.x];
	var_64_0[0] = myVar[0];
	var_64_1[1] = myVar[1];
	var_64_2[2] = myVar[2];
	var_64_3[3] = myVar[3];
	var_64_4[4] = myVar[4];
	var_64_5[5] = myVar[5];
	var_64_6[6] = myVar[6];
	var_64_7[7] = myVar[7];
	var_64_8[8] = myVar[8];
	var_64_9[9] = myVar[9];
	
}

__global__ void kernel_65(float * var_65_0, float * var_65_1, float * var_65_2, float * var_65_3, float * var_65_4, float * var_65_5, float * var_65_6, float * var_65_7, float * var_65_8, float * var_65_9) {
	__shared__ float myVar[1024];
	myVar[5] = 30.269819 * myVar[threadIdx.x];
	myVar[8] = 32.043282 * myVar[threadIdx.x];
	myVar[5] = 45.355472 * myVar[threadIdx.x];
	myVar[8] = 33.669889 * myVar[threadIdx.x];
	myVar[4] = 23.793983 * myVar[threadIdx.x];
	myVar[4] = 18.830421 * myVar[threadIdx.x];
	myVar[4] = 48.601843 * myVar[threadIdx.x];
	myVar[8] = 5.349688 * myVar[threadIdx.x];
	myVar[6] = 40.942829 * myVar[threadIdx.x];
	myVar[0] = 15.357022 * myVar[threadIdx.x];
	var_65_0[0] = myVar[0];
	var_65_1[1] = myVar[1];
	var_65_2[2] = myVar[2];
	var_65_3[3] = myVar[3];
	var_65_4[4] = myVar[4];
	var_65_5[5] = myVar[5];
	var_65_6[6] = myVar[6];
	var_65_7[7] = myVar[7];
	var_65_8[8] = myVar[8];
	var_65_9[9] = myVar[9];
	
}

__global__ void kernel_66(float * var_66_0, float * var_66_1, float * var_66_2, float * var_66_3, float * var_66_4, float * var_66_5, float * var_66_6, float * var_66_7, float * var_66_8, float * var_66_9) {
	__shared__ float myVar[1024];
	myVar[5] = 25.266525 * myVar[threadIdx.x];
	myVar[6] = 13.470612 * myVar[threadIdx.x];
	myVar[8] = 3.439291 * myVar[threadIdx.x];
	myVar[5] = 42.202740 * myVar[threadIdx.x];
	myVar[5] = 18.426540 * myVar[threadIdx.x];
	myVar[0] = 46.380957 * myVar[threadIdx.x];
	myVar[0] = 49.348087 * myVar[threadIdx.x];
	myVar[0] = 41.588064 * myVar[threadIdx.x];
	myVar[2] = 41.296533 * myVar[threadIdx.x];
	myVar[6] = 34.181203 * myVar[threadIdx.x];
	var_66_0[0] = myVar[0];
	var_66_1[1] = myVar[1];
	var_66_2[2] = myVar[2];
	var_66_3[3] = myVar[3];
	var_66_4[4] = myVar[4];
	var_66_5[5] = myVar[5];
	var_66_6[6] = myVar[6];
	var_66_7[7] = myVar[7];
	var_66_8[8] = myVar[8];
	var_66_9[9] = myVar[9];
	
}

__global__ void kernel_67(float * var_67_0, float * var_67_1, float * var_67_2, float * var_67_3, float * var_67_4, float * var_67_5, float * var_67_6, float * var_67_7, float * var_67_8, float * var_67_9) {
	__shared__ float myVar[1024];
	myVar[0] = 38.271522 * myVar[threadIdx.x];
	myVar[8] = 31.755713 * myVar[threadIdx.x];
	myVar[9] = 7.291866 * myVar[threadIdx.x];
	myVar[1] = 14.571830 * myVar[threadIdx.x];
	myVar[3] = 11.520106 * myVar[threadIdx.x];
	myVar[9] = 5.117608 * myVar[threadIdx.x];
	myVar[5] = 21.261513 * myVar[threadIdx.x];
	myVar[6] = 20.912550 * myVar[threadIdx.x];
	myVar[3] = 8.134773 * myVar[threadIdx.x];
	myVar[2] = 0.225429 * myVar[threadIdx.x];
	var_67_0[0] = myVar[0];
	var_67_1[1] = myVar[1];
	var_67_2[2] = myVar[2];
	var_67_3[3] = myVar[3];
	var_67_4[4] = myVar[4];
	var_67_5[5] = myVar[5];
	var_67_6[6] = myVar[6];
	var_67_7[7] = myVar[7];
	var_67_8[8] = myVar[8];
	var_67_9[9] = myVar[9];
	
}

__global__ void kernel_68(float * var_68_0, float * var_68_1, float * var_68_2, float * var_68_3, float * var_68_4, float * var_68_5, float * var_68_6, float * var_68_7, float * var_68_8, float * var_68_9) {
	__shared__ float myVar[1024];
	myVar[5] = 29.808839 * myVar[threadIdx.x];
	myVar[2] = 32.892619 * myVar[threadIdx.x];
	myVar[0] = 37.725041 * myVar[threadIdx.x];
	myVar[3] = 19.956411 * myVar[threadIdx.x];
	myVar[7] = 15.362055 * myVar[threadIdx.x];
	myVar[6] = 35.670678 * myVar[threadIdx.x];
	myVar[4] = 24.520880 * myVar[threadIdx.x];
	myVar[4] = 4.538341 * myVar[threadIdx.x];
	myVar[7] = 15.441425 * myVar[threadIdx.x];
	myVar[4] = 47.349828 * myVar[threadIdx.x];
	var_68_0[0] = myVar[0];
	var_68_1[1] = myVar[1];
	var_68_2[2] = myVar[2];
	var_68_3[3] = myVar[3];
	var_68_4[4] = myVar[4];
	var_68_5[5] = myVar[5];
	var_68_6[6] = myVar[6];
	var_68_7[7] = myVar[7];
	var_68_8[8] = myVar[8];
	var_68_9[9] = myVar[9];
	
}

__global__ void kernel_69(float * var_69_0, float * var_69_1, float * var_69_2, float * var_69_3, float * var_69_4, float * var_69_5, float * var_69_6, float * var_69_7, float * var_69_8, float * var_69_9) {
	__shared__ float myVar[1024];
	myVar[6] = 15.561522 * myVar[threadIdx.x];
	myVar[3] = 28.889930 * myVar[threadIdx.x];
	myVar[0] = 42.692009 * myVar[threadIdx.x];
	myVar[8] = 48.031363 * myVar[threadIdx.x];
	myVar[0] = 40.455473 * myVar[threadIdx.x];
	myVar[5] = 17.495201 * myVar[threadIdx.x];
	myVar[2] = 45.045797 * myVar[threadIdx.x];
	myVar[3] = 41.532599 * myVar[threadIdx.x];
	myVar[6] = 44.325313 * myVar[threadIdx.x];
	myVar[5] = 17.036809 * myVar[threadIdx.x];
	var_69_0[0] = myVar[0];
	var_69_1[1] = myVar[1];
	var_69_2[2] = myVar[2];
	var_69_3[3] = myVar[3];
	var_69_4[4] = myVar[4];
	var_69_5[5] = myVar[5];
	var_69_6[6] = myVar[6];
	var_69_7[7] = myVar[7];
	var_69_8[8] = myVar[8];
	var_69_9[9] = myVar[9];
	
}

__global__ void kernel_70(float * var_70_0, float * var_70_1, float * var_70_2, float * var_70_3, float * var_70_4, float * var_70_5, float * var_70_6, float * var_70_7, float * var_70_8, float * var_70_9) {
	__shared__ float myVar[1024];
	myVar[4] = 43.822753 * myVar[threadIdx.x];
	myVar[3] = 45.664721 * myVar[threadIdx.x];
	myVar[5] = 18.776748 * myVar[threadIdx.x];
	myVar[2] = 31.794177 * myVar[threadIdx.x];
	myVar[8] = 5.619331 * myVar[threadIdx.x];
	myVar[5] = 12.781326 * myVar[threadIdx.x];
	myVar[0] = 0.490978 * myVar[threadIdx.x];
	myVar[6] = 23.569952 * myVar[threadIdx.x];
	myVar[5] = 35.965219 * myVar[threadIdx.x];
	myVar[3] = 46.658023 * myVar[threadIdx.x];
	var_70_0[0] = myVar[0];
	var_70_1[1] = myVar[1];
	var_70_2[2] = myVar[2];
	var_70_3[3] = myVar[3];
	var_70_4[4] = myVar[4];
	var_70_5[5] = myVar[5];
	var_70_6[6] = myVar[6];
	var_70_7[7] = myVar[7];
	var_70_8[8] = myVar[8];
	var_70_9[9] = myVar[9];
	
}

__global__ void kernel_71(float * var_71_0, float * var_71_1, float * var_71_2, float * var_71_3, float * var_71_4, float * var_71_5, float * var_71_6, float * var_71_7, float * var_71_8, float * var_71_9) {
	__shared__ float myVar[1024];
	myVar[4] = 44.703040 * myVar[threadIdx.x];
	myVar[0] = 3.950518 * myVar[threadIdx.x];
	myVar[7] = 19.348442 * myVar[threadIdx.x];
	myVar[2] = 30.184186 * myVar[threadIdx.x];
	myVar[5] = 18.387463 * myVar[threadIdx.x];
	myVar[1] = 28.608455 * myVar[threadIdx.x];
	myVar[9] = 25.229606 * myVar[threadIdx.x];
	myVar[2] = 6.512417 * myVar[threadIdx.x];
	myVar[6] = 21.735847 * myVar[threadIdx.x];
	myVar[2] = 9.153115 * myVar[threadIdx.x];
	var_71_0[0] = myVar[0];
	var_71_1[1] = myVar[1];
	var_71_2[2] = myVar[2];
	var_71_3[3] = myVar[3];
	var_71_4[4] = myVar[4];
	var_71_5[5] = myVar[5];
	var_71_6[6] = myVar[6];
	var_71_7[7] = myVar[7];
	var_71_8[8] = myVar[8];
	var_71_9[9] = myVar[9];
	
}

__global__ void kernel_72(float * var_72_0, float * var_72_1, float * var_72_2, float * var_72_3, float * var_72_4, float * var_72_5, float * var_72_6, float * var_72_7, float * var_72_8, float * var_72_9) {
	__shared__ float myVar[1024];
	myVar[0] = 27.560063 * myVar[threadIdx.x];
	myVar[6] = 34.444489 * myVar[threadIdx.x];
	myVar[4] = 5.275940 * myVar[threadIdx.x];
	myVar[8] = 10.500783 * myVar[threadIdx.x];
	myVar[1] = 6.446349 * myVar[threadIdx.x];
	myVar[6] = 28.947571 * myVar[threadIdx.x];
	myVar[3] = 44.292461 * myVar[threadIdx.x];
	myVar[0] = 24.047115 * myVar[threadIdx.x];
	myVar[0] = 29.227834 * myVar[threadIdx.x];
	myVar[0] = 1.828860 * myVar[threadIdx.x];
	var_72_0[0] = myVar[0];
	var_72_1[1] = myVar[1];
	var_72_2[2] = myVar[2];
	var_72_3[3] = myVar[3];
	var_72_4[4] = myVar[4];
	var_72_5[5] = myVar[5];
	var_72_6[6] = myVar[6];
	var_72_7[7] = myVar[7];
	var_72_8[8] = myVar[8];
	var_72_9[9] = myVar[9];
	
}

__global__ void kernel_73(float * var_73_0, float * var_73_1, float * var_73_2, float * var_73_3, float * var_73_4, float * var_73_5, float * var_73_6, float * var_73_7, float * var_73_8, float * var_73_9) {
	__shared__ float myVar[1024];
	myVar[9] = 18.401364 * myVar[threadIdx.x];
	myVar[1] = 18.918785 * myVar[threadIdx.x];
	myVar[2] = 12.418437 * myVar[threadIdx.x];
	myVar[2] = 22.731725 * myVar[threadIdx.x];
	myVar[9] = 7.621444 * myVar[threadIdx.x];
	myVar[7] = 20.529277 * myVar[threadIdx.x];
	myVar[7] = 2.812689 * myVar[threadIdx.x];
	myVar[2] = 34.598437 * myVar[threadIdx.x];
	myVar[0] = 5.511729 * myVar[threadIdx.x];
	myVar[4] = 5.773224 * myVar[threadIdx.x];
	var_73_0[0] = myVar[0];
	var_73_1[1] = myVar[1];
	var_73_2[2] = myVar[2];
	var_73_3[3] = myVar[3];
	var_73_4[4] = myVar[4];
	var_73_5[5] = myVar[5];
	var_73_6[6] = myVar[6];
	var_73_7[7] = myVar[7];
	var_73_8[8] = myVar[8];
	var_73_9[9] = myVar[9];
	
}

__global__ void kernel_74(float * var_74_0, float * var_74_1, float * var_74_2, float * var_74_3, float * var_74_4, float * var_74_5, float * var_74_6, float * var_74_7, float * var_74_8, float * var_74_9) {
	__shared__ float myVar[1024];
	myVar[6] = 6.423541 * myVar[threadIdx.x];
	myVar[1] = 45.527027 * myVar[threadIdx.x];
	myVar[2] = 9.083565 * myVar[threadIdx.x];
	myVar[8] = 33.581670 * myVar[threadIdx.x];
	myVar[3] = 25.234802 * myVar[threadIdx.x];
	myVar[3] = 5.522352 * myVar[threadIdx.x];
	myVar[6] = 47.150690 * myVar[threadIdx.x];
	myVar[7] = 46.448180 * myVar[threadIdx.x];
	myVar[8] = 12.366391 * myVar[threadIdx.x];
	myVar[6] = 7.389587 * myVar[threadIdx.x];
	var_74_0[0] = myVar[0];
	var_74_1[1] = myVar[1];
	var_74_2[2] = myVar[2];
	var_74_3[3] = myVar[3];
	var_74_4[4] = myVar[4];
	var_74_5[5] = myVar[5];
	var_74_6[6] = myVar[6];
	var_74_7[7] = myVar[7];
	var_74_8[8] = myVar[8];
	var_74_9[9] = myVar[9];
	
}

__global__ void kernel_75(float * var_75_0, float * var_75_1, float * var_75_2, float * var_75_3, float * var_75_4, float * var_75_5, float * var_75_6, float * var_75_7, float * var_75_8, float * var_75_9) {
	__shared__ float myVar[1024];
	myVar[9] = 49.933159 * myVar[threadIdx.x];
	myVar[9] = 45.530373 * myVar[threadIdx.x];
	myVar[1] = 21.071016 * myVar[threadIdx.x];
	myVar[9] = 19.223554 * myVar[threadIdx.x];
	myVar[2] = 8.624469 * myVar[threadIdx.x];
	myVar[5] = 21.207931 * myVar[threadIdx.x];
	myVar[3] = 49.200149 * myVar[threadIdx.x];
	myVar[4] = 1.103956 * myVar[threadIdx.x];
	myVar[7] = 13.565424 * myVar[threadIdx.x];
	myVar[2] = 19.014096 * myVar[threadIdx.x];
	var_75_0[0] = myVar[0];
	var_75_1[1] = myVar[1];
	var_75_2[2] = myVar[2];
	var_75_3[3] = myVar[3];
	var_75_4[4] = myVar[4];
	var_75_5[5] = myVar[5];
	var_75_6[6] = myVar[6];
	var_75_7[7] = myVar[7];
	var_75_8[8] = myVar[8];
	var_75_9[9] = myVar[9];
	
}

__global__ void kernel_76(float * var_76_0, float * var_76_1, float * var_76_2, float * var_76_3, float * var_76_4, float * var_76_5, float * var_76_6, float * var_76_7, float * var_76_8, float * var_76_9) {
	__shared__ float myVar[1024];
	myVar[1] = 13.045966 * myVar[threadIdx.x];
	myVar[9] = 38.295206 * myVar[threadIdx.x];
	myVar[1] = 16.186648 * myVar[threadIdx.x];
	myVar[9] = 39.560001 * myVar[threadIdx.x];
	myVar[5] = 32.686363 * myVar[threadIdx.x];
	myVar[6] = 29.674898 * myVar[threadIdx.x];
	myVar[2] = 29.036698 * myVar[threadIdx.x];
	myVar[4] = 18.136690 * myVar[threadIdx.x];
	myVar[7] = 37.859740 * myVar[threadIdx.x];
	myVar[5] = 39.406620 * myVar[threadIdx.x];
	var_76_0[0] = myVar[0];
	var_76_1[1] = myVar[1];
	var_76_2[2] = myVar[2];
	var_76_3[3] = myVar[3];
	var_76_4[4] = myVar[4];
	var_76_5[5] = myVar[5];
	var_76_6[6] = myVar[6];
	var_76_7[7] = myVar[7];
	var_76_8[8] = myVar[8];
	var_76_9[9] = myVar[9];
	
}

__global__ void kernel_77(float * var_77_0, float * var_77_1, float * var_77_2, float * var_77_3, float * var_77_4, float * var_77_5, float * var_77_6, float * var_77_7, float * var_77_8, float * var_77_9) {
	__shared__ float myVar[1024];
	myVar[3] = 13.892454 * myVar[threadIdx.x];
	myVar[4] = 15.717451 * myVar[threadIdx.x];
	myVar[2] = 45.974949 * myVar[threadIdx.x];
	myVar[2] = 26.309925 * myVar[threadIdx.x];
	myVar[4] = 42.529810 * myVar[threadIdx.x];
	myVar[8] = 26.425277 * myVar[threadIdx.x];
	myVar[3] = 32.305462 * myVar[threadIdx.x];
	myVar[1] = 8.888775 * myVar[threadIdx.x];
	myVar[5] = 30.602284 * myVar[threadIdx.x];
	myVar[3] = 47.799063 * myVar[threadIdx.x];
	var_77_0[0] = myVar[0];
	var_77_1[1] = myVar[1];
	var_77_2[2] = myVar[2];
	var_77_3[3] = myVar[3];
	var_77_4[4] = myVar[4];
	var_77_5[5] = myVar[5];
	var_77_6[6] = myVar[6];
	var_77_7[7] = myVar[7];
	var_77_8[8] = myVar[8];
	var_77_9[9] = myVar[9];
	
}

__global__ void kernel_78(float * var_78_0, float * var_78_1, float * var_78_2, float * var_78_3, float * var_78_4, float * var_78_5, float * var_78_6, float * var_78_7, float * var_78_8, float * var_78_9) {
	__shared__ float myVar[1024];
	myVar[2] = 5.436115 * myVar[threadIdx.x];
	myVar[3] = 10.161375 * myVar[threadIdx.x];
	myVar[8] = 46.097263 * myVar[threadIdx.x];
	myVar[3] = 0.070710 * myVar[threadIdx.x];
	myVar[3] = 5.119169 * myVar[threadIdx.x];
	myVar[2] = 46.053299 * myVar[threadIdx.x];
	myVar[4] = 13.764615 * myVar[threadIdx.x];
	myVar[6] = 37.530775 * myVar[threadIdx.x];
	myVar[7] = 1.313748 * myVar[threadIdx.x];
	myVar[7] = 17.369290 * myVar[threadIdx.x];
	var_78_0[0] = myVar[0];
	var_78_1[1] = myVar[1];
	var_78_2[2] = myVar[2];
	var_78_3[3] = myVar[3];
	var_78_4[4] = myVar[4];
	var_78_5[5] = myVar[5];
	var_78_6[6] = myVar[6];
	var_78_7[7] = myVar[7];
	var_78_8[8] = myVar[8];
	var_78_9[9] = myVar[9];
	
}

__global__ void kernel_79(float * var_79_0, float * var_79_1, float * var_79_2, float * var_79_3, float * var_79_4, float * var_79_5, float * var_79_6, float * var_79_7, float * var_79_8, float * var_79_9) {
	__shared__ float myVar[1024];
	myVar[1] = 48.598773 * myVar[threadIdx.x];
	myVar[4] = 23.854089 * myVar[threadIdx.x];
	myVar[1] = 44.375970 * myVar[threadIdx.x];
	myVar[7] = 47.116124 * myVar[threadIdx.x];
	myVar[6] = 46.363602 * myVar[threadIdx.x];
	myVar[7] = 38.127347 * myVar[threadIdx.x];
	myVar[3] = 37.386352 * myVar[threadIdx.x];
	myVar[4] = 19.948166 * myVar[threadIdx.x];
	myVar[8] = 41.688540 * myVar[threadIdx.x];
	myVar[2] = 30.193864 * myVar[threadIdx.x];
	var_79_0[0] = myVar[0];
	var_79_1[1] = myVar[1];
	var_79_2[2] = myVar[2];
	var_79_3[3] = myVar[3];
	var_79_4[4] = myVar[4];
	var_79_5[5] = myVar[5];
	var_79_6[6] = myVar[6];
	var_79_7[7] = myVar[7];
	var_79_8[8] = myVar[8];
	var_79_9[9] = myVar[9];
	
}


int main(void) {
	
	float * h_var_0_0 = (float *)malloc(sizeof(float *));
	float * d_var_0_0;
	cudaMalloc((void **)&d_var_0_0, sizeof(float *));
	
	float * h_var_0_1 = (float *)malloc(sizeof(float *));
	float * d_var_0_1;
	cudaMalloc((void **)&d_var_0_1, sizeof(float *));
	
	float * h_var_0_2 = (float *)malloc(sizeof(float *));
	float * d_var_0_2;
	cudaMalloc((void **)&d_var_0_2, sizeof(float *));
	
	float * h_var_0_3 = (float *)malloc(sizeof(float *));
	float * d_var_0_3;
	cudaMalloc((void **)&d_var_0_3, sizeof(float *));
	
	float * h_var_0_4 = (float *)malloc(sizeof(float *));
	float * d_var_0_4;
	cudaMalloc((void **)&d_var_0_4, sizeof(float *));
	
	float * h_var_0_5 = (float *)malloc(sizeof(float *));
	float * d_var_0_5;
	cudaMalloc((void **)&d_var_0_5, sizeof(float *));
	
	float * h_var_0_6 = (float *)malloc(sizeof(float *));
	float * d_var_0_6;
	cudaMalloc((void **)&d_var_0_6, sizeof(float *));
	
	float * h_var_0_7 = (float *)malloc(sizeof(float *));
	float * d_var_0_7;
	cudaMalloc((void **)&d_var_0_7, sizeof(float *));
	
	float * h_var_0_8 = (float *)malloc(sizeof(float *));
	float * d_var_0_8;
	cudaMalloc((void **)&d_var_0_8, sizeof(float *));
	
	float * h_var_0_9 = (float *)malloc(sizeof(float *));
	float * d_var_0_9;
	cudaMalloc((void **)&d_var_0_9, sizeof(float *));
	
	float * h_var_1_0 = (float *)malloc(sizeof(float *));
	float * d_var_1_0;
	cudaMalloc((void **)&d_var_1_0, sizeof(float *));
	
	float * h_var_1_1 = (float *)malloc(sizeof(float *));
	float * d_var_1_1;
	cudaMalloc((void **)&d_var_1_1, sizeof(float *));
	
	float * h_var_1_2 = (float *)malloc(sizeof(float *));
	float * d_var_1_2;
	cudaMalloc((void **)&d_var_1_2, sizeof(float *));
	
	float * h_var_1_3 = (float *)malloc(sizeof(float *));
	float * d_var_1_3;
	cudaMalloc((void **)&d_var_1_3, sizeof(float *));
	
	float * h_var_1_4 = (float *)malloc(sizeof(float *));
	float * d_var_1_4;
	cudaMalloc((void **)&d_var_1_4, sizeof(float *));
	
	float * h_var_1_5 = (float *)malloc(sizeof(float *));
	float * d_var_1_5;
	cudaMalloc((void **)&d_var_1_5, sizeof(float *));
	
	float * h_var_1_6 = (float *)malloc(sizeof(float *));
	float * d_var_1_6;
	cudaMalloc((void **)&d_var_1_6, sizeof(float *));
	
	float * h_var_1_7 = (float *)malloc(sizeof(float *));
	float * d_var_1_7;
	cudaMalloc((void **)&d_var_1_7, sizeof(float *));
	
	float * h_var_1_8 = (float *)malloc(sizeof(float *));
	float * d_var_1_8;
	cudaMalloc((void **)&d_var_1_8, sizeof(float *));
	
	float * h_var_1_9 = (float *)malloc(sizeof(float *));
	float * d_var_1_9;
	cudaMalloc((void **)&d_var_1_9, sizeof(float *));
	
	float * h_var_2_0 = (float *)malloc(sizeof(float *));
	float * d_var_2_0;
	cudaMalloc((void **)&d_var_2_0, sizeof(float *));
	
	float * h_var_2_1 = (float *)malloc(sizeof(float *));
	float * d_var_2_1;
	cudaMalloc((void **)&d_var_2_1, sizeof(float *));
	
	float * h_var_2_2 = (float *)malloc(sizeof(float *));
	float * d_var_2_2;
	cudaMalloc((void **)&d_var_2_2, sizeof(float *));
	
	float * h_var_2_3 = (float *)malloc(sizeof(float *));
	float * d_var_2_3;
	cudaMalloc((void **)&d_var_2_3, sizeof(float *));
	
	float * h_var_2_4 = (float *)malloc(sizeof(float *));
	float * d_var_2_4;
	cudaMalloc((void **)&d_var_2_4, sizeof(float *));
	
	float * h_var_2_5 = (float *)malloc(sizeof(float *));
	float * d_var_2_5;
	cudaMalloc((void **)&d_var_2_5, sizeof(float *));
	
	float * h_var_2_6 = (float *)malloc(sizeof(float *));
	float * d_var_2_6;
	cudaMalloc((void **)&d_var_2_6, sizeof(float *));
	
	float * h_var_2_7 = (float *)malloc(sizeof(float *));
	float * d_var_2_7;
	cudaMalloc((void **)&d_var_2_7, sizeof(float *));
	
	float * h_var_2_8 = (float *)malloc(sizeof(float *));
	float * d_var_2_8;
	cudaMalloc((void **)&d_var_2_8, sizeof(float *));
	
	float * h_var_2_9 = (float *)malloc(sizeof(float *));
	float * d_var_2_9;
	cudaMalloc((void **)&d_var_2_9, sizeof(float *));
	
	float * h_var_3_0 = (float *)malloc(sizeof(float *));
	float * d_var_3_0;
	cudaMalloc((void **)&d_var_3_0, sizeof(float *));
	
	float * h_var_3_1 = (float *)malloc(sizeof(float *));
	float * d_var_3_1;
	cudaMalloc((void **)&d_var_3_1, sizeof(float *));
	
	float * h_var_3_2 = (float *)malloc(sizeof(float *));
	float * d_var_3_2;
	cudaMalloc((void **)&d_var_3_2, sizeof(float *));
	
	float * h_var_3_3 = (float *)malloc(sizeof(float *));
	float * d_var_3_3;
	cudaMalloc((void **)&d_var_3_3, sizeof(float *));
	
	float * h_var_3_4 = (float *)malloc(sizeof(float *));
	float * d_var_3_4;
	cudaMalloc((void **)&d_var_3_4, sizeof(float *));
	
	float * h_var_3_5 = (float *)malloc(sizeof(float *));
	float * d_var_3_5;
	cudaMalloc((void **)&d_var_3_5, sizeof(float *));
	
	float * h_var_3_6 = (float *)malloc(sizeof(float *));
	float * d_var_3_6;
	cudaMalloc((void **)&d_var_3_6, sizeof(float *));
	
	float * h_var_3_7 = (float *)malloc(sizeof(float *));
	float * d_var_3_7;
	cudaMalloc((void **)&d_var_3_7, sizeof(float *));
	
	float * h_var_3_8 = (float *)malloc(sizeof(float *));
	float * d_var_3_8;
	cudaMalloc((void **)&d_var_3_8, sizeof(float *));
	
	float * h_var_3_9 = (float *)malloc(sizeof(float *));
	float * d_var_3_9;
	cudaMalloc((void **)&d_var_3_9, sizeof(float *));
	
	float * h_var_4_0 = (float *)malloc(sizeof(float *));
	float * d_var_4_0;
	cudaMalloc((void **)&d_var_4_0, sizeof(float *));
	
	float * h_var_4_1 = (float *)malloc(sizeof(float *));
	float * d_var_4_1;
	cudaMalloc((void **)&d_var_4_1, sizeof(float *));
	
	float * h_var_4_2 = (float *)malloc(sizeof(float *));
	float * d_var_4_2;
	cudaMalloc((void **)&d_var_4_2, sizeof(float *));
	
	float * h_var_4_3 = (float *)malloc(sizeof(float *));
	float * d_var_4_3;
	cudaMalloc((void **)&d_var_4_3, sizeof(float *));
	
	float * h_var_4_4 = (float *)malloc(sizeof(float *));
	float * d_var_4_4;
	cudaMalloc((void **)&d_var_4_4, sizeof(float *));
	
	float * h_var_4_5 = (float *)malloc(sizeof(float *));
	float * d_var_4_5;
	cudaMalloc((void **)&d_var_4_5, sizeof(float *));
	
	float * h_var_4_6 = (float *)malloc(sizeof(float *));
	float * d_var_4_6;
	cudaMalloc((void **)&d_var_4_6, sizeof(float *));
	
	float * h_var_4_7 = (float *)malloc(sizeof(float *));
	float * d_var_4_7;
	cudaMalloc((void **)&d_var_4_7, sizeof(float *));
	
	float * h_var_4_8 = (float *)malloc(sizeof(float *));
	float * d_var_4_8;
	cudaMalloc((void **)&d_var_4_8, sizeof(float *));
	
	float * h_var_4_9 = (float *)malloc(sizeof(float *));
	float * d_var_4_9;
	cudaMalloc((void **)&d_var_4_9, sizeof(float *));
	
	float * h_var_5_0 = (float *)malloc(sizeof(float *));
	float * d_var_5_0;
	cudaMalloc((void **)&d_var_5_0, sizeof(float *));
	
	float * h_var_5_1 = (float *)malloc(sizeof(float *));
	float * d_var_5_1;
	cudaMalloc((void **)&d_var_5_1, sizeof(float *));
	
	float * h_var_5_2 = (float *)malloc(sizeof(float *));
	float * d_var_5_2;
	cudaMalloc((void **)&d_var_5_2, sizeof(float *));
	
	float * h_var_5_3 = (float *)malloc(sizeof(float *));
	float * d_var_5_3;
	cudaMalloc((void **)&d_var_5_3, sizeof(float *));
	
	float * h_var_5_4 = (float *)malloc(sizeof(float *));
	float * d_var_5_4;
	cudaMalloc((void **)&d_var_5_4, sizeof(float *));
	
	float * h_var_5_5 = (float *)malloc(sizeof(float *));
	float * d_var_5_5;
	cudaMalloc((void **)&d_var_5_5, sizeof(float *));
	
	float * h_var_5_6 = (float *)malloc(sizeof(float *));
	float * d_var_5_6;
	cudaMalloc((void **)&d_var_5_6, sizeof(float *));
	
	float * h_var_5_7 = (float *)malloc(sizeof(float *));
	float * d_var_5_7;
	cudaMalloc((void **)&d_var_5_7, sizeof(float *));
	
	float * h_var_5_8 = (float *)malloc(sizeof(float *));
	float * d_var_5_8;
	cudaMalloc((void **)&d_var_5_8, sizeof(float *));
	
	float * h_var_5_9 = (float *)malloc(sizeof(float *));
	float * d_var_5_9;
	cudaMalloc((void **)&d_var_5_9, sizeof(float *));
	
	float * h_var_6_0 = (float *)malloc(sizeof(float *));
	float * d_var_6_0;
	cudaMalloc((void **)&d_var_6_0, sizeof(float *));
	
	float * h_var_6_1 = (float *)malloc(sizeof(float *));
	float * d_var_6_1;
	cudaMalloc((void **)&d_var_6_1, sizeof(float *));
	
	float * h_var_6_2 = (float *)malloc(sizeof(float *));
	float * d_var_6_2;
	cudaMalloc((void **)&d_var_6_2, sizeof(float *));
	
	float * h_var_6_3 = (float *)malloc(sizeof(float *));
	float * d_var_6_3;
	cudaMalloc((void **)&d_var_6_3, sizeof(float *));
	
	float * h_var_6_4 = (float *)malloc(sizeof(float *));
	float * d_var_6_4;
	cudaMalloc((void **)&d_var_6_4, sizeof(float *));
	
	float * h_var_6_5 = (float *)malloc(sizeof(float *));
	float * d_var_6_5;
	cudaMalloc((void **)&d_var_6_5, sizeof(float *));
	
	float * h_var_6_6 = (float *)malloc(sizeof(float *));
	float * d_var_6_6;
	cudaMalloc((void **)&d_var_6_6, sizeof(float *));
	
	float * h_var_6_7 = (float *)malloc(sizeof(float *));
	float * d_var_6_7;
	cudaMalloc((void **)&d_var_6_7, sizeof(float *));
	
	float * h_var_6_8 = (float *)malloc(sizeof(float *));
	float * d_var_6_8;
	cudaMalloc((void **)&d_var_6_8, sizeof(float *));
	
	float * h_var_6_9 = (float *)malloc(sizeof(float *));
	float * d_var_6_9;
	cudaMalloc((void **)&d_var_6_9, sizeof(float *));
	
	float * h_var_7_0 = (float *)malloc(sizeof(float *));
	float * d_var_7_0;
	cudaMalloc((void **)&d_var_7_0, sizeof(float *));
	
	float * h_var_7_1 = (float *)malloc(sizeof(float *));
	float * d_var_7_1;
	cudaMalloc((void **)&d_var_7_1, sizeof(float *));
	
	float * h_var_7_2 = (float *)malloc(sizeof(float *));
	float * d_var_7_2;
	cudaMalloc((void **)&d_var_7_2, sizeof(float *));
	
	float * h_var_7_3 = (float *)malloc(sizeof(float *));
	float * d_var_7_3;
	cudaMalloc((void **)&d_var_7_3, sizeof(float *));
	
	float * h_var_7_4 = (float *)malloc(sizeof(float *));
	float * d_var_7_4;
	cudaMalloc((void **)&d_var_7_4, sizeof(float *));
	
	float * h_var_7_5 = (float *)malloc(sizeof(float *));
	float * d_var_7_5;
	cudaMalloc((void **)&d_var_7_5, sizeof(float *));
	
	float * h_var_7_6 = (float *)malloc(sizeof(float *));
	float * d_var_7_6;
	cudaMalloc((void **)&d_var_7_6, sizeof(float *));
	
	float * h_var_7_7 = (float *)malloc(sizeof(float *));
	float * d_var_7_7;
	cudaMalloc((void **)&d_var_7_7, sizeof(float *));
	
	float * h_var_7_8 = (float *)malloc(sizeof(float *));
	float * d_var_7_8;
	cudaMalloc((void **)&d_var_7_8, sizeof(float *));
	
	float * h_var_7_9 = (float *)malloc(sizeof(float *));
	float * d_var_7_9;
	cudaMalloc((void **)&d_var_7_9, sizeof(float *));
	
	float * h_var_8_0 = (float *)malloc(sizeof(float *));
	float * d_var_8_0;
	cudaMalloc((void **)&d_var_8_0, sizeof(float *));
	
	float * h_var_8_1 = (float *)malloc(sizeof(float *));
	float * d_var_8_1;
	cudaMalloc((void **)&d_var_8_1, sizeof(float *));
	
	float * h_var_8_2 = (float *)malloc(sizeof(float *));
	float * d_var_8_2;
	cudaMalloc((void **)&d_var_8_2, sizeof(float *));
	
	float * h_var_8_3 = (float *)malloc(sizeof(float *));
	float * d_var_8_3;
	cudaMalloc((void **)&d_var_8_3, sizeof(float *));
	
	float * h_var_8_4 = (float *)malloc(sizeof(float *));
	float * d_var_8_4;
	cudaMalloc((void **)&d_var_8_4, sizeof(float *));
	
	float * h_var_8_5 = (float *)malloc(sizeof(float *));
	float * d_var_8_5;
	cudaMalloc((void **)&d_var_8_5, sizeof(float *));
	
	float * h_var_8_6 = (float *)malloc(sizeof(float *));
	float * d_var_8_6;
	cudaMalloc((void **)&d_var_8_6, sizeof(float *));
	
	float * h_var_8_7 = (float *)malloc(sizeof(float *));
	float * d_var_8_7;
	cudaMalloc((void **)&d_var_8_7, sizeof(float *));
	
	float * h_var_8_8 = (float *)malloc(sizeof(float *));
	float * d_var_8_8;
	cudaMalloc((void **)&d_var_8_8, sizeof(float *));
	
	float * h_var_8_9 = (float *)malloc(sizeof(float *));
	float * d_var_8_9;
	cudaMalloc((void **)&d_var_8_9, sizeof(float *));
	
	float * h_var_9_0 = (float *)malloc(sizeof(float *));
	float * d_var_9_0;
	cudaMalloc((void **)&d_var_9_0, sizeof(float *));
	
	float * h_var_9_1 = (float *)malloc(sizeof(float *));
	float * d_var_9_1;
	cudaMalloc((void **)&d_var_9_1, sizeof(float *));
	
	float * h_var_9_2 = (float *)malloc(sizeof(float *));
	float * d_var_9_2;
	cudaMalloc((void **)&d_var_9_2, sizeof(float *));
	
	float * h_var_9_3 = (float *)malloc(sizeof(float *));
	float * d_var_9_3;
	cudaMalloc((void **)&d_var_9_3, sizeof(float *));
	
	float * h_var_9_4 = (float *)malloc(sizeof(float *));
	float * d_var_9_4;
	cudaMalloc((void **)&d_var_9_4, sizeof(float *));
	
	float * h_var_9_5 = (float *)malloc(sizeof(float *));
	float * d_var_9_5;
	cudaMalloc((void **)&d_var_9_5, sizeof(float *));
	
	float * h_var_9_6 = (float *)malloc(sizeof(float *));
	float * d_var_9_6;
	cudaMalloc((void **)&d_var_9_6, sizeof(float *));
	
	float * h_var_9_7 = (float *)malloc(sizeof(float *));
	float * d_var_9_7;
	cudaMalloc((void **)&d_var_9_7, sizeof(float *));
	
	float * h_var_9_8 = (float *)malloc(sizeof(float *));
	float * d_var_9_8;
	cudaMalloc((void **)&d_var_9_8, sizeof(float *));
	
	float * h_var_9_9 = (float *)malloc(sizeof(float *));
	float * d_var_9_9;
	cudaMalloc((void **)&d_var_9_9, sizeof(float *));
	
	float * h_var_10_0 = (float *)malloc(sizeof(float *));
	float * d_var_10_0;
	cudaMalloc((void **)&d_var_10_0, sizeof(float *));
	
	float * h_var_10_1 = (float *)malloc(sizeof(float *));
	float * d_var_10_1;
	cudaMalloc((void **)&d_var_10_1, sizeof(float *));
	
	float * h_var_10_2 = (float *)malloc(sizeof(float *));
	float * d_var_10_2;
	cudaMalloc((void **)&d_var_10_2, sizeof(float *));
	
	float * h_var_10_3 = (float *)malloc(sizeof(float *));
	float * d_var_10_3;
	cudaMalloc((void **)&d_var_10_3, sizeof(float *));
	
	float * h_var_10_4 = (float *)malloc(sizeof(float *));
	float * d_var_10_4;
	cudaMalloc((void **)&d_var_10_4, sizeof(float *));
	
	float * h_var_10_5 = (float *)malloc(sizeof(float *));
	float * d_var_10_5;
	cudaMalloc((void **)&d_var_10_5, sizeof(float *));
	
	float * h_var_10_6 = (float *)malloc(sizeof(float *));
	float * d_var_10_6;
	cudaMalloc((void **)&d_var_10_6, sizeof(float *));
	
	float * h_var_10_7 = (float *)malloc(sizeof(float *));
	float * d_var_10_7;
	cudaMalloc((void **)&d_var_10_7, sizeof(float *));
	
	float * h_var_10_8 = (float *)malloc(sizeof(float *));
	float * d_var_10_8;
	cudaMalloc((void **)&d_var_10_8, sizeof(float *));
	
	float * h_var_10_9 = (float *)malloc(sizeof(float *));
	float * d_var_10_9;
	cudaMalloc((void **)&d_var_10_9, sizeof(float *));
	
	float * h_var_11_0 = (float *)malloc(sizeof(float *));
	float * d_var_11_0;
	cudaMalloc((void **)&d_var_11_0, sizeof(float *));
	
	float * h_var_11_1 = (float *)malloc(sizeof(float *));
	float * d_var_11_1;
	cudaMalloc((void **)&d_var_11_1, sizeof(float *));
	
	float * h_var_11_2 = (float *)malloc(sizeof(float *));
	float * d_var_11_2;
	cudaMalloc((void **)&d_var_11_2, sizeof(float *));
	
	float * h_var_11_3 = (float *)malloc(sizeof(float *));
	float * d_var_11_3;
	cudaMalloc((void **)&d_var_11_3, sizeof(float *));
	
	float * h_var_11_4 = (float *)malloc(sizeof(float *));
	float * d_var_11_4;
	cudaMalloc((void **)&d_var_11_4, sizeof(float *));
	
	float * h_var_11_5 = (float *)malloc(sizeof(float *));
	float * d_var_11_5;
	cudaMalloc((void **)&d_var_11_5, sizeof(float *));
	
	float * h_var_11_6 = (float *)malloc(sizeof(float *));
	float * d_var_11_6;
	cudaMalloc((void **)&d_var_11_6, sizeof(float *));
	
	float * h_var_11_7 = (float *)malloc(sizeof(float *));
	float * d_var_11_7;
	cudaMalloc((void **)&d_var_11_7, sizeof(float *));
	
	float * h_var_11_8 = (float *)malloc(sizeof(float *));
	float * d_var_11_8;
	cudaMalloc((void **)&d_var_11_8, sizeof(float *));
	
	float * h_var_11_9 = (float *)malloc(sizeof(float *));
	float * d_var_11_9;
	cudaMalloc((void **)&d_var_11_9, sizeof(float *));
	
	float * h_var_12_0 = (float *)malloc(sizeof(float *));
	float * d_var_12_0;
	cudaMalloc((void **)&d_var_12_0, sizeof(float *));
	
	float * h_var_12_1 = (float *)malloc(sizeof(float *));
	float * d_var_12_1;
	cudaMalloc((void **)&d_var_12_1, sizeof(float *));
	
	float * h_var_12_2 = (float *)malloc(sizeof(float *));
	float * d_var_12_2;
	cudaMalloc((void **)&d_var_12_2, sizeof(float *));
	
	float * h_var_12_3 = (float *)malloc(sizeof(float *));
	float * d_var_12_3;
	cudaMalloc((void **)&d_var_12_3, sizeof(float *));
	
	float * h_var_12_4 = (float *)malloc(sizeof(float *));
	float * d_var_12_4;
	cudaMalloc((void **)&d_var_12_4, sizeof(float *));
	
	float * h_var_12_5 = (float *)malloc(sizeof(float *));
	float * d_var_12_5;
	cudaMalloc((void **)&d_var_12_5, sizeof(float *));
	
	float * h_var_12_6 = (float *)malloc(sizeof(float *));
	float * d_var_12_6;
	cudaMalloc((void **)&d_var_12_6, sizeof(float *));
	
	float * h_var_12_7 = (float *)malloc(sizeof(float *));
	float * d_var_12_7;
	cudaMalloc((void **)&d_var_12_7, sizeof(float *));
	
	float * h_var_12_8 = (float *)malloc(sizeof(float *));
	float * d_var_12_8;
	cudaMalloc((void **)&d_var_12_8, sizeof(float *));
	
	float * h_var_12_9 = (float *)malloc(sizeof(float *));
	float * d_var_12_9;
	cudaMalloc((void **)&d_var_12_9, sizeof(float *));
	
	float * h_var_13_0 = (float *)malloc(sizeof(float *));
	float * d_var_13_0;
	cudaMalloc((void **)&d_var_13_0, sizeof(float *));
	
	float * h_var_13_1 = (float *)malloc(sizeof(float *));
	float * d_var_13_1;
	cudaMalloc((void **)&d_var_13_1, sizeof(float *));
	
	float * h_var_13_2 = (float *)malloc(sizeof(float *));
	float * d_var_13_2;
	cudaMalloc((void **)&d_var_13_2, sizeof(float *));
	
	float * h_var_13_3 = (float *)malloc(sizeof(float *));
	float * d_var_13_3;
	cudaMalloc((void **)&d_var_13_3, sizeof(float *));
	
	float * h_var_13_4 = (float *)malloc(sizeof(float *));
	float * d_var_13_4;
	cudaMalloc((void **)&d_var_13_4, sizeof(float *));
	
	float * h_var_13_5 = (float *)malloc(sizeof(float *));
	float * d_var_13_5;
	cudaMalloc((void **)&d_var_13_5, sizeof(float *));
	
	float * h_var_13_6 = (float *)malloc(sizeof(float *));
	float * d_var_13_6;
	cudaMalloc((void **)&d_var_13_6, sizeof(float *));
	
	float * h_var_13_7 = (float *)malloc(sizeof(float *));
	float * d_var_13_7;
	cudaMalloc((void **)&d_var_13_7, sizeof(float *));
	
	float * h_var_13_8 = (float *)malloc(sizeof(float *));
	float * d_var_13_8;
	cudaMalloc((void **)&d_var_13_8, sizeof(float *));
	
	float * h_var_13_9 = (float *)malloc(sizeof(float *));
	float * d_var_13_9;
	cudaMalloc((void **)&d_var_13_9, sizeof(float *));
	
	float * h_var_14_0 = (float *)malloc(sizeof(float *));
	float * d_var_14_0;
	cudaMalloc((void **)&d_var_14_0, sizeof(float *));
	
	float * h_var_14_1 = (float *)malloc(sizeof(float *));
	float * d_var_14_1;
	cudaMalloc((void **)&d_var_14_1, sizeof(float *));
	
	float * h_var_14_2 = (float *)malloc(sizeof(float *));
	float * d_var_14_2;
	cudaMalloc((void **)&d_var_14_2, sizeof(float *));
	
	float * h_var_14_3 = (float *)malloc(sizeof(float *));
	float * d_var_14_3;
	cudaMalloc((void **)&d_var_14_3, sizeof(float *));
	
	float * h_var_14_4 = (float *)malloc(sizeof(float *));
	float * d_var_14_4;
	cudaMalloc((void **)&d_var_14_4, sizeof(float *));
	
	float * h_var_14_5 = (float *)malloc(sizeof(float *));
	float * d_var_14_5;
	cudaMalloc((void **)&d_var_14_5, sizeof(float *));
	
	float * h_var_14_6 = (float *)malloc(sizeof(float *));
	float * d_var_14_6;
	cudaMalloc((void **)&d_var_14_6, sizeof(float *));
	
	float * h_var_14_7 = (float *)malloc(sizeof(float *));
	float * d_var_14_7;
	cudaMalloc((void **)&d_var_14_7, sizeof(float *));
	
	float * h_var_14_8 = (float *)malloc(sizeof(float *));
	float * d_var_14_8;
	cudaMalloc((void **)&d_var_14_8, sizeof(float *));
	
	float * h_var_14_9 = (float *)malloc(sizeof(float *));
	float * d_var_14_9;
	cudaMalloc((void **)&d_var_14_9, sizeof(float *));
	
	float * h_var_15_0 = (float *)malloc(sizeof(float *));
	float * d_var_15_0;
	cudaMalloc((void **)&d_var_15_0, sizeof(float *));
	
	float * h_var_15_1 = (float *)malloc(sizeof(float *));
	float * d_var_15_1;
	cudaMalloc((void **)&d_var_15_1, sizeof(float *));
	
	float * h_var_15_2 = (float *)malloc(sizeof(float *));
	float * d_var_15_2;
	cudaMalloc((void **)&d_var_15_2, sizeof(float *));
	
	float * h_var_15_3 = (float *)malloc(sizeof(float *));
	float * d_var_15_3;
	cudaMalloc((void **)&d_var_15_3, sizeof(float *));
	
	float * h_var_15_4 = (float *)malloc(sizeof(float *));
	float * d_var_15_4;
	cudaMalloc((void **)&d_var_15_4, sizeof(float *));
	
	float * h_var_15_5 = (float *)malloc(sizeof(float *));
	float * d_var_15_5;
	cudaMalloc((void **)&d_var_15_5, sizeof(float *));
	
	float * h_var_15_6 = (float *)malloc(sizeof(float *));
	float * d_var_15_6;
	cudaMalloc((void **)&d_var_15_6, sizeof(float *));
	
	float * h_var_15_7 = (float *)malloc(sizeof(float *));
	float * d_var_15_7;
	cudaMalloc((void **)&d_var_15_7, sizeof(float *));
	
	float * h_var_15_8 = (float *)malloc(sizeof(float *));
	float * d_var_15_8;
	cudaMalloc((void **)&d_var_15_8, sizeof(float *));
	
	float * h_var_15_9 = (float *)malloc(sizeof(float *));
	float * d_var_15_9;
	cudaMalloc((void **)&d_var_15_9, sizeof(float *));
	
	float * h_var_16_0 = (float *)malloc(sizeof(float *));
	float * d_var_16_0;
	cudaMalloc((void **)&d_var_16_0, sizeof(float *));
	
	float * h_var_16_1 = (float *)malloc(sizeof(float *));
	float * d_var_16_1;
	cudaMalloc((void **)&d_var_16_1, sizeof(float *));
	
	float * h_var_16_2 = (float *)malloc(sizeof(float *));
	float * d_var_16_2;
	cudaMalloc((void **)&d_var_16_2, sizeof(float *));
	
	float * h_var_16_3 = (float *)malloc(sizeof(float *));
	float * d_var_16_3;
	cudaMalloc((void **)&d_var_16_3, sizeof(float *));
	
	float * h_var_16_4 = (float *)malloc(sizeof(float *));
	float * d_var_16_4;
	cudaMalloc((void **)&d_var_16_4, sizeof(float *));
	
	float * h_var_16_5 = (float *)malloc(sizeof(float *));
	float * d_var_16_5;
	cudaMalloc((void **)&d_var_16_5, sizeof(float *));
	
	float * h_var_16_6 = (float *)malloc(sizeof(float *));
	float * d_var_16_6;
	cudaMalloc((void **)&d_var_16_6, sizeof(float *));
	
	float * h_var_16_7 = (float *)malloc(sizeof(float *));
	float * d_var_16_7;
	cudaMalloc((void **)&d_var_16_7, sizeof(float *));
	
	float * h_var_16_8 = (float *)malloc(sizeof(float *));
	float * d_var_16_8;
	cudaMalloc((void **)&d_var_16_8, sizeof(float *));
	
	float * h_var_16_9 = (float *)malloc(sizeof(float *));
	float * d_var_16_9;
	cudaMalloc((void **)&d_var_16_9, sizeof(float *));
	
	float * h_var_17_0 = (float *)malloc(sizeof(float *));
	float * d_var_17_0;
	cudaMalloc((void **)&d_var_17_0, sizeof(float *));
	
	float * h_var_17_1 = (float *)malloc(sizeof(float *));
	float * d_var_17_1;
	cudaMalloc((void **)&d_var_17_1, sizeof(float *));
	
	float * h_var_17_2 = (float *)malloc(sizeof(float *));
	float * d_var_17_2;
	cudaMalloc((void **)&d_var_17_2, sizeof(float *));
	
	float * h_var_17_3 = (float *)malloc(sizeof(float *));
	float * d_var_17_3;
	cudaMalloc((void **)&d_var_17_3, sizeof(float *));
	
	float * h_var_17_4 = (float *)malloc(sizeof(float *));
	float * d_var_17_4;
	cudaMalloc((void **)&d_var_17_4, sizeof(float *));
	
	float * h_var_17_5 = (float *)malloc(sizeof(float *));
	float * d_var_17_5;
	cudaMalloc((void **)&d_var_17_5, sizeof(float *));
	
	float * h_var_17_6 = (float *)malloc(sizeof(float *));
	float * d_var_17_6;
	cudaMalloc((void **)&d_var_17_6, sizeof(float *));
	
	float * h_var_17_7 = (float *)malloc(sizeof(float *));
	float * d_var_17_7;
	cudaMalloc((void **)&d_var_17_7, sizeof(float *));
	
	float * h_var_17_8 = (float *)malloc(sizeof(float *));
	float * d_var_17_8;
	cudaMalloc((void **)&d_var_17_8, sizeof(float *));
	
	float * h_var_17_9 = (float *)malloc(sizeof(float *));
	float * d_var_17_9;
	cudaMalloc((void **)&d_var_17_9, sizeof(float *));
	
	float * h_var_18_0 = (float *)malloc(sizeof(float *));
	float * d_var_18_0;
	cudaMalloc((void **)&d_var_18_0, sizeof(float *));
	
	float * h_var_18_1 = (float *)malloc(sizeof(float *));
	float * d_var_18_1;
	cudaMalloc((void **)&d_var_18_1, sizeof(float *));
	
	float * h_var_18_2 = (float *)malloc(sizeof(float *));
	float * d_var_18_2;
	cudaMalloc((void **)&d_var_18_2, sizeof(float *));
	
	float * h_var_18_3 = (float *)malloc(sizeof(float *));
	float * d_var_18_3;
	cudaMalloc((void **)&d_var_18_3, sizeof(float *));
	
	float * h_var_18_4 = (float *)malloc(sizeof(float *));
	float * d_var_18_4;
	cudaMalloc((void **)&d_var_18_4, sizeof(float *));
	
	float * h_var_18_5 = (float *)malloc(sizeof(float *));
	float * d_var_18_5;
	cudaMalloc((void **)&d_var_18_5, sizeof(float *));
	
	float * h_var_18_6 = (float *)malloc(sizeof(float *));
	float * d_var_18_6;
	cudaMalloc((void **)&d_var_18_6, sizeof(float *));
	
	float * h_var_18_7 = (float *)malloc(sizeof(float *));
	float * d_var_18_7;
	cudaMalloc((void **)&d_var_18_7, sizeof(float *));
	
	float * h_var_18_8 = (float *)malloc(sizeof(float *));
	float * d_var_18_8;
	cudaMalloc((void **)&d_var_18_8, sizeof(float *));
	
	float * h_var_18_9 = (float *)malloc(sizeof(float *));
	float * d_var_18_9;
	cudaMalloc((void **)&d_var_18_9, sizeof(float *));
	
	float * h_var_19_0 = (float *)malloc(sizeof(float *));
	float * d_var_19_0;
	cudaMalloc((void **)&d_var_19_0, sizeof(float *));
	
	float * h_var_19_1 = (float *)malloc(sizeof(float *));
	float * d_var_19_1;
	cudaMalloc((void **)&d_var_19_1, sizeof(float *));
	
	float * h_var_19_2 = (float *)malloc(sizeof(float *));
	float * d_var_19_2;
	cudaMalloc((void **)&d_var_19_2, sizeof(float *));
	
	float * h_var_19_3 = (float *)malloc(sizeof(float *));
	float * d_var_19_3;
	cudaMalloc((void **)&d_var_19_3, sizeof(float *));
	
	float * h_var_19_4 = (float *)malloc(sizeof(float *));
	float * d_var_19_4;
	cudaMalloc((void **)&d_var_19_4, sizeof(float *));
	
	float * h_var_19_5 = (float *)malloc(sizeof(float *));
	float * d_var_19_5;
	cudaMalloc((void **)&d_var_19_5, sizeof(float *));
	
	float * h_var_19_6 = (float *)malloc(sizeof(float *));
	float * d_var_19_6;
	cudaMalloc((void **)&d_var_19_6, sizeof(float *));
	
	float * h_var_19_7 = (float *)malloc(sizeof(float *));
	float * d_var_19_7;
	cudaMalloc((void **)&d_var_19_7, sizeof(float *));
	
	float * h_var_19_8 = (float *)malloc(sizeof(float *));
	float * d_var_19_8;
	cudaMalloc((void **)&d_var_19_8, sizeof(float *));
	
	float * h_var_19_9 = (float *)malloc(sizeof(float *));
	float * d_var_19_9;
	cudaMalloc((void **)&d_var_19_9, sizeof(float *));
	
	float * h_var_20_0 = (float *)malloc(sizeof(float *));
	float * d_var_20_0;
	cudaMalloc((void **)&d_var_20_0, sizeof(float *));
	
	float * h_var_20_1 = (float *)malloc(sizeof(float *));
	float * d_var_20_1;
	cudaMalloc((void **)&d_var_20_1, sizeof(float *));
	
	float * h_var_20_2 = (float *)malloc(sizeof(float *));
	float * d_var_20_2;
	cudaMalloc((void **)&d_var_20_2, sizeof(float *));
	
	float * h_var_20_3 = (float *)malloc(sizeof(float *));
	float * d_var_20_3;
	cudaMalloc((void **)&d_var_20_3, sizeof(float *));
	
	float * h_var_20_4 = (float *)malloc(sizeof(float *));
	float * d_var_20_4;
	cudaMalloc((void **)&d_var_20_4, sizeof(float *));
	
	float * h_var_20_5 = (float *)malloc(sizeof(float *));
	float * d_var_20_5;
	cudaMalloc((void **)&d_var_20_5, sizeof(float *));
	
	float * h_var_20_6 = (float *)malloc(sizeof(float *));
	float * d_var_20_6;
	cudaMalloc((void **)&d_var_20_6, sizeof(float *));
	
	float * h_var_20_7 = (float *)malloc(sizeof(float *));
	float * d_var_20_7;
	cudaMalloc((void **)&d_var_20_7, sizeof(float *));
	
	float * h_var_20_8 = (float *)malloc(sizeof(float *));
	float * d_var_20_8;
	cudaMalloc((void **)&d_var_20_8, sizeof(float *));
	
	float * h_var_20_9 = (float *)malloc(sizeof(float *));
	float * d_var_20_9;
	cudaMalloc((void **)&d_var_20_9, sizeof(float *));
	
	float * h_var_21_0 = (float *)malloc(sizeof(float *));
	float * d_var_21_0;
	cudaMalloc((void **)&d_var_21_0, sizeof(float *));
	
	float * h_var_21_1 = (float *)malloc(sizeof(float *));
	float * d_var_21_1;
	cudaMalloc((void **)&d_var_21_1, sizeof(float *));
	
	float * h_var_21_2 = (float *)malloc(sizeof(float *));
	float * d_var_21_2;
	cudaMalloc((void **)&d_var_21_2, sizeof(float *));
	
	float * h_var_21_3 = (float *)malloc(sizeof(float *));
	float * d_var_21_3;
	cudaMalloc((void **)&d_var_21_3, sizeof(float *));
	
	float * h_var_21_4 = (float *)malloc(sizeof(float *));
	float * d_var_21_4;
	cudaMalloc((void **)&d_var_21_4, sizeof(float *));
	
	float * h_var_21_5 = (float *)malloc(sizeof(float *));
	float * d_var_21_5;
	cudaMalloc((void **)&d_var_21_5, sizeof(float *));
	
	float * h_var_21_6 = (float *)malloc(sizeof(float *));
	float * d_var_21_6;
	cudaMalloc((void **)&d_var_21_6, sizeof(float *));
	
	float * h_var_21_7 = (float *)malloc(sizeof(float *));
	float * d_var_21_7;
	cudaMalloc((void **)&d_var_21_7, sizeof(float *));
	
	float * h_var_21_8 = (float *)malloc(sizeof(float *));
	float * d_var_21_8;
	cudaMalloc((void **)&d_var_21_8, sizeof(float *));
	
	float * h_var_21_9 = (float *)malloc(sizeof(float *));
	float * d_var_21_9;
	cudaMalloc((void **)&d_var_21_9, sizeof(float *));
	
	float * h_var_22_0 = (float *)malloc(sizeof(float *));
	float * d_var_22_0;
	cudaMalloc((void **)&d_var_22_0, sizeof(float *));
	
	float * h_var_22_1 = (float *)malloc(sizeof(float *));
	float * d_var_22_1;
	cudaMalloc((void **)&d_var_22_1, sizeof(float *));
	
	float * h_var_22_2 = (float *)malloc(sizeof(float *));
	float * d_var_22_2;
	cudaMalloc((void **)&d_var_22_2, sizeof(float *));
	
	float * h_var_22_3 = (float *)malloc(sizeof(float *));
	float * d_var_22_3;
	cudaMalloc((void **)&d_var_22_3, sizeof(float *));
	
	float * h_var_22_4 = (float *)malloc(sizeof(float *));
	float * d_var_22_4;
	cudaMalloc((void **)&d_var_22_4, sizeof(float *));
	
	float * h_var_22_5 = (float *)malloc(sizeof(float *));
	float * d_var_22_5;
	cudaMalloc((void **)&d_var_22_5, sizeof(float *));
	
	float * h_var_22_6 = (float *)malloc(sizeof(float *));
	float * d_var_22_6;
	cudaMalloc((void **)&d_var_22_6, sizeof(float *));
	
	float * h_var_22_7 = (float *)malloc(sizeof(float *));
	float * d_var_22_7;
	cudaMalloc((void **)&d_var_22_7, sizeof(float *));
	
	float * h_var_22_8 = (float *)malloc(sizeof(float *));
	float * d_var_22_8;
	cudaMalloc((void **)&d_var_22_8, sizeof(float *));
	
	float * h_var_22_9 = (float *)malloc(sizeof(float *));
	float * d_var_22_9;
	cudaMalloc((void **)&d_var_22_9, sizeof(float *));
	
	float * h_var_23_0 = (float *)malloc(sizeof(float *));
	float * d_var_23_0;
	cudaMalloc((void **)&d_var_23_0, sizeof(float *));
	
	float * h_var_23_1 = (float *)malloc(sizeof(float *));
	float * d_var_23_1;
	cudaMalloc((void **)&d_var_23_1, sizeof(float *));
	
	float * h_var_23_2 = (float *)malloc(sizeof(float *));
	float * d_var_23_2;
	cudaMalloc((void **)&d_var_23_2, sizeof(float *));
	
	float * h_var_23_3 = (float *)malloc(sizeof(float *));
	float * d_var_23_3;
	cudaMalloc((void **)&d_var_23_3, sizeof(float *));
	
	float * h_var_23_4 = (float *)malloc(sizeof(float *));
	float * d_var_23_4;
	cudaMalloc((void **)&d_var_23_4, sizeof(float *));
	
	float * h_var_23_5 = (float *)malloc(sizeof(float *));
	float * d_var_23_5;
	cudaMalloc((void **)&d_var_23_5, sizeof(float *));
	
	float * h_var_23_6 = (float *)malloc(sizeof(float *));
	float * d_var_23_6;
	cudaMalloc((void **)&d_var_23_6, sizeof(float *));
	
	float * h_var_23_7 = (float *)malloc(sizeof(float *));
	float * d_var_23_7;
	cudaMalloc((void **)&d_var_23_7, sizeof(float *));
	
	float * h_var_23_8 = (float *)malloc(sizeof(float *));
	float * d_var_23_8;
	cudaMalloc((void **)&d_var_23_8, sizeof(float *));
	
	float * h_var_23_9 = (float *)malloc(sizeof(float *));
	float * d_var_23_9;
	cudaMalloc((void **)&d_var_23_9, sizeof(float *));
	
	float * h_var_24_0 = (float *)malloc(sizeof(float *));
	float * d_var_24_0;
	cudaMalloc((void **)&d_var_24_0, sizeof(float *));
	
	float * h_var_24_1 = (float *)malloc(sizeof(float *));
	float * d_var_24_1;
	cudaMalloc((void **)&d_var_24_1, sizeof(float *));
	
	float * h_var_24_2 = (float *)malloc(sizeof(float *));
	float * d_var_24_2;
	cudaMalloc((void **)&d_var_24_2, sizeof(float *));
	
	float * h_var_24_3 = (float *)malloc(sizeof(float *));
	float * d_var_24_3;
	cudaMalloc((void **)&d_var_24_3, sizeof(float *));
	
	float * h_var_24_4 = (float *)malloc(sizeof(float *));
	float * d_var_24_4;
	cudaMalloc((void **)&d_var_24_4, sizeof(float *));
	
	float * h_var_24_5 = (float *)malloc(sizeof(float *));
	float * d_var_24_5;
	cudaMalloc((void **)&d_var_24_5, sizeof(float *));
	
	float * h_var_24_6 = (float *)malloc(sizeof(float *));
	float * d_var_24_6;
	cudaMalloc((void **)&d_var_24_6, sizeof(float *));
	
	float * h_var_24_7 = (float *)malloc(sizeof(float *));
	float * d_var_24_7;
	cudaMalloc((void **)&d_var_24_7, sizeof(float *));
	
	float * h_var_24_8 = (float *)malloc(sizeof(float *));
	float * d_var_24_8;
	cudaMalloc((void **)&d_var_24_8, sizeof(float *));
	
	float * h_var_24_9 = (float *)malloc(sizeof(float *));
	float * d_var_24_9;
	cudaMalloc((void **)&d_var_24_9, sizeof(float *));
	
	float * h_var_25_0 = (float *)malloc(sizeof(float *));
	float * d_var_25_0;
	cudaMalloc((void **)&d_var_25_0, sizeof(float *));
	
	float * h_var_25_1 = (float *)malloc(sizeof(float *));
	float * d_var_25_1;
	cudaMalloc((void **)&d_var_25_1, sizeof(float *));
	
	float * h_var_25_2 = (float *)malloc(sizeof(float *));
	float * d_var_25_2;
	cudaMalloc((void **)&d_var_25_2, sizeof(float *));
	
	float * h_var_25_3 = (float *)malloc(sizeof(float *));
	float * d_var_25_3;
	cudaMalloc((void **)&d_var_25_3, sizeof(float *));
	
	float * h_var_25_4 = (float *)malloc(sizeof(float *));
	float * d_var_25_4;
	cudaMalloc((void **)&d_var_25_4, sizeof(float *));
	
	float * h_var_25_5 = (float *)malloc(sizeof(float *));
	float * d_var_25_5;
	cudaMalloc((void **)&d_var_25_5, sizeof(float *));
	
	float * h_var_25_6 = (float *)malloc(sizeof(float *));
	float * d_var_25_6;
	cudaMalloc((void **)&d_var_25_6, sizeof(float *));
	
	float * h_var_25_7 = (float *)malloc(sizeof(float *));
	float * d_var_25_7;
	cudaMalloc((void **)&d_var_25_7, sizeof(float *));
	
	float * h_var_25_8 = (float *)malloc(sizeof(float *));
	float * d_var_25_8;
	cudaMalloc((void **)&d_var_25_8, sizeof(float *));
	
	float * h_var_25_9 = (float *)malloc(sizeof(float *));
	float * d_var_25_9;
	cudaMalloc((void **)&d_var_25_9, sizeof(float *));
	
	float * h_var_26_0 = (float *)malloc(sizeof(float *));
	float * d_var_26_0;
	cudaMalloc((void **)&d_var_26_0, sizeof(float *));
	
	float * h_var_26_1 = (float *)malloc(sizeof(float *));
	float * d_var_26_1;
	cudaMalloc((void **)&d_var_26_1, sizeof(float *));
	
	float * h_var_26_2 = (float *)malloc(sizeof(float *));
	float * d_var_26_2;
	cudaMalloc((void **)&d_var_26_2, sizeof(float *));
	
	float * h_var_26_3 = (float *)malloc(sizeof(float *));
	float * d_var_26_3;
	cudaMalloc((void **)&d_var_26_3, sizeof(float *));
	
	float * h_var_26_4 = (float *)malloc(sizeof(float *));
	float * d_var_26_4;
	cudaMalloc((void **)&d_var_26_4, sizeof(float *));
	
	float * h_var_26_5 = (float *)malloc(sizeof(float *));
	float * d_var_26_5;
	cudaMalloc((void **)&d_var_26_5, sizeof(float *));
	
	float * h_var_26_6 = (float *)malloc(sizeof(float *));
	float * d_var_26_6;
	cudaMalloc((void **)&d_var_26_6, sizeof(float *));
	
	float * h_var_26_7 = (float *)malloc(sizeof(float *));
	float * d_var_26_7;
	cudaMalloc((void **)&d_var_26_7, sizeof(float *));
	
	float * h_var_26_8 = (float *)malloc(sizeof(float *));
	float * d_var_26_8;
	cudaMalloc((void **)&d_var_26_8, sizeof(float *));
	
	float * h_var_26_9 = (float *)malloc(sizeof(float *));
	float * d_var_26_9;
	cudaMalloc((void **)&d_var_26_9, sizeof(float *));
	
	float * h_var_27_0 = (float *)malloc(sizeof(float *));
	float * d_var_27_0;
	cudaMalloc((void **)&d_var_27_0, sizeof(float *));
	
	float * h_var_27_1 = (float *)malloc(sizeof(float *));
	float * d_var_27_1;
	cudaMalloc((void **)&d_var_27_1, sizeof(float *));
	
	float * h_var_27_2 = (float *)malloc(sizeof(float *));
	float * d_var_27_2;
	cudaMalloc((void **)&d_var_27_2, sizeof(float *));
	
	float * h_var_27_3 = (float *)malloc(sizeof(float *));
	float * d_var_27_3;
	cudaMalloc((void **)&d_var_27_3, sizeof(float *));
	
	float * h_var_27_4 = (float *)malloc(sizeof(float *));
	float * d_var_27_4;
	cudaMalloc((void **)&d_var_27_4, sizeof(float *));
	
	float * h_var_27_5 = (float *)malloc(sizeof(float *));
	float * d_var_27_5;
	cudaMalloc((void **)&d_var_27_5, sizeof(float *));
	
	float * h_var_27_6 = (float *)malloc(sizeof(float *));
	float * d_var_27_6;
	cudaMalloc((void **)&d_var_27_6, sizeof(float *));
	
	float * h_var_27_7 = (float *)malloc(sizeof(float *));
	float * d_var_27_7;
	cudaMalloc((void **)&d_var_27_7, sizeof(float *));
	
	float * h_var_27_8 = (float *)malloc(sizeof(float *));
	float * d_var_27_8;
	cudaMalloc((void **)&d_var_27_8, sizeof(float *));
	
	float * h_var_27_9 = (float *)malloc(sizeof(float *));
	float * d_var_27_9;
	cudaMalloc((void **)&d_var_27_9, sizeof(float *));
	
	float * h_var_28_0 = (float *)malloc(sizeof(float *));
	float * d_var_28_0;
	cudaMalloc((void **)&d_var_28_0, sizeof(float *));
	
	float * h_var_28_1 = (float *)malloc(sizeof(float *));
	float * d_var_28_1;
	cudaMalloc((void **)&d_var_28_1, sizeof(float *));
	
	float * h_var_28_2 = (float *)malloc(sizeof(float *));
	float * d_var_28_2;
	cudaMalloc((void **)&d_var_28_2, sizeof(float *));
	
	float * h_var_28_3 = (float *)malloc(sizeof(float *));
	float * d_var_28_3;
	cudaMalloc((void **)&d_var_28_3, sizeof(float *));
	
	float * h_var_28_4 = (float *)malloc(sizeof(float *));
	float * d_var_28_4;
	cudaMalloc((void **)&d_var_28_4, sizeof(float *));
	
	float * h_var_28_5 = (float *)malloc(sizeof(float *));
	float * d_var_28_5;
	cudaMalloc((void **)&d_var_28_5, sizeof(float *));
	
	float * h_var_28_6 = (float *)malloc(sizeof(float *));
	float * d_var_28_6;
	cudaMalloc((void **)&d_var_28_6, sizeof(float *));
	
	float * h_var_28_7 = (float *)malloc(sizeof(float *));
	float * d_var_28_7;
	cudaMalloc((void **)&d_var_28_7, sizeof(float *));
	
	float * h_var_28_8 = (float *)malloc(sizeof(float *));
	float * d_var_28_8;
	cudaMalloc((void **)&d_var_28_8, sizeof(float *));
	
	float * h_var_28_9 = (float *)malloc(sizeof(float *));
	float * d_var_28_9;
	cudaMalloc((void **)&d_var_28_9, sizeof(float *));
	
	float * h_var_29_0 = (float *)malloc(sizeof(float *));
	float * d_var_29_0;
	cudaMalloc((void **)&d_var_29_0, sizeof(float *));
	
	float * h_var_29_1 = (float *)malloc(sizeof(float *));
	float * d_var_29_1;
	cudaMalloc((void **)&d_var_29_1, sizeof(float *));
	
	float * h_var_29_2 = (float *)malloc(sizeof(float *));
	float * d_var_29_2;
	cudaMalloc((void **)&d_var_29_2, sizeof(float *));
	
	float * h_var_29_3 = (float *)malloc(sizeof(float *));
	float * d_var_29_3;
	cudaMalloc((void **)&d_var_29_3, sizeof(float *));
	
	float * h_var_29_4 = (float *)malloc(sizeof(float *));
	float * d_var_29_4;
	cudaMalloc((void **)&d_var_29_4, sizeof(float *));
	
	float * h_var_29_5 = (float *)malloc(sizeof(float *));
	float * d_var_29_5;
	cudaMalloc((void **)&d_var_29_5, sizeof(float *));
	
	float * h_var_29_6 = (float *)malloc(sizeof(float *));
	float * d_var_29_6;
	cudaMalloc((void **)&d_var_29_6, sizeof(float *));
	
	float * h_var_29_7 = (float *)malloc(sizeof(float *));
	float * d_var_29_7;
	cudaMalloc((void **)&d_var_29_7, sizeof(float *));
	
	float * h_var_29_8 = (float *)malloc(sizeof(float *));
	float * d_var_29_8;
	cudaMalloc((void **)&d_var_29_8, sizeof(float *));
	
	float * h_var_29_9 = (float *)malloc(sizeof(float *));
	float * d_var_29_9;
	cudaMalloc((void **)&d_var_29_9, sizeof(float *));
	
	float * h_var_30_0 = (float *)malloc(sizeof(float *));
	float * d_var_30_0;
	cudaMalloc((void **)&d_var_30_0, sizeof(float *));
	
	float * h_var_30_1 = (float *)malloc(sizeof(float *));
	float * d_var_30_1;
	cudaMalloc((void **)&d_var_30_1, sizeof(float *));
	
	float * h_var_30_2 = (float *)malloc(sizeof(float *));
	float * d_var_30_2;
	cudaMalloc((void **)&d_var_30_2, sizeof(float *));
	
	float * h_var_30_3 = (float *)malloc(sizeof(float *));
	float * d_var_30_3;
	cudaMalloc((void **)&d_var_30_3, sizeof(float *));
	
	float * h_var_30_4 = (float *)malloc(sizeof(float *));
	float * d_var_30_4;
	cudaMalloc((void **)&d_var_30_4, sizeof(float *));
	
	float * h_var_30_5 = (float *)malloc(sizeof(float *));
	float * d_var_30_5;
	cudaMalloc((void **)&d_var_30_5, sizeof(float *));
	
	float * h_var_30_6 = (float *)malloc(sizeof(float *));
	float * d_var_30_6;
	cudaMalloc((void **)&d_var_30_6, sizeof(float *));
	
	float * h_var_30_7 = (float *)malloc(sizeof(float *));
	float * d_var_30_7;
	cudaMalloc((void **)&d_var_30_7, sizeof(float *));
	
	float * h_var_30_8 = (float *)malloc(sizeof(float *));
	float * d_var_30_8;
	cudaMalloc((void **)&d_var_30_8, sizeof(float *));
	
	float * h_var_30_9 = (float *)malloc(sizeof(float *));
	float * d_var_30_9;
	cudaMalloc((void **)&d_var_30_9, sizeof(float *));
	
	float * h_var_31_0 = (float *)malloc(sizeof(float *));
	float * d_var_31_0;
	cudaMalloc((void **)&d_var_31_0, sizeof(float *));
	
	float * h_var_31_1 = (float *)malloc(sizeof(float *));
	float * d_var_31_1;
	cudaMalloc((void **)&d_var_31_1, sizeof(float *));
	
	float * h_var_31_2 = (float *)malloc(sizeof(float *));
	float * d_var_31_2;
	cudaMalloc((void **)&d_var_31_2, sizeof(float *));
	
	float * h_var_31_3 = (float *)malloc(sizeof(float *));
	float * d_var_31_3;
	cudaMalloc((void **)&d_var_31_3, sizeof(float *));
	
	float * h_var_31_4 = (float *)malloc(sizeof(float *));
	float * d_var_31_4;
	cudaMalloc((void **)&d_var_31_4, sizeof(float *));
	
	float * h_var_31_5 = (float *)malloc(sizeof(float *));
	float * d_var_31_5;
	cudaMalloc((void **)&d_var_31_5, sizeof(float *));
	
	float * h_var_31_6 = (float *)malloc(sizeof(float *));
	float * d_var_31_6;
	cudaMalloc((void **)&d_var_31_6, sizeof(float *));
	
	float * h_var_31_7 = (float *)malloc(sizeof(float *));
	float * d_var_31_7;
	cudaMalloc((void **)&d_var_31_7, sizeof(float *));
	
	float * h_var_31_8 = (float *)malloc(sizeof(float *));
	float * d_var_31_8;
	cudaMalloc((void **)&d_var_31_8, sizeof(float *));
	
	float * h_var_31_9 = (float *)malloc(sizeof(float *));
	float * d_var_31_9;
	cudaMalloc((void **)&d_var_31_9, sizeof(float *));
	
	float * h_var_32_0 = (float *)malloc(sizeof(float *));
	float * d_var_32_0;
	cudaMalloc((void **)&d_var_32_0, sizeof(float *));
	
	float * h_var_32_1 = (float *)malloc(sizeof(float *));
	float * d_var_32_1;
	cudaMalloc((void **)&d_var_32_1, sizeof(float *));
	
	float * h_var_32_2 = (float *)malloc(sizeof(float *));
	float * d_var_32_2;
	cudaMalloc((void **)&d_var_32_2, sizeof(float *));
	
	float * h_var_32_3 = (float *)malloc(sizeof(float *));
	float * d_var_32_3;
	cudaMalloc((void **)&d_var_32_3, sizeof(float *));
	
	float * h_var_32_4 = (float *)malloc(sizeof(float *));
	float * d_var_32_4;
	cudaMalloc((void **)&d_var_32_4, sizeof(float *));
	
	float * h_var_32_5 = (float *)malloc(sizeof(float *));
	float * d_var_32_5;
	cudaMalloc((void **)&d_var_32_5, sizeof(float *));
	
	float * h_var_32_6 = (float *)malloc(sizeof(float *));
	float * d_var_32_6;
	cudaMalloc((void **)&d_var_32_6, sizeof(float *));
	
	float * h_var_32_7 = (float *)malloc(sizeof(float *));
	float * d_var_32_7;
	cudaMalloc((void **)&d_var_32_7, sizeof(float *));
	
	float * h_var_32_8 = (float *)malloc(sizeof(float *));
	float * d_var_32_8;
	cudaMalloc((void **)&d_var_32_8, sizeof(float *));
	
	float * h_var_32_9 = (float *)malloc(sizeof(float *));
	float * d_var_32_9;
	cudaMalloc((void **)&d_var_32_9, sizeof(float *));
	
	float * h_var_33_0 = (float *)malloc(sizeof(float *));
	float * d_var_33_0;
	cudaMalloc((void **)&d_var_33_0, sizeof(float *));
	
	float * h_var_33_1 = (float *)malloc(sizeof(float *));
	float * d_var_33_1;
	cudaMalloc((void **)&d_var_33_1, sizeof(float *));
	
	float * h_var_33_2 = (float *)malloc(sizeof(float *));
	float * d_var_33_2;
	cudaMalloc((void **)&d_var_33_2, sizeof(float *));
	
	float * h_var_33_3 = (float *)malloc(sizeof(float *));
	float * d_var_33_3;
	cudaMalloc((void **)&d_var_33_3, sizeof(float *));
	
	float * h_var_33_4 = (float *)malloc(sizeof(float *));
	float * d_var_33_4;
	cudaMalloc((void **)&d_var_33_4, sizeof(float *));
	
	float * h_var_33_5 = (float *)malloc(sizeof(float *));
	float * d_var_33_5;
	cudaMalloc((void **)&d_var_33_5, sizeof(float *));
	
	float * h_var_33_6 = (float *)malloc(sizeof(float *));
	float * d_var_33_6;
	cudaMalloc((void **)&d_var_33_6, sizeof(float *));
	
	float * h_var_33_7 = (float *)malloc(sizeof(float *));
	float * d_var_33_7;
	cudaMalloc((void **)&d_var_33_7, sizeof(float *));
	
	float * h_var_33_8 = (float *)malloc(sizeof(float *));
	float * d_var_33_8;
	cudaMalloc((void **)&d_var_33_8, sizeof(float *));
	
	float * h_var_33_9 = (float *)malloc(sizeof(float *));
	float * d_var_33_9;
	cudaMalloc((void **)&d_var_33_9, sizeof(float *));
	
	float * h_var_34_0 = (float *)malloc(sizeof(float *));
	float * d_var_34_0;
	cudaMalloc((void **)&d_var_34_0, sizeof(float *));
	
	float * h_var_34_1 = (float *)malloc(sizeof(float *));
	float * d_var_34_1;
	cudaMalloc((void **)&d_var_34_1, sizeof(float *));
	
	float * h_var_34_2 = (float *)malloc(sizeof(float *));
	float * d_var_34_2;
	cudaMalloc((void **)&d_var_34_2, sizeof(float *));
	
	float * h_var_34_3 = (float *)malloc(sizeof(float *));
	float * d_var_34_3;
	cudaMalloc((void **)&d_var_34_3, sizeof(float *));
	
	float * h_var_34_4 = (float *)malloc(sizeof(float *));
	float * d_var_34_4;
	cudaMalloc((void **)&d_var_34_4, sizeof(float *));
	
	float * h_var_34_5 = (float *)malloc(sizeof(float *));
	float * d_var_34_5;
	cudaMalloc((void **)&d_var_34_5, sizeof(float *));
	
	float * h_var_34_6 = (float *)malloc(sizeof(float *));
	float * d_var_34_6;
	cudaMalloc((void **)&d_var_34_6, sizeof(float *));
	
	float * h_var_34_7 = (float *)malloc(sizeof(float *));
	float * d_var_34_7;
	cudaMalloc((void **)&d_var_34_7, sizeof(float *));
	
	float * h_var_34_8 = (float *)malloc(sizeof(float *));
	float * d_var_34_8;
	cudaMalloc((void **)&d_var_34_8, sizeof(float *));
	
	float * h_var_34_9 = (float *)malloc(sizeof(float *));
	float * d_var_34_9;
	cudaMalloc((void **)&d_var_34_9, sizeof(float *));
	
	float * h_var_35_0 = (float *)malloc(sizeof(float *));
	float * d_var_35_0;
	cudaMalloc((void **)&d_var_35_0, sizeof(float *));
	
	float * h_var_35_1 = (float *)malloc(sizeof(float *));
	float * d_var_35_1;
	cudaMalloc((void **)&d_var_35_1, sizeof(float *));
	
	float * h_var_35_2 = (float *)malloc(sizeof(float *));
	float * d_var_35_2;
	cudaMalloc((void **)&d_var_35_2, sizeof(float *));
	
	float * h_var_35_3 = (float *)malloc(sizeof(float *));
	float * d_var_35_3;
	cudaMalloc((void **)&d_var_35_3, sizeof(float *));
	
	float * h_var_35_4 = (float *)malloc(sizeof(float *));
	float * d_var_35_4;
	cudaMalloc((void **)&d_var_35_4, sizeof(float *));
	
	float * h_var_35_5 = (float *)malloc(sizeof(float *));
	float * d_var_35_5;
	cudaMalloc((void **)&d_var_35_5, sizeof(float *));
	
	float * h_var_35_6 = (float *)malloc(sizeof(float *));
	float * d_var_35_6;
	cudaMalloc((void **)&d_var_35_6, sizeof(float *));
	
	float * h_var_35_7 = (float *)malloc(sizeof(float *));
	float * d_var_35_7;
	cudaMalloc((void **)&d_var_35_7, sizeof(float *));
	
	float * h_var_35_8 = (float *)malloc(sizeof(float *));
	float * d_var_35_8;
	cudaMalloc((void **)&d_var_35_8, sizeof(float *));
	
	float * h_var_35_9 = (float *)malloc(sizeof(float *));
	float * d_var_35_9;
	cudaMalloc((void **)&d_var_35_9, sizeof(float *));
	
	float * h_var_36_0 = (float *)malloc(sizeof(float *));
	float * d_var_36_0;
	cudaMalloc((void **)&d_var_36_0, sizeof(float *));
	
	float * h_var_36_1 = (float *)malloc(sizeof(float *));
	float * d_var_36_1;
	cudaMalloc((void **)&d_var_36_1, sizeof(float *));
	
	float * h_var_36_2 = (float *)malloc(sizeof(float *));
	float * d_var_36_2;
	cudaMalloc((void **)&d_var_36_2, sizeof(float *));
	
	float * h_var_36_3 = (float *)malloc(sizeof(float *));
	float * d_var_36_3;
	cudaMalloc((void **)&d_var_36_3, sizeof(float *));
	
	float * h_var_36_4 = (float *)malloc(sizeof(float *));
	float * d_var_36_4;
	cudaMalloc((void **)&d_var_36_4, sizeof(float *));
	
	float * h_var_36_5 = (float *)malloc(sizeof(float *));
	float * d_var_36_5;
	cudaMalloc((void **)&d_var_36_5, sizeof(float *));
	
	float * h_var_36_6 = (float *)malloc(sizeof(float *));
	float * d_var_36_6;
	cudaMalloc((void **)&d_var_36_6, sizeof(float *));
	
	float * h_var_36_7 = (float *)malloc(sizeof(float *));
	float * d_var_36_7;
	cudaMalloc((void **)&d_var_36_7, sizeof(float *));
	
	float * h_var_36_8 = (float *)malloc(sizeof(float *));
	float * d_var_36_8;
	cudaMalloc((void **)&d_var_36_8, sizeof(float *));
	
	float * h_var_36_9 = (float *)malloc(sizeof(float *));
	float * d_var_36_9;
	cudaMalloc((void **)&d_var_36_9, sizeof(float *));
	
	float * h_var_37_0 = (float *)malloc(sizeof(float *));
	float * d_var_37_0;
	cudaMalloc((void **)&d_var_37_0, sizeof(float *));
	
	float * h_var_37_1 = (float *)malloc(sizeof(float *));
	float * d_var_37_1;
	cudaMalloc((void **)&d_var_37_1, sizeof(float *));
	
	float * h_var_37_2 = (float *)malloc(sizeof(float *));
	float * d_var_37_2;
	cudaMalloc((void **)&d_var_37_2, sizeof(float *));
	
	float * h_var_37_3 = (float *)malloc(sizeof(float *));
	float * d_var_37_3;
	cudaMalloc((void **)&d_var_37_3, sizeof(float *));
	
	float * h_var_37_4 = (float *)malloc(sizeof(float *));
	float * d_var_37_4;
	cudaMalloc((void **)&d_var_37_4, sizeof(float *));
	
	float * h_var_37_5 = (float *)malloc(sizeof(float *));
	float * d_var_37_5;
	cudaMalloc((void **)&d_var_37_5, sizeof(float *));
	
	float * h_var_37_6 = (float *)malloc(sizeof(float *));
	float * d_var_37_6;
	cudaMalloc((void **)&d_var_37_6, sizeof(float *));
	
	float * h_var_37_7 = (float *)malloc(sizeof(float *));
	float * d_var_37_7;
	cudaMalloc((void **)&d_var_37_7, sizeof(float *));
	
	float * h_var_37_8 = (float *)malloc(sizeof(float *));
	float * d_var_37_8;
	cudaMalloc((void **)&d_var_37_8, sizeof(float *));
	
	float * h_var_37_9 = (float *)malloc(sizeof(float *));
	float * d_var_37_9;
	cudaMalloc((void **)&d_var_37_9, sizeof(float *));
	
	float * h_var_38_0 = (float *)malloc(sizeof(float *));
	float * d_var_38_0;
	cudaMalloc((void **)&d_var_38_0, sizeof(float *));
	
	float * h_var_38_1 = (float *)malloc(sizeof(float *));
	float * d_var_38_1;
	cudaMalloc((void **)&d_var_38_1, sizeof(float *));
	
	float * h_var_38_2 = (float *)malloc(sizeof(float *));
	float * d_var_38_2;
	cudaMalloc((void **)&d_var_38_2, sizeof(float *));
	
	float * h_var_38_3 = (float *)malloc(sizeof(float *));
	float * d_var_38_3;
	cudaMalloc((void **)&d_var_38_3, sizeof(float *));
	
	float * h_var_38_4 = (float *)malloc(sizeof(float *));
	float * d_var_38_4;
	cudaMalloc((void **)&d_var_38_4, sizeof(float *));
	
	float * h_var_38_5 = (float *)malloc(sizeof(float *));
	float * d_var_38_5;
	cudaMalloc((void **)&d_var_38_5, sizeof(float *));
	
	float * h_var_38_6 = (float *)malloc(sizeof(float *));
	float * d_var_38_6;
	cudaMalloc((void **)&d_var_38_6, sizeof(float *));
	
	float * h_var_38_7 = (float *)malloc(sizeof(float *));
	float * d_var_38_7;
	cudaMalloc((void **)&d_var_38_7, sizeof(float *));
	
	float * h_var_38_8 = (float *)malloc(sizeof(float *));
	float * d_var_38_8;
	cudaMalloc((void **)&d_var_38_8, sizeof(float *));
	
	float * h_var_38_9 = (float *)malloc(sizeof(float *));
	float * d_var_38_9;
	cudaMalloc((void **)&d_var_38_9, sizeof(float *));
	
	float * h_var_39_0 = (float *)malloc(sizeof(float *));
	float * d_var_39_0;
	cudaMalloc((void **)&d_var_39_0, sizeof(float *));
	
	float * h_var_39_1 = (float *)malloc(sizeof(float *));
	float * d_var_39_1;
	cudaMalloc((void **)&d_var_39_1, sizeof(float *));
	
	float * h_var_39_2 = (float *)malloc(sizeof(float *));
	float * d_var_39_2;
	cudaMalloc((void **)&d_var_39_2, sizeof(float *));
	
	float * h_var_39_3 = (float *)malloc(sizeof(float *));
	float * d_var_39_3;
	cudaMalloc((void **)&d_var_39_3, sizeof(float *));
	
	float * h_var_39_4 = (float *)malloc(sizeof(float *));
	float * d_var_39_4;
	cudaMalloc((void **)&d_var_39_4, sizeof(float *));
	
	float * h_var_39_5 = (float *)malloc(sizeof(float *));
	float * d_var_39_5;
	cudaMalloc((void **)&d_var_39_5, sizeof(float *));
	
	float * h_var_39_6 = (float *)malloc(sizeof(float *));
	float * d_var_39_6;
	cudaMalloc((void **)&d_var_39_6, sizeof(float *));
	
	float * h_var_39_7 = (float *)malloc(sizeof(float *));
	float * d_var_39_7;
	cudaMalloc((void **)&d_var_39_7, sizeof(float *));
	
	float * h_var_39_8 = (float *)malloc(sizeof(float *));
	float * d_var_39_8;
	cudaMalloc((void **)&d_var_39_8, sizeof(float *));
	
	float * h_var_39_9 = (float *)malloc(sizeof(float *));
	float * d_var_39_9;
	cudaMalloc((void **)&d_var_39_9, sizeof(float *));
	
	float * h_var_40_0 = (float *)malloc(sizeof(float *));
	float * d_var_40_0;
	cudaMalloc((void **)&d_var_40_0, sizeof(float *));
	
	float * h_var_40_1 = (float *)malloc(sizeof(float *));
	float * d_var_40_1;
	cudaMalloc((void **)&d_var_40_1, sizeof(float *));
	
	float * h_var_40_2 = (float *)malloc(sizeof(float *));
	float * d_var_40_2;
	cudaMalloc((void **)&d_var_40_2, sizeof(float *));
	
	float * h_var_40_3 = (float *)malloc(sizeof(float *));
	float * d_var_40_3;
	cudaMalloc((void **)&d_var_40_3, sizeof(float *));
	
	float * h_var_40_4 = (float *)malloc(sizeof(float *));
	float * d_var_40_4;
	cudaMalloc((void **)&d_var_40_4, sizeof(float *));
	
	float * h_var_40_5 = (float *)malloc(sizeof(float *));
	float * d_var_40_5;
	cudaMalloc((void **)&d_var_40_5, sizeof(float *));
	
	float * h_var_40_6 = (float *)malloc(sizeof(float *));
	float * d_var_40_6;
	cudaMalloc((void **)&d_var_40_6, sizeof(float *));
	
	float * h_var_40_7 = (float *)malloc(sizeof(float *));
	float * d_var_40_7;
	cudaMalloc((void **)&d_var_40_7, sizeof(float *));
	
	float * h_var_40_8 = (float *)malloc(sizeof(float *));
	float * d_var_40_8;
	cudaMalloc((void **)&d_var_40_8, sizeof(float *));
	
	float * h_var_40_9 = (float *)malloc(sizeof(float *));
	float * d_var_40_9;
	cudaMalloc((void **)&d_var_40_9, sizeof(float *));
	
	float * h_var_41_0 = (float *)malloc(sizeof(float *));
	float * d_var_41_0;
	cudaMalloc((void **)&d_var_41_0, sizeof(float *));
	
	float * h_var_41_1 = (float *)malloc(sizeof(float *));
	float * d_var_41_1;
	cudaMalloc((void **)&d_var_41_1, sizeof(float *));
	
	float * h_var_41_2 = (float *)malloc(sizeof(float *));
	float * d_var_41_2;
	cudaMalloc((void **)&d_var_41_2, sizeof(float *));
	
	float * h_var_41_3 = (float *)malloc(sizeof(float *));
	float * d_var_41_3;
	cudaMalloc((void **)&d_var_41_3, sizeof(float *));
	
	float * h_var_41_4 = (float *)malloc(sizeof(float *));
	float * d_var_41_4;
	cudaMalloc((void **)&d_var_41_4, sizeof(float *));
	
	float * h_var_41_5 = (float *)malloc(sizeof(float *));
	float * d_var_41_5;
	cudaMalloc((void **)&d_var_41_5, sizeof(float *));
	
	float * h_var_41_6 = (float *)malloc(sizeof(float *));
	float * d_var_41_6;
	cudaMalloc((void **)&d_var_41_6, sizeof(float *));
	
	float * h_var_41_7 = (float *)malloc(sizeof(float *));
	float * d_var_41_7;
	cudaMalloc((void **)&d_var_41_7, sizeof(float *));
	
	float * h_var_41_8 = (float *)malloc(sizeof(float *));
	float * d_var_41_8;
	cudaMalloc((void **)&d_var_41_8, sizeof(float *));
	
	float * h_var_41_9 = (float *)malloc(sizeof(float *));
	float * d_var_41_9;
	cudaMalloc((void **)&d_var_41_9, sizeof(float *));
	
	float * h_var_42_0 = (float *)malloc(sizeof(float *));
	float * d_var_42_0;
	cudaMalloc((void **)&d_var_42_0, sizeof(float *));
	
	float * h_var_42_1 = (float *)malloc(sizeof(float *));
	float * d_var_42_1;
	cudaMalloc((void **)&d_var_42_1, sizeof(float *));
	
	float * h_var_42_2 = (float *)malloc(sizeof(float *));
	float * d_var_42_2;
	cudaMalloc((void **)&d_var_42_2, sizeof(float *));
	
	float * h_var_42_3 = (float *)malloc(sizeof(float *));
	float * d_var_42_3;
	cudaMalloc((void **)&d_var_42_3, sizeof(float *));
	
	float * h_var_42_4 = (float *)malloc(sizeof(float *));
	float * d_var_42_4;
	cudaMalloc((void **)&d_var_42_4, sizeof(float *));
	
	float * h_var_42_5 = (float *)malloc(sizeof(float *));
	float * d_var_42_5;
	cudaMalloc((void **)&d_var_42_5, sizeof(float *));
	
	float * h_var_42_6 = (float *)malloc(sizeof(float *));
	float * d_var_42_6;
	cudaMalloc((void **)&d_var_42_6, sizeof(float *));
	
	float * h_var_42_7 = (float *)malloc(sizeof(float *));
	float * d_var_42_7;
	cudaMalloc((void **)&d_var_42_7, sizeof(float *));
	
	float * h_var_42_8 = (float *)malloc(sizeof(float *));
	float * d_var_42_8;
	cudaMalloc((void **)&d_var_42_8, sizeof(float *));
	
	float * h_var_42_9 = (float *)malloc(sizeof(float *));
	float * d_var_42_9;
	cudaMalloc((void **)&d_var_42_9, sizeof(float *));
	
	float * h_var_43_0 = (float *)malloc(sizeof(float *));
	float * d_var_43_0;
	cudaMalloc((void **)&d_var_43_0, sizeof(float *));
	
	float * h_var_43_1 = (float *)malloc(sizeof(float *));
	float * d_var_43_1;
	cudaMalloc((void **)&d_var_43_1, sizeof(float *));
	
	float * h_var_43_2 = (float *)malloc(sizeof(float *));
	float * d_var_43_2;
	cudaMalloc((void **)&d_var_43_2, sizeof(float *));
	
	float * h_var_43_3 = (float *)malloc(sizeof(float *));
	float * d_var_43_3;
	cudaMalloc((void **)&d_var_43_3, sizeof(float *));
	
	float * h_var_43_4 = (float *)malloc(sizeof(float *));
	float * d_var_43_4;
	cudaMalloc((void **)&d_var_43_4, sizeof(float *));
	
	float * h_var_43_5 = (float *)malloc(sizeof(float *));
	float * d_var_43_5;
	cudaMalloc((void **)&d_var_43_5, sizeof(float *));
	
	float * h_var_43_6 = (float *)malloc(sizeof(float *));
	float * d_var_43_6;
	cudaMalloc((void **)&d_var_43_6, sizeof(float *));
	
	float * h_var_43_7 = (float *)malloc(sizeof(float *));
	float * d_var_43_7;
	cudaMalloc((void **)&d_var_43_7, sizeof(float *));
	
	float * h_var_43_8 = (float *)malloc(sizeof(float *));
	float * d_var_43_8;
	cudaMalloc((void **)&d_var_43_8, sizeof(float *));
	
	float * h_var_43_9 = (float *)malloc(sizeof(float *));
	float * d_var_43_9;
	cudaMalloc((void **)&d_var_43_9, sizeof(float *));
	
	float * h_var_44_0 = (float *)malloc(sizeof(float *));
	float * d_var_44_0;
	cudaMalloc((void **)&d_var_44_0, sizeof(float *));
	
	float * h_var_44_1 = (float *)malloc(sizeof(float *));
	float * d_var_44_1;
	cudaMalloc((void **)&d_var_44_1, sizeof(float *));
	
	float * h_var_44_2 = (float *)malloc(sizeof(float *));
	float * d_var_44_2;
	cudaMalloc((void **)&d_var_44_2, sizeof(float *));
	
	float * h_var_44_3 = (float *)malloc(sizeof(float *));
	float * d_var_44_3;
	cudaMalloc((void **)&d_var_44_3, sizeof(float *));
	
	float * h_var_44_4 = (float *)malloc(sizeof(float *));
	float * d_var_44_4;
	cudaMalloc((void **)&d_var_44_4, sizeof(float *));
	
	float * h_var_44_5 = (float *)malloc(sizeof(float *));
	float * d_var_44_5;
	cudaMalloc((void **)&d_var_44_5, sizeof(float *));
	
	float * h_var_44_6 = (float *)malloc(sizeof(float *));
	float * d_var_44_6;
	cudaMalloc((void **)&d_var_44_6, sizeof(float *));
	
	float * h_var_44_7 = (float *)malloc(sizeof(float *));
	float * d_var_44_7;
	cudaMalloc((void **)&d_var_44_7, sizeof(float *));
	
	float * h_var_44_8 = (float *)malloc(sizeof(float *));
	float * d_var_44_8;
	cudaMalloc((void **)&d_var_44_8, sizeof(float *));
	
	float * h_var_44_9 = (float *)malloc(sizeof(float *));
	float * d_var_44_9;
	cudaMalloc((void **)&d_var_44_9, sizeof(float *));
	
	float * h_var_45_0 = (float *)malloc(sizeof(float *));
	float * d_var_45_0;
	cudaMalloc((void **)&d_var_45_0, sizeof(float *));
	
	float * h_var_45_1 = (float *)malloc(sizeof(float *));
	float * d_var_45_1;
	cudaMalloc((void **)&d_var_45_1, sizeof(float *));
	
	float * h_var_45_2 = (float *)malloc(sizeof(float *));
	float * d_var_45_2;
	cudaMalloc((void **)&d_var_45_2, sizeof(float *));
	
	float * h_var_45_3 = (float *)malloc(sizeof(float *));
	float * d_var_45_3;
	cudaMalloc((void **)&d_var_45_3, sizeof(float *));
	
	float * h_var_45_4 = (float *)malloc(sizeof(float *));
	float * d_var_45_4;
	cudaMalloc((void **)&d_var_45_4, sizeof(float *));
	
	float * h_var_45_5 = (float *)malloc(sizeof(float *));
	float * d_var_45_5;
	cudaMalloc((void **)&d_var_45_5, sizeof(float *));
	
	float * h_var_45_6 = (float *)malloc(sizeof(float *));
	float * d_var_45_6;
	cudaMalloc((void **)&d_var_45_6, sizeof(float *));
	
	float * h_var_45_7 = (float *)malloc(sizeof(float *));
	float * d_var_45_7;
	cudaMalloc((void **)&d_var_45_7, sizeof(float *));
	
	float * h_var_45_8 = (float *)malloc(sizeof(float *));
	float * d_var_45_8;
	cudaMalloc((void **)&d_var_45_8, sizeof(float *));
	
	float * h_var_45_9 = (float *)malloc(sizeof(float *));
	float * d_var_45_9;
	cudaMalloc((void **)&d_var_45_9, sizeof(float *));
	
	float * h_var_46_0 = (float *)malloc(sizeof(float *));
	float * d_var_46_0;
	cudaMalloc((void **)&d_var_46_0, sizeof(float *));
	
	float * h_var_46_1 = (float *)malloc(sizeof(float *));
	float * d_var_46_1;
	cudaMalloc((void **)&d_var_46_1, sizeof(float *));
	
	float * h_var_46_2 = (float *)malloc(sizeof(float *));
	float * d_var_46_2;
	cudaMalloc((void **)&d_var_46_2, sizeof(float *));
	
	float * h_var_46_3 = (float *)malloc(sizeof(float *));
	float * d_var_46_3;
	cudaMalloc((void **)&d_var_46_3, sizeof(float *));
	
	float * h_var_46_4 = (float *)malloc(sizeof(float *));
	float * d_var_46_4;
	cudaMalloc((void **)&d_var_46_4, sizeof(float *));
	
	float * h_var_46_5 = (float *)malloc(sizeof(float *));
	float * d_var_46_5;
	cudaMalloc((void **)&d_var_46_5, sizeof(float *));
	
	float * h_var_46_6 = (float *)malloc(sizeof(float *));
	float * d_var_46_6;
	cudaMalloc((void **)&d_var_46_6, sizeof(float *));
	
	float * h_var_46_7 = (float *)malloc(sizeof(float *));
	float * d_var_46_7;
	cudaMalloc((void **)&d_var_46_7, sizeof(float *));
	
	float * h_var_46_8 = (float *)malloc(sizeof(float *));
	float * d_var_46_8;
	cudaMalloc((void **)&d_var_46_8, sizeof(float *));
	
	float * h_var_46_9 = (float *)malloc(sizeof(float *));
	float * d_var_46_9;
	cudaMalloc((void **)&d_var_46_9, sizeof(float *));
	
	float * h_var_47_0 = (float *)malloc(sizeof(float *));
	float * d_var_47_0;
	cudaMalloc((void **)&d_var_47_0, sizeof(float *));
	
	float * h_var_47_1 = (float *)malloc(sizeof(float *));
	float * d_var_47_1;
	cudaMalloc((void **)&d_var_47_1, sizeof(float *));
	
	float * h_var_47_2 = (float *)malloc(sizeof(float *));
	float * d_var_47_2;
	cudaMalloc((void **)&d_var_47_2, sizeof(float *));
	
	float * h_var_47_3 = (float *)malloc(sizeof(float *));
	float * d_var_47_3;
	cudaMalloc((void **)&d_var_47_3, sizeof(float *));
	
	float * h_var_47_4 = (float *)malloc(sizeof(float *));
	float * d_var_47_4;
	cudaMalloc((void **)&d_var_47_4, sizeof(float *));
	
	float * h_var_47_5 = (float *)malloc(sizeof(float *));
	float * d_var_47_5;
	cudaMalloc((void **)&d_var_47_5, sizeof(float *));
	
	float * h_var_47_6 = (float *)malloc(sizeof(float *));
	float * d_var_47_6;
	cudaMalloc((void **)&d_var_47_6, sizeof(float *));
	
	float * h_var_47_7 = (float *)malloc(sizeof(float *));
	float * d_var_47_7;
	cudaMalloc((void **)&d_var_47_7, sizeof(float *));
	
	float * h_var_47_8 = (float *)malloc(sizeof(float *));
	float * d_var_47_8;
	cudaMalloc((void **)&d_var_47_8, sizeof(float *));
	
	float * h_var_47_9 = (float *)malloc(sizeof(float *));
	float * d_var_47_9;
	cudaMalloc((void **)&d_var_47_9, sizeof(float *));
	
	float * h_var_48_0 = (float *)malloc(sizeof(float *));
	float * d_var_48_0;
	cudaMalloc((void **)&d_var_48_0, sizeof(float *));
	
	float * h_var_48_1 = (float *)malloc(sizeof(float *));
	float * d_var_48_1;
	cudaMalloc((void **)&d_var_48_1, sizeof(float *));
	
	float * h_var_48_2 = (float *)malloc(sizeof(float *));
	float * d_var_48_2;
	cudaMalloc((void **)&d_var_48_2, sizeof(float *));
	
	float * h_var_48_3 = (float *)malloc(sizeof(float *));
	float * d_var_48_3;
	cudaMalloc((void **)&d_var_48_3, sizeof(float *));
	
	float * h_var_48_4 = (float *)malloc(sizeof(float *));
	float * d_var_48_4;
	cudaMalloc((void **)&d_var_48_4, sizeof(float *));
	
	float * h_var_48_5 = (float *)malloc(sizeof(float *));
	float * d_var_48_5;
	cudaMalloc((void **)&d_var_48_5, sizeof(float *));
	
	float * h_var_48_6 = (float *)malloc(sizeof(float *));
	float * d_var_48_6;
	cudaMalloc((void **)&d_var_48_6, sizeof(float *));
	
	float * h_var_48_7 = (float *)malloc(sizeof(float *));
	float * d_var_48_7;
	cudaMalloc((void **)&d_var_48_7, sizeof(float *));
	
	float * h_var_48_8 = (float *)malloc(sizeof(float *));
	float * d_var_48_8;
	cudaMalloc((void **)&d_var_48_8, sizeof(float *));
	
	float * h_var_48_9 = (float *)malloc(sizeof(float *));
	float * d_var_48_9;
	cudaMalloc((void **)&d_var_48_9, sizeof(float *));
	
	float * h_var_49_0 = (float *)malloc(sizeof(float *));
	float * d_var_49_0;
	cudaMalloc((void **)&d_var_49_0, sizeof(float *));
	
	float * h_var_49_1 = (float *)malloc(sizeof(float *));
	float * d_var_49_1;
	cudaMalloc((void **)&d_var_49_1, sizeof(float *));
	
	float * h_var_49_2 = (float *)malloc(sizeof(float *));
	float * d_var_49_2;
	cudaMalloc((void **)&d_var_49_2, sizeof(float *));
	
	float * h_var_49_3 = (float *)malloc(sizeof(float *));
	float * d_var_49_3;
	cudaMalloc((void **)&d_var_49_3, sizeof(float *));
	
	float * h_var_49_4 = (float *)malloc(sizeof(float *));
	float * d_var_49_4;
	cudaMalloc((void **)&d_var_49_4, sizeof(float *));
	
	float * h_var_49_5 = (float *)malloc(sizeof(float *));
	float * d_var_49_5;
	cudaMalloc((void **)&d_var_49_5, sizeof(float *));
	
	float * h_var_49_6 = (float *)malloc(sizeof(float *));
	float * d_var_49_6;
	cudaMalloc((void **)&d_var_49_6, sizeof(float *));
	
	float * h_var_49_7 = (float *)malloc(sizeof(float *));
	float * d_var_49_7;
	cudaMalloc((void **)&d_var_49_7, sizeof(float *));
	
	float * h_var_49_8 = (float *)malloc(sizeof(float *));
	float * d_var_49_8;
	cudaMalloc((void **)&d_var_49_8, sizeof(float *));
	
	float * h_var_49_9 = (float *)malloc(sizeof(float *));
	float * d_var_49_9;
	cudaMalloc((void **)&d_var_49_9, sizeof(float *));
	
	float * h_var_50_0 = (float *)malloc(sizeof(float *));
	float * d_var_50_0;
	cudaMalloc((void **)&d_var_50_0, sizeof(float *));
	
	float * h_var_50_1 = (float *)malloc(sizeof(float *));
	float * d_var_50_1;
	cudaMalloc((void **)&d_var_50_1, sizeof(float *));
	
	float * h_var_50_2 = (float *)malloc(sizeof(float *));
	float * d_var_50_2;
	cudaMalloc((void **)&d_var_50_2, sizeof(float *));
	
	float * h_var_50_3 = (float *)malloc(sizeof(float *));
	float * d_var_50_3;
	cudaMalloc((void **)&d_var_50_3, sizeof(float *));
	
	float * h_var_50_4 = (float *)malloc(sizeof(float *));
	float * d_var_50_4;
	cudaMalloc((void **)&d_var_50_4, sizeof(float *));
	
	float * h_var_50_5 = (float *)malloc(sizeof(float *));
	float * d_var_50_5;
	cudaMalloc((void **)&d_var_50_5, sizeof(float *));
	
	float * h_var_50_6 = (float *)malloc(sizeof(float *));
	float * d_var_50_6;
	cudaMalloc((void **)&d_var_50_6, sizeof(float *));
	
	float * h_var_50_7 = (float *)malloc(sizeof(float *));
	float * d_var_50_7;
	cudaMalloc((void **)&d_var_50_7, sizeof(float *));
	
	float * h_var_50_8 = (float *)malloc(sizeof(float *));
	float * d_var_50_8;
	cudaMalloc((void **)&d_var_50_8, sizeof(float *));
	
	float * h_var_50_9 = (float *)malloc(sizeof(float *));
	float * d_var_50_9;
	cudaMalloc((void **)&d_var_50_9, sizeof(float *));
	
	float * h_var_51_0 = (float *)malloc(sizeof(float *));
	float * d_var_51_0;
	cudaMalloc((void **)&d_var_51_0, sizeof(float *));
	
	float * h_var_51_1 = (float *)malloc(sizeof(float *));
	float * d_var_51_1;
	cudaMalloc((void **)&d_var_51_1, sizeof(float *));
	
	float * h_var_51_2 = (float *)malloc(sizeof(float *));
	float * d_var_51_2;
	cudaMalloc((void **)&d_var_51_2, sizeof(float *));
	
	float * h_var_51_3 = (float *)malloc(sizeof(float *));
	float * d_var_51_3;
	cudaMalloc((void **)&d_var_51_3, sizeof(float *));
	
	float * h_var_51_4 = (float *)malloc(sizeof(float *));
	float * d_var_51_4;
	cudaMalloc((void **)&d_var_51_4, sizeof(float *));
	
	float * h_var_51_5 = (float *)malloc(sizeof(float *));
	float * d_var_51_5;
	cudaMalloc((void **)&d_var_51_5, sizeof(float *));
	
	float * h_var_51_6 = (float *)malloc(sizeof(float *));
	float * d_var_51_6;
	cudaMalloc((void **)&d_var_51_6, sizeof(float *));
	
	float * h_var_51_7 = (float *)malloc(sizeof(float *));
	float * d_var_51_7;
	cudaMalloc((void **)&d_var_51_7, sizeof(float *));
	
	float * h_var_51_8 = (float *)malloc(sizeof(float *));
	float * d_var_51_8;
	cudaMalloc((void **)&d_var_51_8, sizeof(float *));
	
	float * h_var_51_9 = (float *)malloc(sizeof(float *));
	float * d_var_51_9;
	cudaMalloc((void **)&d_var_51_9, sizeof(float *));
	
	float * h_var_52_0 = (float *)malloc(sizeof(float *));
	float * d_var_52_0;
	cudaMalloc((void **)&d_var_52_0, sizeof(float *));
	
	float * h_var_52_1 = (float *)malloc(sizeof(float *));
	float * d_var_52_1;
	cudaMalloc((void **)&d_var_52_1, sizeof(float *));
	
	float * h_var_52_2 = (float *)malloc(sizeof(float *));
	float * d_var_52_2;
	cudaMalloc((void **)&d_var_52_2, sizeof(float *));
	
	float * h_var_52_3 = (float *)malloc(sizeof(float *));
	float * d_var_52_3;
	cudaMalloc((void **)&d_var_52_3, sizeof(float *));
	
	float * h_var_52_4 = (float *)malloc(sizeof(float *));
	float * d_var_52_4;
	cudaMalloc((void **)&d_var_52_4, sizeof(float *));
	
	float * h_var_52_5 = (float *)malloc(sizeof(float *));
	float * d_var_52_5;
	cudaMalloc((void **)&d_var_52_5, sizeof(float *));
	
	float * h_var_52_6 = (float *)malloc(sizeof(float *));
	float * d_var_52_6;
	cudaMalloc((void **)&d_var_52_6, sizeof(float *));
	
	float * h_var_52_7 = (float *)malloc(sizeof(float *));
	float * d_var_52_7;
	cudaMalloc((void **)&d_var_52_7, sizeof(float *));
	
	float * h_var_52_8 = (float *)malloc(sizeof(float *));
	float * d_var_52_8;
	cudaMalloc((void **)&d_var_52_8, sizeof(float *));
	
	float * h_var_52_9 = (float *)malloc(sizeof(float *));
	float * d_var_52_9;
	cudaMalloc((void **)&d_var_52_9, sizeof(float *));
	
	float * h_var_53_0 = (float *)malloc(sizeof(float *));
	float * d_var_53_0;
	cudaMalloc((void **)&d_var_53_0, sizeof(float *));
	
	float * h_var_53_1 = (float *)malloc(sizeof(float *));
	float * d_var_53_1;
	cudaMalloc((void **)&d_var_53_1, sizeof(float *));
	
	float * h_var_53_2 = (float *)malloc(sizeof(float *));
	float * d_var_53_2;
	cudaMalloc((void **)&d_var_53_2, sizeof(float *));
	
	float * h_var_53_3 = (float *)malloc(sizeof(float *));
	float * d_var_53_3;
	cudaMalloc((void **)&d_var_53_3, sizeof(float *));
	
	float * h_var_53_4 = (float *)malloc(sizeof(float *));
	float * d_var_53_4;
	cudaMalloc((void **)&d_var_53_4, sizeof(float *));
	
	float * h_var_53_5 = (float *)malloc(sizeof(float *));
	float * d_var_53_5;
	cudaMalloc((void **)&d_var_53_5, sizeof(float *));
	
	float * h_var_53_6 = (float *)malloc(sizeof(float *));
	float * d_var_53_6;
	cudaMalloc((void **)&d_var_53_6, sizeof(float *));
	
	float * h_var_53_7 = (float *)malloc(sizeof(float *));
	float * d_var_53_7;
	cudaMalloc((void **)&d_var_53_7, sizeof(float *));
	
	float * h_var_53_8 = (float *)malloc(sizeof(float *));
	float * d_var_53_8;
	cudaMalloc((void **)&d_var_53_8, sizeof(float *));
	
	float * h_var_53_9 = (float *)malloc(sizeof(float *));
	float * d_var_53_9;
	cudaMalloc((void **)&d_var_53_9, sizeof(float *));
	
	float * h_var_54_0 = (float *)malloc(sizeof(float *));
	float * d_var_54_0;
	cudaMalloc((void **)&d_var_54_0, sizeof(float *));
	
	float * h_var_54_1 = (float *)malloc(sizeof(float *));
	float * d_var_54_1;
	cudaMalloc((void **)&d_var_54_1, sizeof(float *));
	
	float * h_var_54_2 = (float *)malloc(sizeof(float *));
	float * d_var_54_2;
	cudaMalloc((void **)&d_var_54_2, sizeof(float *));
	
	float * h_var_54_3 = (float *)malloc(sizeof(float *));
	float * d_var_54_3;
	cudaMalloc((void **)&d_var_54_3, sizeof(float *));
	
	float * h_var_54_4 = (float *)malloc(sizeof(float *));
	float * d_var_54_4;
	cudaMalloc((void **)&d_var_54_4, sizeof(float *));
	
	float * h_var_54_5 = (float *)malloc(sizeof(float *));
	float * d_var_54_5;
	cudaMalloc((void **)&d_var_54_5, sizeof(float *));
	
	float * h_var_54_6 = (float *)malloc(sizeof(float *));
	float * d_var_54_6;
	cudaMalloc((void **)&d_var_54_6, sizeof(float *));
	
	float * h_var_54_7 = (float *)malloc(sizeof(float *));
	float * d_var_54_7;
	cudaMalloc((void **)&d_var_54_7, sizeof(float *));
	
	float * h_var_54_8 = (float *)malloc(sizeof(float *));
	float * d_var_54_8;
	cudaMalloc((void **)&d_var_54_8, sizeof(float *));
	
	float * h_var_54_9 = (float *)malloc(sizeof(float *));
	float * d_var_54_9;
	cudaMalloc((void **)&d_var_54_9, sizeof(float *));
	
	float * h_var_55_0 = (float *)malloc(sizeof(float *));
	float * d_var_55_0;
	cudaMalloc((void **)&d_var_55_0, sizeof(float *));
	
	float * h_var_55_1 = (float *)malloc(sizeof(float *));
	float * d_var_55_1;
	cudaMalloc((void **)&d_var_55_1, sizeof(float *));
	
	float * h_var_55_2 = (float *)malloc(sizeof(float *));
	float * d_var_55_2;
	cudaMalloc((void **)&d_var_55_2, sizeof(float *));
	
	float * h_var_55_3 = (float *)malloc(sizeof(float *));
	float * d_var_55_3;
	cudaMalloc((void **)&d_var_55_3, sizeof(float *));
	
	float * h_var_55_4 = (float *)malloc(sizeof(float *));
	float * d_var_55_4;
	cudaMalloc((void **)&d_var_55_4, sizeof(float *));
	
	float * h_var_55_5 = (float *)malloc(sizeof(float *));
	float * d_var_55_5;
	cudaMalloc((void **)&d_var_55_5, sizeof(float *));
	
	float * h_var_55_6 = (float *)malloc(sizeof(float *));
	float * d_var_55_6;
	cudaMalloc((void **)&d_var_55_6, sizeof(float *));
	
	float * h_var_55_7 = (float *)malloc(sizeof(float *));
	float * d_var_55_7;
	cudaMalloc((void **)&d_var_55_7, sizeof(float *));
	
	float * h_var_55_8 = (float *)malloc(sizeof(float *));
	float * d_var_55_8;
	cudaMalloc((void **)&d_var_55_8, sizeof(float *));
	
	float * h_var_55_9 = (float *)malloc(sizeof(float *));
	float * d_var_55_9;
	cudaMalloc((void **)&d_var_55_9, sizeof(float *));
	
	float * h_var_56_0 = (float *)malloc(sizeof(float *));
	float * d_var_56_0;
	cudaMalloc((void **)&d_var_56_0, sizeof(float *));
	
	float * h_var_56_1 = (float *)malloc(sizeof(float *));
	float * d_var_56_1;
	cudaMalloc((void **)&d_var_56_1, sizeof(float *));
	
	float * h_var_56_2 = (float *)malloc(sizeof(float *));
	float * d_var_56_2;
	cudaMalloc((void **)&d_var_56_2, sizeof(float *));
	
	float * h_var_56_3 = (float *)malloc(sizeof(float *));
	float * d_var_56_3;
	cudaMalloc((void **)&d_var_56_3, sizeof(float *));
	
	float * h_var_56_4 = (float *)malloc(sizeof(float *));
	float * d_var_56_4;
	cudaMalloc((void **)&d_var_56_4, sizeof(float *));
	
	float * h_var_56_5 = (float *)malloc(sizeof(float *));
	float * d_var_56_5;
	cudaMalloc((void **)&d_var_56_5, sizeof(float *));
	
	float * h_var_56_6 = (float *)malloc(sizeof(float *));
	float * d_var_56_6;
	cudaMalloc((void **)&d_var_56_6, sizeof(float *));
	
	float * h_var_56_7 = (float *)malloc(sizeof(float *));
	float * d_var_56_7;
	cudaMalloc((void **)&d_var_56_7, sizeof(float *));
	
	float * h_var_56_8 = (float *)malloc(sizeof(float *));
	float * d_var_56_8;
	cudaMalloc((void **)&d_var_56_8, sizeof(float *));
	
	float * h_var_56_9 = (float *)malloc(sizeof(float *));
	float * d_var_56_9;
	cudaMalloc((void **)&d_var_56_9, sizeof(float *));
	
	float * h_var_57_0 = (float *)malloc(sizeof(float *));
	float * d_var_57_0;
	cudaMalloc((void **)&d_var_57_0, sizeof(float *));
	
	float * h_var_57_1 = (float *)malloc(sizeof(float *));
	float * d_var_57_1;
	cudaMalloc((void **)&d_var_57_1, sizeof(float *));
	
	float * h_var_57_2 = (float *)malloc(sizeof(float *));
	float * d_var_57_2;
	cudaMalloc((void **)&d_var_57_2, sizeof(float *));
	
	float * h_var_57_3 = (float *)malloc(sizeof(float *));
	float * d_var_57_3;
	cudaMalloc((void **)&d_var_57_3, sizeof(float *));
	
	float * h_var_57_4 = (float *)malloc(sizeof(float *));
	float * d_var_57_4;
	cudaMalloc((void **)&d_var_57_4, sizeof(float *));
	
	float * h_var_57_5 = (float *)malloc(sizeof(float *));
	float * d_var_57_5;
	cudaMalloc((void **)&d_var_57_5, sizeof(float *));
	
	float * h_var_57_6 = (float *)malloc(sizeof(float *));
	float * d_var_57_6;
	cudaMalloc((void **)&d_var_57_6, sizeof(float *));
	
	float * h_var_57_7 = (float *)malloc(sizeof(float *));
	float * d_var_57_7;
	cudaMalloc((void **)&d_var_57_7, sizeof(float *));
	
	float * h_var_57_8 = (float *)malloc(sizeof(float *));
	float * d_var_57_8;
	cudaMalloc((void **)&d_var_57_8, sizeof(float *));
	
	float * h_var_57_9 = (float *)malloc(sizeof(float *));
	float * d_var_57_9;
	cudaMalloc((void **)&d_var_57_9, sizeof(float *));
	
	float * h_var_58_0 = (float *)malloc(sizeof(float *));
	float * d_var_58_0;
	cudaMalloc((void **)&d_var_58_0, sizeof(float *));
	
	float * h_var_58_1 = (float *)malloc(sizeof(float *));
	float * d_var_58_1;
	cudaMalloc((void **)&d_var_58_1, sizeof(float *));
	
	float * h_var_58_2 = (float *)malloc(sizeof(float *));
	float * d_var_58_2;
	cudaMalloc((void **)&d_var_58_2, sizeof(float *));
	
	float * h_var_58_3 = (float *)malloc(sizeof(float *));
	float * d_var_58_3;
	cudaMalloc((void **)&d_var_58_3, sizeof(float *));
	
	float * h_var_58_4 = (float *)malloc(sizeof(float *));
	float * d_var_58_4;
	cudaMalloc((void **)&d_var_58_4, sizeof(float *));
	
	float * h_var_58_5 = (float *)malloc(sizeof(float *));
	float * d_var_58_5;
	cudaMalloc((void **)&d_var_58_5, sizeof(float *));
	
	float * h_var_58_6 = (float *)malloc(sizeof(float *));
	float * d_var_58_6;
	cudaMalloc((void **)&d_var_58_6, sizeof(float *));
	
	float * h_var_58_7 = (float *)malloc(sizeof(float *));
	float * d_var_58_7;
	cudaMalloc((void **)&d_var_58_7, sizeof(float *));
	
	float * h_var_58_8 = (float *)malloc(sizeof(float *));
	float * d_var_58_8;
	cudaMalloc((void **)&d_var_58_8, sizeof(float *));
	
	float * h_var_58_9 = (float *)malloc(sizeof(float *));
	float * d_var_58_9;
	cudaMalloc((void **)&d_var_58_9, sizeof(float *));
	
	float * h_var_59_0 = (float *)malloc(sizeof(float *));
	float * d_var_59_0;
	cudaMalloc((void **)&d_var_59_0, sizeof(float *));
	
	float * h_var_59_1 = (float *)malloc(sizeof(float *));
	float * d_var_59_1;
	cudaMalloc((void **)&d_var_59_1, sizeof(float *));
	
	float * h_var_59_2 = (float *)malloc(sizeof(float *));
	float * d_var_59_2;
	cudaMalloc((void **)&d_var_59_2, sizeof(float *));
	
	float * h_var_59_3 = (float *)malloc(sizeof(float *));
	float * d_var_59_3;
	cudaMalloc((void **)&d_var_59_3, sizeof(float *));
	
	float * h_var_59_4 = (float *)malloc(sizeof(float *));
	float * d_var_59_4;
	cudaMalloc((void **)&d_var_59_4, sizeof(float *));
	
	float * h_var_59_5 = (float *)malloc(sizeof(float *));
	float * d_var_59_5;
	cudaMalloc((void **)&d_var_59_5, sizeof(float *));
	
	float * h_var_59_6 = (float *)malloc(sizeof(float *));
	float * d_var_59_6;
	cudaMalloc((void **)&d_var_59_6, sizeof(float *));
	
	float * h_var_59_7 = (float *)malloc(sizeof(float *));
	float * d_var_59_7;
	cudaMalloc((void **)&d_var_59_7, sizeof(float *));
	
	float * h_var_59_8 = (float *)malloc(sizeof(float *));
	float * d_var_59_8;
	cudaMalloc((void **)&d_var_59_8, sizeof(float *));
	
	float * h_var_59_9 = (float *)malloc(sizeof(float *));
	float * d_var_59_9;
	cudaMalloc((void **)&d_var_59_9, sizeof(float *));
	
	float * h_var_60_0 = (float *)malloc(sizeof(float *));
	float * d_var_60_0;
	cudaMalloc((void **)&d_var_60_0, sizeof(float *));
	
	float * h_var_60_1 = (float *)malloc(sizeof(float *));
	float * d_var_60_1;
	cudaMalloc((void **)&d_var_60_1, sizeof(float *));
	
	float * h_var_60_2 = (float *)malloc(sizeof(float *));
	float * d_var_60_2;
	cudaMalloc((void **)&d_var_60_2, sizeof(float *));
	
	float * h_var_60_3 = (float *)malloc(sizeof(float *));
	float * d_var_60_3;
	cudaMalloc((void **)&d_var_60_3, sizeof(float *));
	
	float * h_var_60_4 = (float *)malloc(sizeof(float *));
	float * d_var_60_4;
	cudaMalloc((void **)&d_var_60_4, sizeof(float *));
	
	float * h_var_60_5 = (float *)malloc(sizeof(float *));
	float * d_var_60_5;
	cudaMalloc((void **)&d_var_60_5, sizeof(float *));
	
	float * h_var_60_6 = (float *)malloc(sizeof(float *));
	float * d_var_60_6;
	cudaMalloc((void **)&d_var_60_6, sizeof(float *));
	
	float * h_var_60_7 = (float *)malloc(sizeof(float *));
	float * d_var_60_7;
	cudaMalloc((void **)&d_var_60_7, sizeof(float *));
	
	float * h_var_60_8 = (float *)malloc(sizeof(float *));
	float * d_var_60_8;
	cudaMalloc((void **)&d_var_60_8, sizeof(float *));
	
	float * h_var_60_9 = (float *)malloc(sizeof(float *));
	float * d_var_60_9;
	cudaMalloc((void **)&d_var_60_9, sizeof(float *));
	
	float * h_var_61_0 = (float *)malloc(sizeof(float *));
	float * d_var_61_0;
	cudaMalloc((void **)&d_var_61_0, sizeof(float *));
	
	float * h_var_61_1 = (float *)malloc(sizeof(float *));
	float * d_var_61_1;
	cudaMalloc((void **)&d_var_61_1, sizeof(float *));
	
	float * h_var_61_2 = (float *)malloc(sizeof(float *));
	float * d_var_61_2;
	cudaMalloc((void **)&d_var_61_2, sizeof(float *));
	
	float * h_var_61_3 = (float *)malloc(sizeof(float *));
	float * d_var_61_3;
	cudaMalloc((void **)&d_var_61_3, sizeof(float *));
	
	float * h_var_61_4 = (float *)malloc(sizeof(float *));
	float * d_var_61_4;
	cudaMalloc((void **)&d_var_61_4, sizeof(float *));
	
	float * h_var_61_5 = (float *)malloc(sizeof(float *));
	float * d_var_61_5;
	cudaMalloc((void **)&d_var_61_5, sizeof(float *));
	
	float * h_var_61_6 = (float *)malloc(sizeof(float *));
	float * d_var_61_6;
	cudaMalloc((void **)&d_var_61_6, sizeof(float *));
	
	float * h_var_61_7 = (float *)malloc(sizeof(float *));
	float * d_var_61_7;
	cudaMalloc((void **)&d_var_61_7, sizeof(float *));
	
	float * h_var_61_8 = (float *)malloc(sizeof(float *));
	float * d_var_61_8;
	cudaMalloc((void **)&d_var_61_8, sizeof(float *));
	
	float * h_var_61_9 = (float *)malloc(sizeof(float *));
	float * d_var_61_9;
	cudaMalloc((void **)&d_var_61_9, sizeof(float *));
	
	float * h_var_62_0 = (float *)malloc(sizeof(float *));
	float * d_var_62_0;
	cudaMalloc((void **)&d_var_62_0, sizeof(float *));
	
	float * h_var_62_1 = (float *)malloc(sizeof(float *));
	float * d_var_62_1;
	cudaMalloc((void **)&d_var_62_1, sizeof(float *));
	
	float * h_var_62_2 = (float *)malloc(sizeof(float *));
	float * d_var_62_2;
	cudaMalloc((void **)&d_var_62_2, sizeof(float *));
	
	float * h_var_62_3 = (float *)malloc(sizeof(float *));
	float * d_var_62_3;
	cudaMalloc((void **)&d_var_62_3, sizeof(float *));
	
	float * h_var_62_4 = (float *)malloc(sizeof(float *));
	float * d_var_62_4;
	cudaMalloc((void **)&d_var_62_4, sizeof(float *));
	
	float * h_var_62_5 = (float *)malloc(sizeof(float *));
	float * d_var_62_5;
	cudaMalloc((void **)&d_var_62_5, sizeof(float *));
	
	float * h_var_62_6 = (float *)malloc(sizeof(float *));
	float * d_var_62_6;
	cudaMalloc((void **)&d_var_62_6, sizeof(float *));
	
	float * h_var_62_7 = (float *)malloc(sizeof(float *));
	float * d_var_62_7;
	cudaMalloc((void **)&d_var_62_7, sizeof(float *));
	
	float * h_var_62_8 = (float *)malloc(sizeof(float *));
	float * d_var_62_8;
	cudaMalloc((void **)&d_var_62_8, sizeof(float *));
	
	float * h_var_62_9 = (float *)malloc(sizeof(float *));
	float * d_var_62_9;
	cudaMalloc((void **)&d_var_62_9, sizeof(float *));
	
	float * h_var_63_0 = (float *)malloc(sizeof(float *));
	float * d_var_63_0;
	cudaMalloc((void **)&d_var_63_0, sizeof(float *));
	
	float * h_var_63_1 = (float *)malloc(sizeof(float *));
	float * d_var_63_1;
	cudaMalloc((void **)&d_var_63_1, sizeof(float *));
	
	float * h_var_63_2 = (float *)malloc(sizeof(float *));
	float * d_var_63_2;
	cudaMalloc((void **)&d_var_63_2, sizeof(float *));
	
	float * h_var_63_3 = (float *)malloc(sizeof(float *));
	float * d_var_63_3;
	cudaMalloc((void **)&d_var_63_3, sizeof(float *));
	
	float * h_var_63_4 = (float *)malloc(sizeof(float *));
	float * d_var_63_4;
	cudaMalloc((void **)&d_var_63_4, sizeof(float *));
	
	float * h_var_63_5 = (float *)malloc(sizeof(float *));
	float * d_var_63_5;
	cudaMalloc((void **)&d_var_63_5, sizeof(float *));
	
	float * h_var_63_6 = (float *)malloc(sizeof(float *));
	float * d_var_63_6;
	cudaMalloc((void **)&d_var_63_6, sizeof(float *));
	
	float * h_var_63_7 = (float *)malloc(sizeof(float *));
	float * d_var_63_7;
	cudaMalloc((void **)&d_var_63_7, sizeof(float *));
	
	float * h_var_63_8 = (float *)malloc(sizeof(float *));
	float * d_var_63_8;
	cudaMalloc((void **)&d_var_63_8, sizeof(float *));
	
	float * h_var_63_9 = (float *)malloc(sizeof(float *));
	float * d_var_63_9;
	cudaMalloc((void **)&d_var_63_9, sizeof(float *));
	
	float * h_var_64_0 = (float *)malloc(sizeof(float *));
	float * d_var_64_0;
	cudaMalloc((void **)&d_var_64_0, sizeof(float *));
	
	float * h_var_64_1 = (float *)malloc(sizeof(float *));
	float * d_var_64_1;
	cudaMalloc((void **)&d_var_64_1, sizeof(float *));
	
	float * h_var_64_2 = (float *)malloc(sizeof(float *));
	float * d_var_64_2;
	cudaMalloc((void **)&d_var_64_2, sizeof(float *));
	
	float * h_var_64_3 = (float *)malloc(sizeof(float *));
	float * d_var_64_3;
	cudaMalloc((void **)&d_var_64_3, sizeof(float *));
	
	float * h_var_64_4 = (float *)malloc(sizeof(float *));
	float * d_var_64_4;
	cudaMalloc((void **)&d_var_64_4, sizeof(float *));
	
	float * h_var_64_5 = (float *)malloc(sizeof(float *));
	float * d_var_64_5;
	cudaMalloc((void **)&d_var_64_5, sizeof(float *));
	
	float * h_var_64_6 = (float *)malloc(sizeof(float *));
	float * d_var_64_6;
	cudaMalloc((void **)&d_var_64_6, sizeof(float *));
	
	float * h_var_64_7 = (float *)malloc(sizeof(float *));
	float * d_var_64_7;
	cudaMalloc((void **)&d_var_64_7, sizeof(float *));
	
	float * h_var_64_8 = (float *)malloc(sizeof(float *));
	float * d_var_64_8;
	cudaMalloc((void **)&d_var_64_8, sizeof(float *));
	
	float * h_var_64_9 = (float *)malloc(sizeof(float *));
	float * d_var_64_9;
	cudaMalloc((void **)&d_var_64_9, sizeof(float *));
	
	float * h_var_65_0 = (float *)malloc(sizeof(float *));
	float * d_var_65_0;
	cudaMalloc((void **)&d_var_65_0, sizeof(float *));
	
	float * h_var_65_1 = (float *)malloc(sizeof(float *));
	float * d_var_65_1;
	cudaMalloc((void **)&d_var_65_1, sizeof(float *));
	
	float * h_var_65_2 = (float *)malloc(sizeof(float *));
	float * d_var_65_2;
	cudaMalloc((void **)&d_var_65_2, sizeof(float *));
	
	float * h_var_65_3 = (float *)malloc(sizeof(float *));
	float * d_var_65_3;
	cudaMalloc((void **)&d_var_65_3, sizeof(float *));
	
	float * h_var_65_4 = (float *)malloc(sizeof(float *));
	float * d_var_65_4;
	cudaMalloc((void **)&d_var_65_4, sizeof(float *));
	
	float * h_var_65_5 = (float *)malloc(sizeof(float *));
	float * d_var_65_5;
	cudaMalloc((void **)&d_var_65_5, sizeof(float *));
	
	float * h_var_65_6 = (float *)malloc(sizeof(float *));
	float * d_var_65_6;
	cudaMalloc((void **)&d_var_65_6, sizeof(float *));
	
	float * h_var_65_7 = (float *)malloc(sizeof(float *));
	float * d_var_65_7;
	cudaMalloc((void **)&d_var_65_7, sizeof(float *));
	
	float * h_var_65_8 = (float *)malloc(sizeof(float *));
	float * d_var_65_8;
	cudaMalloc((void **)&d_var_65_8, sizeof(float *));
	
	float * h_var_65_9 = (float *)malloc(sizeof(float *));
	float * d_var_65_9;
	cudaMalloc((void **)&d_var_65_9, sizeof(float *));
	
	float * h_var_66_0 = (float *)malloc(sizeof(float *));
	float * d_var_66_0;
	cudaMalloc((void **)&d_var_66_0, sizeof(float *));
	
	float * h_var_66_1 = (float *)malloc(sizeof(float *));
	float * d_var_66_1;
	cudaMalloc((void **)&d_var_66_1, sizeof(float *));
	
	float * h_var_66_2 = (float *)malloc(sizeof(float *));
	float * d_var_66_2;
	cudaMalloc((void **)&d_var_66_2, sizeof(float *));
	
	float * h_var_66_3 = (float *)malloc(sizeof(float *));
	float * d_var_66_3;
	cudaMalloc((void **)&d_var_66_3, sizeof(float *));
	
	float * h_var_66_4 = (float *)malloc(sizeof(float *));
	float * d_var_66_4;
	cudaMalloc((void **)&d_var_66_4, sizeof(float *));
	
	float * h_var_66_5 = (float *)malloc(sizeof(float *));
	float * d_var_66_5;
	cudaMalloc((void **)&d_var_66_5, sizeof(float *));
	
	float * h_var_66_6 = (float *)malloc(sizeof(float *));
	float * d_var_66_6;
	cudaMalloc((void **)&d_var_66_6, sizeof(float *));
	
	float * h_var_66_7 = (float *)malloc(sizeof(float *));
	float * d_var_66_7;
	cudaMalloc((void **)&d_var_66_7, sizeof(float *));
	
	float * h_var_66_8 = (float *)malloc(sizeof(float *));
	float * d_var_66_8;
	cudaMalloc((void **)&d_var_66_8, sizeof(float *));
	
	float * h_var_66_9 = (float *)malloc(sizeof(float *));
	float * d_var_66_9;
	cudaMalloc((void **)&d_var_66_9, sizeof(float *));
	
	float * h_var_67_0 = (float *)malloc(sizeof(float *));
	float * d_var_67_0;
	cudaMalloc((void **)&d_var_67_0, sizeof(float *));
	
	float * h_var_67_1 = (float *)malloc(sizeof(float *));
	float * d_var_67_1;
	cudaMalloc((void **)&d_var_67_1, sizeof(float *));
	
	float * h_var_67_2 = (float *)malloc(sizeof(float *));
	float * d_var_67_2;
	cudaMalloc((void **)&d_var_67_2, sizeof(float *));
	
	float * h_var_67_3 = (float *)malloc(sizeof(float *));
	float * d_var_67_3;
	cudaMalloc((void **)&d_var_67_3, sizeof(float *));
	
	float * h_var_67_4 = (float *)malloc(sizeof(float *));
	float * d_var_67_4;
	cudaMalloc((void **)&d_var_67_4, sizeof(float *));
	
	float * h_var_67_5 = (float *)malloc(sizeof(float *));
	float * d_var_67_5;
	cudaMalloc((void **)&d_var_67_5, sizeof(float *));
	
	float * h_var_67_6 = (float *)malloc(sizeof(float *));
	float * d_var_67_6;
	cudaMalloc((void **)&d_var_67_6, sizeof(float *));
	
	float * h_var_67_7 = (float *)malloc(sizeof(float *));
	float * d_var_67_7;
	cudaMalloc((void **)&d_var_67_7, sizeof(float *));
	
	float * h_var_67_8 = (float *)malloc(sizeof(float *));
	float * d_var_67_8;
	cudaMalloc((void **)&d_var_67_8, sizeof(float *));
	
	float * h_var_67_9 = (float *)malloc(sizeof(float *));
	float * d_var_67_9;
	cudaMalloc((void **)&d_var_67_9, sizeof(float *));
	
	float * h_var_68_0 = (float *)malloc(sizeof(float *));
	float * d_var_68_0;
	cudaMalloc((void **)&d_var_68_0, sizeof(float *));
	
	float * h_var_68_1 = (float *)malloc(sizeof(float *));
	float * d_var_68_1;
	cudaMalloc((void **)&d_var_68_1, sizeof(float *));
	
	float * h_var_68_2 = (float *)malloc(sizeof(float *));
	float * d_var_68_2;
	cudaMalloc((void **)&d_var_68_2, sizeof(float *));
	
	float * h_var_68_3 = (float *)malloc(sizeof(float *));
	float * d_var_68_3;
	cudaMalloc((void **)&d_var_68_3, sizeof(float *));
	
	float * h_var_68_4 = (float *)malloc(sizeof(float *));
	float * d_var_68_4;
	cudaMalloc((void **)&d_var_68_4, sizeof(float *));
	
	float * h_var_68_5 = (float *)malloc(sizeof(float *));
	float * d_var_68_5;
	cudaMalloc((void **)&d_var_68_5, sizeof(float *));
	
	float * h_var_68_6 = (float *)malloc(sizeof(float *));
	float * d_var_68_6;
	cudaMalloc((void **)&d_var_68_6, sizeof(float *));
	
	float * h_var_68_7 = (float *)malloc(sizeof(float *));
	float * d_var_68_7;
	cudaMalloc((void **)&d_var_68_7, sizeof(float *));
	
	float * h_var_68_8 = (float *)malloc(sizeof(float *));
	float * d_var_68_8;
	cudaMalloc((void **)&d_var_68_8, sizeof(float *));
	
	float * h_var_68_9 = (float *)malloc(sizeof(float *));
	float * d_var_68_9;
	cudaMalloc((void **)&d_var_68_9, sizeof(float *));
	
	float * h_var_69_0 = (float *)malloc(sizeof(float *));
	float * d_var_69_0;
	cudaMalloc((void **)&d_var_69_0, sizeof(float *));
	
	float * h_var_69_1 = (float *)malloc(sizeof(float *));
	float * d_var_69_1;
	cudaMalloc((void **)&d_var_69_1, sizeof(float *));
	
	float * h_var_69_2 = (float *)malloc(sizeof(float *));
	float * d_var_69_2;
	cudaMalloc((void **)&d_var_69_2, sizeof(float *));
	
	float * h_var_69_3 = (float *)malloc(sizeof(float *));
	float * d_var_69_3;
	cudaMalloc((void **)&d_var_69_3, sizeof(float *));
	
	float * h_var_69_4 = (float *)malloc(sizeof(float *));
	float * d_var_69_4;
	cudaMalloc((void **)&d_var_69_4, sizeof(float *));
	
	float * h_var_69_5 = (float *)malloc(sizeof(float *));
	float * d_var_69_5;
	cudaMalloc((void **)&d_var_69_5, sizeof(float *));
	
	float * h_var_69_6 = (float *)malloc(sizeof(float *));
	float * d_var_69_6;
	cudaMalloc((void **)&d_var_69_6, sizeof(float *));
	
	float * h_var_69_7 = (float *)malloc(sizeof(float *));
	float * d_var_69_7;
	cudaMalloc((void **)&d_var_69_7, sizeof(float *));
	
	float * h_var_69_8 = (float *)malloc(sizeof(float *));
	float * d_var_69_8;
	cudaMalloc((void **)&d_var_69_8, sizeof(float *));
	
	float * h_var_69_9 = (float *)malloc(sizeof(float *));
	float * d_var_69_9;
	cudaMalloc((void **)&d_var_69_9, sizeof(float *));
	
	float * h_var_70_0 = (float *)malloc(sizeof(float *));
	float * d_var_70_0;
	cudaMalloc((void **)&d_var_70_0, sizeof(float *));
	
	float * h_var_70_1 = (float *)malloc(sizeof(float *));
	float * d_var_70_1;
	cudaMalloc((void **)&d_var_70_1, sizeof(float *));
	
	float * h_var_70_2 = (float *)malloc(sizeof(float *));
	float * d_var_70_2;
	cudaMalloc((void **)&d_var_70_2, sizeof(float *));
	
	float * h_var_70_3 = (float *)malloc(sizeof(float *));
	float * d_var_70_3;
	cudaMalloc((void **)&d_var_70_3, sizeof(float *));
	
	float * h_var_70_4 = (float *)malloc(sizeof(float *));
	float * d_var_70_4;
	cudaMalloc((void **)&d_var_70_4, sizeof(float *));
	
	float * h_var_70_5 = (float *)malloc(sizeof(float *));
	float * d_var_70_5;
	cudaMalloc((void **)&d_var_70_5, sizeof(float *));
	
	float * h_var_70_6 = (float *)malloc(sizeof(float *));
	float * d_var_70_6;
	cudaMalloc((void **)&d_var_70_6, sizeof(float *));
	
	float * h_var_70_7 = (float *)malloc(sizeof(float *));
	float * d_var_70_7;
	cudaMalloc((void **)&d_var_70_7, sizeof(float *));
	
	float * h_var_70_8 = (float *)malloc(sizeof(float *));
	float * d_var_70_8;
	cudaMalloc((void **)&d_var_70_8, sizeof(float *));
	
	float * h_var_70_9 = (float *)malloc(sizeof(float *));
	float * d_var_70_9;
	cudaMalloc((void **)&d_var_70_9, sizeof(float *));
	
	float * h_var_71_0 = (float *)malloc(sizeof(float *));
	float * d_var_71_0;
	cudaMalloc((void **)&d_var_71_0, sizeof(float *));
	
	float * h_var_71_1 = (float *)malloc(sizeof(float *));
	float * d_var_71_1;
	cudaMalloc((void **)&d_var_71_1, sizeof(float *));
	
	float * h_var_71_2 = (float *)malloc(sizeof(float *));
	float * d_var_71_2;
	cudaMalloc((void **)&d_var_71_2, sizeof(float *));
	
	float * h_var_71_3 = (float *)malloc(sizeof(float *));
	float * d_var_71_3;
	cudaMalloc((void **)&d_var_71_3, sizeof(float *));
	
	float * h_var_71_4 = (float *)malloc(sizeof(float *));
	float * d_var_71_4;
	cudaMalloc((void **)&d_var_71_4, sizeof(float *));
	
	float * h_var_71_5 = (float *)malloc(sizeof(float *));
	float * d_var_71_5;
	cudaMalloc((void **)&d_var_71_5, sizeof(float *));
	
	float * h_var_71_6 = (float *)malloc(sizeof(float *));
	float * d_var_71_6;
	cudaMalloc((void **)&d_var_71_6, sizeof(float *));
	
	float * h_var_71_7 = (float *)malloc(sizeof(float *));
	float * d_var_71_7;
	cudaMalloc((void **)&d_var_71_7, sizeof(float *));
	
	float * h_var_71_8 = (float *)malloc(sizeof(float *));
	float * d_var_71_8;
	cudaMalloc((void **)&d_var_71_8, sizeof(float *));
	
	float * h_var_71_9 = (float *)malloc(sizeof(float *));
	float * d_var_71_9;
	cudaMalloc((void **)&d_var_71_9, sizeof(float *));
	
	float * h_var_72_0 = (float *)malloc(sizeof(float *));
	float * d_var_72_0;
	cudaMalloc((void **)&d_var_72_0, sizeof(float *));
	
	float * h_var_72_1 = (float *)malloc(sizeof(float *));
	float * d_var_72_1;
	cudaMalloc((void **)&d_var_72_1, sizeof(float *));
	
	float * h_var_72_2 = (float *)malloc(sizeof(float *));
	float * d_var_72_2;
	cudaMalloc((void **)&d_var_72_2, sizeof(float *));
	
	float * h_var_72_3 = (float *)malloc(sizeof(float *));
	float * d_var_72_3;
	cudaMalloc((void **)&d_var_72_3, sizeof(float *));
	
	float * h_var_72_4 = (float *)malloc(sizeof(float *));
	float * d_var_72_4;
	cudaMalloc((void **)&d_var_72_4, sizeof(float *));
	
	float * h_var_72_5 = (float *)malloc(sizeof(float *));
	float * d_var_72_5;
	cudaMalloc((void **)&d_var_72_5, sizeof(float *));
	
	float * h_var_72_6 = (float *)malloc(sizeof(float *));
	float * d_var_72_6;
	cudaMalloc((void **)&d_var_72_6, sizeof(float *));
	
	float * h_var_72_7 = (float *)malloc(sizeof(float *));
	float * d_var_72_7;
	cudaMalloc((void **)&d_var_72_7, sizeof(float *));
	
	float * h_var_72_8 = (float *)malloc(sizeof(float *));
	float * d_var_72_8;
	cudaMalloc((void **)&d_var_72_8, sizeof(float *));
	
	float * h_var_72_9 = (float *)malloc(sizeof(float *));
	float * d_var_72_9;
	cudaMalloc((void **)&d_var_72_9, sizeof(float *));
	
	float * h_var_73_0 = (float *)malloc(sizeof(float *));
	float * d_var_73_0;
	cudaMalloc((void **)&d_var_73_0, sizeof(float *));
	
	float * h_var_73_1 = (float *)malloc(sizeof(float *));
	float * d_var_73_1;
	cudaMalloc((void **)&d_var_73_1, sizeof(float *));
	
	float * h_var_73_2 = (float *)malloc(sizeof(float *));
	float * d_var_73_2;
	cudaMalloc((void **)&d_var_73_2, sizeof(float *));
	
	float * h_var_73_3 = (float *)malloc(sizeof(float *));
	float * d_var_73_3;
	cudaMalloc((void **)&d_var_73_3, sizeof(float *));
	
	float * h_var_73_4 = (float *)malloc(sizeof(float *));
	float * d_var_73_4;
	cudaMalloc((void **)&d_var_73_4, sizeof(float *));
	
	float * h_var_73_5 = (float *)malloc(sizeof(float *));
	float * d_var_73_5;
	cudaMalloc((void **)&d_var_73_5, sizeof(float *));
	
	float * h_var_73_6 = (float *)malloc(sizeof(float *));
	float * d_var_73_6;
	cudaMalloc((void **)&d_var_73_6, sizeof(float *));
	
	float * h_var_73_7 = (float *)malloc(sizeof(float *));
	float * d_var_73_7;
	cudaMalloc((void **)&d_var_73_7, sizeof(float *));
	
	float * h_var_73_8 = (float *)malloc(sizeof(float *));
	float * d_var_73_8;
	cudaMalloc((void **)&d_var_73_8, sizeof(float *));
	
	float * h_var_73_9 = (float *)malloc(sizeof(float *));
	float * d_var_73_9;
	cudaMalloc((void **)&d_var_73_9, sizeof(float *));
	
	float * h_var_74_0 = (float *)malloc(sizeof(float *));
	float * d_var_74_0;
	cudaMalloc((void **)&d_var_74_0, sizeof(float *));
	
	float * h_var_74_1 = (float *)malloc(sizeof(float *));
	float * d_var_74_1;
	cudaMalloc((void **)&d_var_74_1, sizeof(float *));
	
	float * h_var_74_2 = (float *)malloc(sizeof(float *));
	float * d_var_74_2;
	cudaMalloc((void **)&d_var_74_2, sizeof(float *));
	
	float * h_var_74_3 = (float *)malloc(sizeof(float *));
	float * d_var_74_3;
	cudaMalloc((void **)&d_var_74_3, sizeof(float *));
	
	float * h_var_74_4 = (float *)malloc(sizeof(float *));
	float * d_var_74_4;
	cudaMalloc((void **)&d_var_74_4, sizeof(float *));
	
	float * h_var_74_5 = (float *)malloc(sizeof(float *));
	float * d_var_74_5;
	cudaMalloc((void **)&d_var_74_5, sizeof(float *));
	
	float * h_var_74_6 = (float *)malloc(sizeof(float *));
	float * d_var_74_6;
	cudaMalloc((void **)&d_var_74_6, sizeof(float *));
	
	float * h_var_74_7 = (float *)malloc(sizeof(float *));
	float * d_var_74_7;
	cudaMalloc((void **)&d_var_74_7, sizeof(float *));
	
	float * h_var_74_8 = (float *)malloc(sizeof(float *));
	float * d_var_74_8;
	cudaMalloc((void **)&d_var_74_8, sizeof(float *));
	
	float * h_var_74_9 = (float *)malloc(sizeof(float *));
	float * d_var_74_9;
	cudaMalloc((void **)&d_var_74_9, sizeof(float *));
	
	float * h_var_75_0 = (float *)malloc(sizeof(float *));
	float * d_var_75_0;
	cudaMalloc((void **)&d_var_75_0, sizeof(float *));
	
	float * h_var_75_1 = (float *)malloc(sizeof(float *));
	float * d_var_75_1;
	cudaMalloc((void **)&d_var_75_1, sizeof(float *));
	
	float * h_var_75_2 = (float *)malloc(sizeof(float *));
	float * d_var_75_2;
	cudaMalloc((void **)&d_var_75_2, sizeof(float *));
	
	float * h_var_75_3 = (float *)malloc(sizeof(float *));
	float * d_var_75_3;
	cudaMalloc((void **)&d_var_75_3, sizeof(float *));
	
	float * h_var_75_4 = (float *)malloc(sizeof(float *));
	float * d_var_75_4;
	cudaMalloc((void **)&d_var_75_4, sizeof(float *));
	
	float * h_var_75_5 = (float *)malloc(sizeof(float *));
	float * d_var_75_5;
	cudaMalloc((void **)&d_var_75_5, sizeof(float *));
	
	float * h_var_75_6 = (float *)malloc(sizeof(float *));
	float * d_var_75_6;
	cudaMalloc((void **)&d_var_75_6, sizeof(float *));
	
	float * h_var_75_7 = (float *)malloc(sizeof(float *));
	float * d_var_75_7;
	cudaMalloc((void **)&d_var_75_7, sizeof(float *));
	
	float * h_var_75_8 = (float *)malloc(sizeof(float *));
	float * d_var_75_8;
	cudaMalloc((void **)&d_var_75_8, sizeof(float *));
	
	float * h_var_75_9 = (float *)malloc(sizeof(float *));
	float * d_var_75_9;
	cudaMalloc((void **)&d_var_75_9, sizeof(float *));
	
	float * h_var_76_0 = (float *)malloc(sizeof(float *));
	float * d_var_76_0;
	cudaMalloc((void **)&d_var_76_0, sizeof(float *));
	
	float * h_var_76_1 = (float *)malloc(sizeof(float *));
	float * d_var_76_1;
	cudaMalloc((void **)&d_var_76_1, sizeof(float *));
	
	float * h_var_76_2 = (float *)malloc(sizeof(float *));
	float * d_var_76_2;
	cudaMalloc((void **)&d_var_76_2, sizeof(float *));
	
	float * h_var_76_3 = (float *)malloc(sizeof(float *));
	float * d_var_76_3;
	cudaMalloc((void **)&d_var_76_3, sizeof(float *));
	
	float * h_var_76_4 = (float *)malloc(sizeof(float *));
	float * d_var_76_4;
	cudaMalloc((void **)&d_var_76_4, sizeof(float *));
	
	float * h_var_76_5 = (float *)malloc(sizeof(float *));
	float * d_var_76_5;
	cudaMalloc((void **)&d_var_76_5, sizeof(float *));
	
	float * h_var_76_6 = (float *)malloc(sizeof(float *));
	float * d_var_76_6;
	cudaMalloc((void **)&d_var_76_6, sizeof(float *));
	
	float * h_var_76_7 = (float *)malloc(sizeof(float *));
	float * d_var_76_7;
	cudaMalloc((void **)&d_var_76_7, sizeof(float *));
	
	float * h_var_76_8 = (float *)malloc(sizeof(float *));
	float * d_var_76_8;
	cudaMalloc((void **)&d_var_76_8, sizeof(float *));
	
	float * h_var_76_9 = (float *)malloc(sizeof(float *));
	float * d_var_76_9;
	cudaMalloc((void **)&d_var_76_9, sizeof(float *));
	
	float * h_var_77_0 = (float *)malloc(sizeof(float *));
	float * d_var_77_0;
	cudaMalloc((void **)&d_var_77_0, sizeof(float *));
	
	float * h_var_77_1 = (float *)malloc(sizeof(float *));
	float * d_var_77_1;
	cudaMalloc((void **)&d_var_77_1, sizeof(float *));
	
	float * h_var_77_2 = (float *)malloc(sizeof(float *));
	float * d_var_77_2;
	cudaMalloc((void **)&d_var_77_2, sizeof(float *));
	
	float * h_var_77_3 = (float *)malloc(sizeof(float *));
	float * d_var_77_3;
	cudaMalloc((void **)&d_var_77_3, sizeof(float *));
	
	float * h_var_77_4 = (float *)malloc(sizeof(float *));
	float * d_var_77_4;
	cudaMalloc((void **)&d_var_77_4, sizeof(float *));
	
	float * h_var_77_5 = (float *)malloc(sizeof(float *));
	float * d_var_77_5;
	cudaMalloc((void **)&d_var_77_5, sizeof(float *));
	
	float * h_var_77_6 = (float *)malloc(sizeof(float *));
	float * d_var_77_6;
	cudaMalloc((void **)&d_var_77_6, sizeof(float *));
	
	float * h_var_77_7 = (float *)malloc(sizeof(float *));
	float * d_var_77_7;
	cudaMalloc((void **)&d_var_77_7, sizeof(float *));
	
	float * h_var_77_8 = (float *)malloc(sizeof(float *));
	float * d_var_77_8;
	cudaMalloc((void **)&d_var_77_8, sizeof(float *));
	
	float * h_var_77_9 = (float *)malloc(sizeof(float *));
	float * d_var_77_9;
	cudaMalloc((void **)&d_var_77_9, sizeof(float *));
	
	float * h_var_78_0 = (float *)malloc(sizeof(float *));
	float * d_var_78_0;
	cudaMalloc((void **)&d_var_78_0, sizeof(float *));
	
	float * h_var_78_1 = (float *)malloc(sizeof(float *));
	float * d_var_78_1;
	cudaMalloc((void **)&d_var_78_1, sizeof(float *));
	
	float * h_var_78_2 = (float *)malloc(sizeof(float *));
	float * d_var_78_2;
	cudaMalloc((void **)&d_var_78_2, sizeof(float *));
	
	float * h_var_78_3 = (float *)malloc(sizeof(float *));
	float * d_var_78_3;
	cudaMalloc((void **)&d_var_78_3, sizeof(float *));
	
	float * h_var_78_4 = (float *)malloc(sizeof(float *));
	float * d_var_78_4;
	cudaMalloc((void **)&d_var_78_4, sizeof(float *));
	
	float * h_var_78_5 = (float *)malloc(sizeof(float *));
	float * d_var_78_5;
	cudaMalloc((void **)&d_var_78_5, sizeof(float *));
	
	float * h_var_78_6 = (float *)malloc(sizeof(float *));
	float * d_var_78_6;
	cudaMalloc((void **)&d_var_78_6, sizeof(float *));
	
	float * h_var_78_7 = (float *)malloc(sizeof(float *));
	float * d_var_78_7;
	cudaMalloc((void **)&d_var_78_7, sizeof(float *));
	
	float * h_var_78_8 = (float *)malloc(sizeof(float *));
	float * d_var_78_8;
	cudaMalloc((void **)&d_var_78_8, sizeof(float *));
	
	float * h_var_78_9 = (float *)malloc(sizeof(float *));
	float * d_var_78_9;
	cudaMalloc((void **)&d_var_78_9, sizeof(float *));
	
	float * h_var_79_0 = (float *)malloc(sizeof(float *));
	float * d_var_79_0;
	cudaMalloc((void **)&d_var_79_0, sizeof(float *));
	
	float * h_var_79_1 = (float *)malloc(sizeof(float *));
	float * d_var_79_1;
	cudaMalloc((void **)&d_var_79_1, sizeof(float *));
	
	float * h_var_79_2 = (float *)malloc(sizeof(float *));
	float * d_var_79_2;
	cudaMalloc((void **)&d_var_79_2, sizeof(float *));
	
	float * h_var_79_3 = (float *)malloc(sizeof(float *));
	float * d_var_79_3;
	cudaMalloc((void **)&d_var_79_3, sizeof(float *));
	
	float * h_var_79_4 = (float *)malloc(sizeof(float *));
	float * d_var_79_4;
	cudaMalloc((void **)&d_var_79_4, sizeof(float *));
	
	float * h_var_79_5 = (float *)malloc(sizeof(float *));
	float * d_var_79_5;
	cudaMalloc((void **)&d_var_79_5, sizeof(float *));
	
	float * h_var_79_6 = (float *)malloc(sizeof(float *));
	float * d_var_79_6;
	cudaMalloc((void **)&d_var_79_6, sizeof(float *));
	
	float * h_var_79_7 = (float *)malloc(sizeof(float *));
	float * d_var_79_7;
	cudaMalloc((void **)&d_var_79_7, sizeof(float *));
	
	float * h_var_79_8 = (float *)malloc(sizeof(float *));
	float * d_var_79_8;
	cudaMalloc((void **)&d_var_79_8, sizeof(float *));
	
	float * h_var_79_9 = (float *)malloc(sizeof(float *));
	float * d_var_79_9;
	cudaMalloc((void **)&d_var_79_9, sizeof(float *));
	

    // clang-format off
	
	kernel_0<<<10, 10>>>(d_var_0_0, d_var_0_1, d_var_0_2, d_var_0_3, d_var_0_4, d_var_0_5, d_var_0_6, d_var_0_7, d_var_0_8, d_var_0_9);
	
	kernel_1<<<10, 10>>>(d_var_1_0, d_var_1_1, d_var_1_2, d_var_1_3, d_var_1_4, d_var_1_5, d_var_1_6, d_var_1_7, d_var_1_8, d_var_1_9);
	
	kernel_2<<<10, 10>>>(d_var_2_0, d_var_2_1, d_var_2_2, d_var_2_3, d_var_2_4, d_var_2_5, d_var_2_6, d_var_2_7, d_var_2_8, d_var_2_9);
	
	kernel_3<<<10, 10>>>(d_var_3_0, d_var_3_1, d_var_3_2, d_var_3_3, d_var_3_4, d_var_3_5, d_var_3_6, d_var_3_7, d_var_3_8, d_var_3_9);
	
	kernel_4<<<10, 10>>>(d_var_4_0, d_var_4_1, d_var_4_2, d_var_4_3, d_var_4_4, d_var_4_5, d_var_4_6, d_var_4_7, d_var_4_8, d_var_4_9);
	
	kernel_5<<<10, 10>>>(d_var_5_0, d_var_5_1, d_var_5_2, d_var_5_3, d_var_5_4, d_var_5_5, d_var_5_6, d_var_5_7, d_var_5_8, d_var_5_9);
	
	kernel_6<<<10, 10>>>(d_var_6_0, d_var_6_1, d_var_6_2, d_var_6_3, d_var_6_4, d_var_6_5, d_var_6_6, d_var_6_7, d_var_6_8, d_var_6_9);
	
	kernel_7<<<10, 10>>>(d_var_7_0, d_var_7_1, d_var_7_2, d_var_7_3, d_var_7_4, d_var_7_5, d_var_7_6, d_var_7_7, d_var_7_8, d_var_7_9);
	
	kernel_8<<<10, 10>>>(d_var_8_0, d_var_8_1, d_var_8_2, d_var_8_3, d_var_8_4, d_var_8_5, d_var_8_6, d_var_8_7, d_var_8_8, d_var_8_9);
	
	kernel_9<<<10, 10>>>(d_var_9_0, d_var_9_1, d_var_9_2, d_var_9_3, d_var_9_4, d_var_9_5, d_var_9_6, d_var_9_7, d_var_9_8, d_var_9_9);
	
	kernel_10<<<10, 10>>>(d_var_10_0, d_var_10_1, d_var_10_2, d_var_10_3, d_var_10_4, d_var_10_5, d_var_10_6, d_var_10_7, d_var_10_8, d_var_10_9);
	
	kernel_11<<<10, 10>>>(d_var_11_0, d_var_11_1, d_var_11_2, d_var_11_3, d_var_11_4, d_var_11_5, d_var_11_6, d_var_11_7, d_var_11_8, d_var_11_9);
	
	kernel_12<<<10, 10>>>(d_var_12_0, d_var_12_1, d_var_12_2, d_var_12_3, d_var_12_4, d_var_12_5, d_var_12_6, d_var_12_7, d_var_12_8, d_var_12_9);
	
	kernel_13<<<10, 10>>>(d_var_13_0, d_var_13_1, d_var_13_2, d_var_13_3, d_var_13_4, d_var_13_5, d_var_13_6, d_var_13_7, d_var_13_8, d_var_13_9);
	
	kernel_14<<<10, 10>>>(d_var_14_0, d_var_14_1, d_var_14_2, d_var_14_3, d_var_14_4, d_var_14_5, d_var_14_6, d_var_14_7, d_var_14_8, d_var_14_9);
	
	kernel_15<<<10, 10>>>(d_var_15_0, d_var_15_1, d_var_15_2, d_var_15_3, d_var_15_4, d_var_15_5, d_var_15_6, d_var_15_7, d_var_15_8, d_var_15_9);
	
	kernel_16<<<10, 10>>>(d_var_16_0, d_var_16_1, d_var_16_2, d_var_16_3, d_var_16_4, d_var_16_5, d_var_16_6, d_var_16_7, d_var_16_8, d_var_16_9);
	
	kernel_17<<<10, 10>>>(d_var_17_0, d_var_17_1, d_var_17_2, d_var_17_3, d_var_17_4, d_var_17_5, d_var_17_6, d_var_17_7, d_var_17_8, d_var_17_9);
	
	kernel_18<<<10, 10>>>(d_var_18_0, d_var_18_1, d_var_18_2, d_var_18_3, d_var_18_4, d_var_18_5, d_var_18_6, d_var_18_7, d_var_18_8, d_var_18_9);
	
	kernel_19<<<10, 10>>>(d_var_19_0, d_var_19_1, d_var_19_2, d_var_19_3, d_var_19_4, d_var_19_5, d_var_19_6, d_var_19_7, d_var_19_8, d_var_19_9);
	
	kernel_20<<<10, 10>>>(d_var_20_0, d_var_20_1, d_var_20_2, d_var_20_3, d_var_20_4, d_var_20_5, d_var_20_6, d_var_20_7, d_var_20_8, d_var_20_9);
	
	kernel_21<<<10, 10>>>(d_var_21_0, d_var_21_1, d_var_21_2, d_var_21_3, d_var_21_4, d_var_21_5, d_var_21_6, d_var_21_7, d_var_21_8, d_var_21_9);
	
	kernel_22<<<10, 10>>>(d_var_22_0, d_var_22_1, d_var_22_2, d_var_22_3, d_var_22_4, d_var_22_5, d_var_22_6, d_var_22_7, d_var_22_8, d_var_22_9);
	
	kernel_23<<<10, 10>>>(d_var_23_0, d_var_23_1, d_var_23_2, d_var_23_3, d_var_23_4, d_var_23_5, d_var_23_6, d_var_23_7, d_var_23_8, d_var_23_9);
	
	kernel_24<<<10, 10>>>(d_var_24_0, d_var_24_1, d_var_24_2, d_var_24_3, d_var_24_4, d_var_24_5, d_var_24_6, d_var_24_7, d_var_24_8, d_var_24_9);
	
	kernel_25<<<10, 10>>>(d_var_25_0, d_var_25_1, d_var_25_2, d_var_25_3, d_var_25_4, d_var_25_5, d_var_25_6, d_var_25_7, d_var_25_8, d_var_25_9);
	
	kernel_26<<<10, 10>>>(d_var_26_0, d_var_26_1, d_var_26_2, d_var_26_3, d_var_26_4, d_var_26_5, d_var_26_6, d_var_26_7, d_var_26_8, d_var_26_9);
	
	kernel_27<<<10, 10>>>(d_var_27_0, d_var_27_1, d_var_27_2, d_var_27_3, d_var_27_4, d_var_27_5, d_var_27_6, d_var_27_7, d_var_27_8, d_var_27_9);
	
	kernel_28<<<10, 10>>>(d_var_28_0, d_var_28_1, d_var_28_2, d_var_28_3, d_var_28_4, d_var_28_5, d_var_28_6, d_var_28_7, d_var_28_8, d_var_28_9);
	
	kernel_29<<<10, 10>>>(d_var_29_0, d_var_29_1, d_var_29_2, d_var_29_3, d_var_29_4, d_var_29_5, d_var_29_6, d_var_29_7, d_var_29_8, d_var_29_9);
	
	kernel_30<<<10, 10>>>(d_var_30_0, d_var_30_1, d_var_30_2, d_var_30_3, d_var_30_4, d_var_30_5, d_var_30_6, d_var_30_7, d_var_30_8, d_var_30_9);
	
	kernel_31<<<10, 10>>>(d_var_31_0, d_var_31_1, d_var_31_2, d_var_31_3, d_var_31_4, d_var_31_5, d_var_31_6, d_var_31_7, d_var_31_8, d_var_31_9);
	
	kernel_32<<<10, 10>>>(d_var_32_0, d_var_32_1, d_var_32_2, d_var_32_3, d_var_32_4, d_var_32_5, d_var_32_6, d_var_32_7, d_var_32_8, d_var_32_9);
	
	kernel_33<<<10, 10>>>(d_var_33_0, d_var_33_1, d_var_33_2, d_var_33_3, d_var_33_4, d_var_33_5, d_var_33_6, d_var_33_7, d_var_33_8, d_var_33_9);
	
	kernel_34<<<10, 10>>>(d_var_34_0, d_var_34_1, d_var_34_2, d_var_34_3, d_var_34_4, d_var_34_5, d_var_34_6, d_var_34_7, d_var_34_8, d_var_34_9);
	
	kernel_35<<<10, 10>>>(d_var_35_0, d_var_35_1, d_var_35_2, d_var_35_3, d_var_35_4, d_var_35_5, d_var_35_6, d_var_35_7, d_var_35_8, d_var_35_9);
	
	kernel_36<<<10, 10>>>(d_var_36_0, d_var_36_1, d_var_36_2, d_var_36_3, d_var_36_4, d_var_36_5, d_var_36_6, d_var_36_7, d_var_36_8, d_var_36_9);
	
	kernel_37<<<10, 10>>>(d_var_37_0, d_var_37_1, d_var_37_2, d_var_37_3, d_var_37_4, d_var_37_5, d_var_37_6, d_var_37_7, d_var_37_8, d_var_37_9);
	
	kernel_38<<<10, 10>>>(d_var_38_0, d_var_38_1, d_var_38_2, d_var_38_3, d_var_38_4, d_var_38_5, d_var_38_6, d_var_38_7, d_var_38_8, d_var_38_9);
	
	kernel_39<<<10, 10>>>(d_var_39_0, d_var_39_1, d_var_39_2, d_var_39_3, d_var_39_4, d_var_39_5, d_var_39_6, d_var_39_7, d_var_39_8, d_var_39_9);
	
	kernel_40<<<10, 10>>>(d_var_40_0, d_var_40_1, d_var_40_2, d_var_40_3, d_var_40_4, d_var_40_5, d_var_40_6, d_var_40_7, d_var_40_8, d_var_40_9);
	
	kernel_41<<<10, 10>>>(d_var_41_0, d_var_41_1, d_var_41_2, d_var_41_3, d_var_41_4, d_var_41_5, d_var_41_6, d_var_41_7, d_var_41_8, d_var_41_9);
	
	kernel_42<<<10, 10>>>(d_var_42_0, d_var_42_1, d_var_42_2, d_var_42_3, d_var_42_4, d_var_42_5, d_var_42_6, d_var_42_7, d_var_42_8, d_var_42_9);
	
	kernel_43<<<10, 10>>>(d_var_43_0, d_var_43_1, d_var_43_2, d_var_43_3, d_var_43_4, d_var_43_5, d_var_43_6, d_var_43_7, d_var_43_8, d_var_43_9);
	
	kernel_44<<<10, 10>>>(d_var_44_0, d_var_44_1, d_var_44_2, d_var_44_3, d_var_44_4, d_var_44_5, d_var_44_6, d_var_44_7, d_var_44_8, d_var_44_9);
	
	kernel_45<<<10, 10>>>(d_var_45_0, d_var_45_1, d_var_45_2, d_var_45_3, d_var_45_4, d_var_45_5, d_var_45_6, d_var_45_7, d_var_45_8, d_var_45_9);
	
	kernel_46<<<10, 10>>>(d_var_46_0, d_var_46_1, d_var_46_2, d_var_46_3, d_var_46_4, d_var_46_5, d_var_46_6, d_var_46_7, d_var_46_8, d_var_46_9);
	
	kernel_47<<<10, 10>>>(d_var_47_0, d_var_47_1, d_var_47_2, d_var_47_3, d_var_47_4, d_var_47_5, d_var_47_6, d_var_47_7, d_var_47_8, d_var_47_9);
	
	kernel_48<<<10, 10>>>(d_var_48_0, d_var_48_1, d_var_48_2, d_var_48_3, d_var_48_4, d_var_48_5, d_var_48_6, d_var_48_7, d_var_48_8, d_var_48_9);
	
	kernel_49<<<10, 10>>>(d_var_49_0, d_var_49_1, d_var_49_2, d_var_49_3, d_var_49_4, d_var_49_5, d_var_49_6, d_var_49_7, d_var_49_8, d_var_49_9);
	
	kernel_50<<<10, 10>>>(d_var_50_0, d_var_50_1, d_var_50_2, d_var_50_3, d_var_50_4, d_var_50_5, d_var_50_6, d_var_50_7, d_var_50_8, d_var_50_9);
	
	kernel_51<<<10, 10>>>(d_var_51_0, d_var_51_1, d_var_51_2, d_var_51_3, d_var_51_4, d_var_51_5, d_var_51_6, d_var_51_7, d_var_51_8, d_var_51_9);
	
	kernel_52<<<10, 10>>>(d_var_52_0, d_var_52_1, d_var_52_2, d_var_52_3, d_var_52_4, d_var_52_5, d_var_52_6, d_var_52_7, d_var_52_8, d_var_52_9);
	
	kernel_53<<<10, 10>>>(d_var_53_0, d_var_53_1, d_var_53_2, d_var_53_3, d_var_53_4, d_var_53_5, d_var_53_6, d_var_53_7, d_var_53_8, d_var_53_9);
	
	kernel_54<<<10, 10>>>(d_var_54_0, d_var_54_1, d_var_54_2, d_var_54_3, d_var_54_4, d_var_54_5, d_var_54_6, d_var_54_7, d_var_54_8, d_var_54_9);
	
	kernel_55<<<10, 10>>>(d_var_55_0, d_var_55_1, d_var_55_2, d_var_55_3, d_var_55_4, d_var_55_5, d_var_55_6, d_var_55_7, d_var_55_8, d_var_55_9);
	
	kernel_56<<<10, 10>>>(d_var_56_0, d_var_56_1, d_var_56_2, d_var_56_3, d_var_56_4, d_var_56_5, d_var_56_6, d_var_56_7, d_var_56_8, d_var_56_9);
	
	kernel_57<<<10, 10>>>(d_var_57_0, d_var_57_1, d_var_57_2, d_var_57_3, d_var_57_4, d_var_57_5, d_var_57_6, d_var_57_7, d_var_57_8, d_var_57_9);
	
	kernel_58<<<10, 10>>>(d_var_58_0, d_var_58_1, d_var_58_2, d_var_58_3, d_var_58_4, d_var_58_5, d_var_58_6, d_var_58_7, d_var_58_8, d_var_58_9);
	
	kernel_59<<<10, 10>>>(d_var_59_0, d_var_59_1, d_var_59_2, d_var_59_3, d_var_59_4, d_var_59_5, d_var_59_6, d_var_59_7, d_var_59_8, d_var_59_9);
	
	kernel_60<<<10, 10>>>(d_var_60_0, d_var_60_1, d_var_60_2, d_var_60_3, d_var_60_4, d_var_60_5, d_var_60_6, d_var_60_7, d_var_60_8, d_var_60_9);
	
	kernel_61<<<10, 10>>>(d_var_61_0, d_var_61_1, d_var_61_2, d_var_61_3, d_var_61_4, d_var_61_5, d_var_61_6, d_var_61_7, d_var_61_8, d_var_61_9);
	
	kernel_62<<<10, 10>>>(d_var_62_0, d_var_62_1, d_var_62_2, d_var_62_3, d_var_62_4, d_var_62_5, d_var_62_6, d_var_62_7, d_var_62_8, d_var_62_9);
	
	kernel_63<<<10, 10>>>(d_var_63_0, d_var_63_1, d_var_63_2, d_var_63_3, d_var_63_4, d_var_63_5, d_var_63_6, d_var_63_7, d_var_63_8, d_var_63_9);
	
	kernel_64<<<10, 10>>>(d_var_64_0, d_var_64_1, d_var_64_2, d_var_64_3, d_var_64_4, d_var_64_5, d_var_64_6, d_var_64_7, d_var_64_8, d_var_64_9);
	
	kernel_65<<<10, 10>>>(d_var_65_0, d_var_65_1, d_var_65_2, d_var_65_3, d_var_65_4, d_var_65_5, d_var_65_6, d_var_65_7, d_var_65_8, d_var_65_9);
	
	kernel_66<<<10, 10>>>(d_var_66_0, d_var_66_1, d_var_66_2, d_var_66_3, d_var_66_4, d_var_66_5, d_var_66_6, d_var_66_7, d_var_66_8, d_var_66_9);
	
	kernel_67<<<10, 10>>>(d_var_67_0, d_var_67_1, d_var_67_2, d_var_67_3, d_var_67_4, d_var_67_5, d_var_67_6, d_var_67_7, d_var_67_8, d_var_67_9);
	
	kernel_68<<<10, 10>>>(d_var_68_0, d_var_68_1, d_var_68_2, d_var_68_3, d_var_68_4, d_var_68_5, d_var_68_6, d_var_68_7, d_var_68_8, d_var_68_9);
	
	kernel_69<<<10, 10>>>(d_var_69_0, d_var_69_1, d_var_69_2, d_var_69_3, d_var_69_4, d_var_69_5, d_var_69_6, d_var_69_7, d_var_69_8, d_var_69_9);
	
	kernel_70<<<10, 10>>>(d_var_70_0, d_var_70_1, d_var_70_2, d_var_70_3, d_var_70_4, d_var_70_5, d_var_70_6, d_var_70_7, d_var_70_8, d_var_70_9);
	
	kernel_71<<<10, 10>>>(d_var_71_0, d_var_71_1, d_var_71_2, d_var_71_3, d_var_71_4, d_var_71_5, d_var_71_6, d_var_71_7, d_var_71_8, d_var_71_9);
	
	kernel_72<<<10, 10>>>(d_var_72_0, d_var_72_1, d_var_72_2, d_var_72_3, d_var_72_4, d_var_72_5, d_var_72_6, d_var_72_7, d_var_72_8, d_var_72_9);
	
	kernel_73<<<10, 10>>>(d_var_73_0, d_var_73_1, d_var_73_2, d_var_73_3, d_var_73_4, d_var_73_5, d_var_73_6, d_var_73_7, d_var_73_8, d_var_73_9);
	
	kernel_74<<<10, 10>>>(d_var_74_0, d_var_74_1, d_var_74_2, d_var_74_3, d_var_74_4, d_var_74_5, d_var_74_6, d_var_74_7, d_var_74_8, d_var_74_9);
	
	kernel_75<<<10, 10>>>(d_var_75_0, d_var_75_1, d_var_75_2, d_var_75_3, d_var_75_4, d_var_75_5, d_var_75_6, d_var_75_7, d_var_75_8, d_var_75_9);
	
	kernel_76<<<10, 10>>>(d_var_76_0, d_var_76_1, d_var_76_2, d_var_76_3, d_var_76_4, d_var_76_5, d_var_76_6, d_var_76_7, d_var_76_8, d_var_76_9);
	
	kernel_77<<<10, 10>>>(d_var_77_0, d_var_77_1, d_var_77_2, d_var_77_3, d_var_77_4, d_var_77_5, d_var_77_6, d_var_77_7, d_var_77_8, d_var_77_9);
	
	kernel_78<<<10, 10>>>(d_var_78_0, d_var_78_1, d_var_78_2, d_var_78_3, d_var_78_4, d_var_78_5, d_var_78_6, d_var_78_7, d_var_78_8, d_var_78_9);
	
	kernel_79<<<10, 10>>>(d_var_79_0, d_var_79_1, d_var_79_2, d_var_79_3, d_var_79_4, d_var_79_5, d_var_79_6, d_var_79_7, d_var_79_8, d_var_79_9);
	
    // clang-format on

    printf("Done\n");
    return 0;
}
