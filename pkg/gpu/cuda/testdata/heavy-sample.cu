/* AUTO-GENERATED, DO NOT CHANGE */

// To regenerate, run `make heavy-sample.cu` in this directory

#include <stdio.h>

#include <cuda_runtime.h>


__global__ void kernel_0(float * var_0_0, float * var_0_1, float * var_0_2, float * var_0_3, float * var_0_4, float * var_0_5, float * var_0_6, float * var_0_7, float * var_0_8, float * var_0_9, float * var_0_10, float * var_0_11, float * var_0_12, float * var_0_13, float * var_0_14, float * var_0_15, float * var_0_16, float * var_0_17, float * var_0_18, float * var_0_19) {
	__shared__ float myVar[1024];
	myVar[7] = 13.358492 * myVar[threadIdx.x];
	myVar[0] = 29.996864 * myVar[threadIdx.x];
	myVar[7] = 26.850372 * myVar[threadIdx.x];
	myVar[7] = 47.405648 * myVar[threadIdx.x];
	myVar[8] = 28.605543 * myVar[threadIdx.x];
	myVar[8] = 30.566722 * myVar[threadIdx.x];
	myVar[3] = 8.149732 * myVar[threadIdx.x];
	myVar[2] = 42.915701 * myVar[threadIdx.x];
	myVar[0] = 36.084747 * myVar[threadIdx.x];
	myVar[6] = 2.276106 * myVar[threadIdx.x];
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
	var_0_10[10] = myVar[10];
	var_0_11[11] = myVar[11];
	var_0_12[12] = myVar[12];
	var_0_13[13] = myVar[13];
	var_0_14[14] = myVar[14];
	var_0_15[15] = myVar[15];
	var_0_16[16] = myVar[16];
	var_0_17[17] = myVar[17];
	var_0_18[18] = myVar[18];
	var_0_19[19] = myVar[19];
	
}

__global__ void kernel_1(float * var_1_0, float * var_1_1, float * var_1_2, float * var_1_3, float * var_1_4, float * var_1_5, float * var_1_6, float * var_1_7, float * var_1_8, float * var_1_9, float * var_1_10, float * var_1_11, float * var_1_12, float * var_1_13, float * var_1_14, float * var_1_15, float * var_1_16, float * var_1_17, float * var_1_18, float * var_1_19) {
	__shared__ float myVar[1024];
	myVar[4] = 4.577338 * myVar[threadIdx.x];
	myVar[4] = 21.162567 * myVar[threadIdx.x];
	myVar[7] = 0.128262 * myVar[threadIdx.x];
	myVar[8] = 22.890511 * myVar[threadIdx.x];
	myVar[8] = 22.667310 * myVar[threadIdx.x];
	myVar[0] = 29.358554 * myVar[threadIdx.x];
	myVar[5] = 13.730931 * myVar[threadIdx.x];
	myVar[6] = 32.720741 * myVar[threadIdx.x];
	myVar[7] = 11.978346 * myVar[threadIdx.x];
	myVar[2] = 32.092827 * myVar[threadIdx.x];
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
	var_1_10[10] = myVar[10];
	var_1_11[11] = myVar[11];
	var_1_12[12] = myVar[12];
	var_1_13[13] = myVar[13];
	var_1_14[14] = myVar[14];
	var_1_15[15] = myVar[15];
	var_1_16[16] = myVar[16];
	var_1_17[17] = myVar[17];
	var_1_18[18] = myVar[18];
	var_1_19[19] = myVar[19];
	
}

__global__ void kernel_2(float * var_2_0, float * var_2_1, float * var_2_2, float * var_2_3, float * var_2_4, float * var_2_5, float * var_2_6, float * var_2_7, float * var_2_8, float * var_2_9, float * var_2_10, float * var_2_11, float * var_2_12, float * var_2_13, float * var_2_14, float * var_2_15, float * var_2_16, float * var_2_17, float * var_2_18, float * var_2_19) {
	__shared__ float myVar[1024];
	myVar[3] = 49.191664 * myVar[threadIdx.x];
	myVar[3] = 5.722245 * myVar[threadIdx.x];
	myVar[5] = 3.621897 * myVar[threadIdx.x];
	myVar[0] = 10.483091 * myVar[threadIdx.x];
	myVar[6] = 36.425846 * myVar[threadIdx.x];
	myVar[9] = 49.066374 * myVar[threadIdx.x];
	myVar[3] = 41.370231 * myVar[threadIdx.x];
	myVar[2] = 38.310331 * myVar[threadIdx.x];
	myVar[2] = 38.696066 * myVar[threadIdx.x];
	myVar[5] = 17.802057 * myVar[threadIdx.x];
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
	var_2_10[10] = myVar[10];
	var_2_11[11] = myVar[11];
	var_2_12[12] = myVar[12];
	var_2_13[13] = myVar[13];
	var_2_14[14] = myVar[14];
	var_2_15[15] = myVar[15];
	var_2_16[16] = myVar[16];
	var_2_17[17] = myVar[17];
	var_2_18[18] = myVar[18];
	var_2_19[19] = myVar[19];
	
}

__global__ void kernel_3(float * var_3_0, float * var_3_1, float * var_3_2, float * var_3_3, float * var_3_4, float * var_3_5, float * var_3_6, float * var_3_7, float * var_3_8, float * var_3_9, float * var_3_10, float * var_3_11, float * var_3_12, float * var_3_13, float * var_3_14, float * var_3_15, float * var_3_16, float * var_3_17, float * var_3_18, float * var_3_19) {
	__shared__ float myVar[1024];
	myVar[1] = 3.657881 * myVar[threadIdx.x];
	myVar[9] = 47.144202 * myVar[threadIdx.x];
	myVar[7] = 26.768394 * myVar[threadIdx.x];
	myVar[4] = 25.254215 * myVar[threadIdx.x];
	myVar[8] = 36.901003 * myVar[threadIdx.x];
	myVar[5] = 12.665010 * myVar[threadIdx.x];
	myVar[6] = 12.501012 * myVar[threadIdx.x];
	myVar[2] = 33.849199 * myVar[threadIdx.x];
	myVar[1] = 44.025130 * myVar[threadIdx.x];
	myVar[1] = 48.566763 * myVar[threadIdx.x];
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
	var_3_10[10] = myVar[10];
	var_3_11[11] = myVar[11];
	var_3_12[12] = myVar[12];
	var_3_13[13] = myVar[13];
	var_3_14[14] = myVar[14];
	var_3_15[15] = myVar[15];
	var_3_16[16] = myVar[16];
	var_3_17[17] = myVar[17];
	var_3_18[18] = myVar[18];
	var_3_19[19] = myVar[19];
	
}

__global__ void kernel_4(float * var_4_0, float * var_4_1, float * var_4_2, float * var_4_3, float * var_4_4, float * var_4_5, float * var_4_6, float * var_4_7, float * var_4_8, float * var_4_9, float * var_4_10, float * var_4_11, float * var_4_12, float * var_4_13, float * var_4_14, float * var_4_15, float * var_4_16, float * var_4_17, float * var_4_18, float * var_4_19) {
	__shared__ float myVar[1024];
	myVar[9] = 34.814125 * myVar[threadIdx.x];
	myVar[0] = 7.439284 * myVar[threadIdx.x];
	myVar[8] = 24.182738 * myVar[threadIdx.x];
	myVar[6] = 41.521589 * myVar[threadIdx.x];
	myVar[0] = 8.476383 * myVar[threadIdx.x];
	myVar[4] = 4.308362 * myVar[threadIdx.x];
	myVar[0] = 34.019957 * myVar[threadIdx.x];
	myVar[4] = 14.880842 * myVar[threadIdx.x];
	myVar[1] = 25.167021 * myVar[threadIdx.x];
	myVar[8] = 14.620295 * myVar[threadIdx.x];
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
	var_4_10[10] = myVar[10];
	var_4_11[11] = myVar[11];
	var_4_12[12] = myVar[12];
	var_4_13[13] = myVar[13];
	var_4_14[14] = myVar[14];
	var_4_15[15] = myVar[15];
	var_4_16[16] = myVar[16];
	var_4_17[17] = myVar[17];
	var_4_18[18] = myVar[18];
	var_4_19[19] = myVar[19];
	
}

__global__ void kernel_5(float * var_5_0, float * var_5_1, float * var_5_2, float * var_5_3, float * var_5_4, float * var_5_5, float * var_5_6, float * var_5_7, float * var_5_8, float * var_5_9, float * var_5_10, float * var_5_11, float * var_5_12, float * var_5_13, float * var_5_14, float * var_5_15, float * var_5_16, float * var_5_17, float * var_5_18, float * var_5_19) {
	__shared__ float myVar[1024];
	myVar[2] = 37.042397 * myVar[threadIdx.x];
	myVar[0] = 15.897506 * myVar[threadIdx.x];
	myVar[9] = 29.881857 * myVar[threadIdx.x];
	myVar[4] = 28.287586 * myVar[threadIdx.x];
	myVar[8] = 13.202707 * myVar[threadIdx.x];
	myVar[4] = 23.235843 * myVar[threadIdx.x];
	myVar[3] = 45.601563 * myVar[threadIdx.x];
	myVar[0] = 22.131163 * myVar[threadIdx.x];
	myVar[1] = 2.447910 * myVar[threadIdx.x];
	myVar[6] = 48.949264 * myVar[threadIdx.x];
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
	var_5_10[10] = myVar[10];
	var_5_11[11] = myVar[11];
	var_5_12[12] = myVar[12];
	var_5_13[13] = myVar[13];
	var_5_14[14] = myVar[14];
	var_5_15[15] = myVar[15];
	var_5_16[16] = myVar[16];
	var_5_17[17] = myVar[17];
	var_5_18[18] = myVar[18];
	var_5_19[19] = myVar[19];
	
}

__global__ void kernel_6(float * var_6_0, float * var_6_1, float * var_6_2, float * var_6_3, float * var_6_4, float * var_6_5, float * var_6_6, float * var_6_7, float * var_6_8, float * var_6_9, float * var_6_10, float * var_6_11, float * var_6_12, float * var_6_13, float * var_6_14, float * var_6_15, float * var_6_16, float * var_6_17, float * var_6_18, float * var_6_19) {
	__shared__ float myVar[1024];
	myVar[5] = 42.612716 * myVar[threadIdx.x];
	myVar[1] = 32.709412 * myVar[threadIdx.x];
	myVar[2] = 1.664703 * myVar[threadIdx.x];
	myVar[7] = 17.973638 * myVar[threadIdx.x];
	myVar[1] = 20.774570 * myVar[threadIdx.x];
	myVar[5] = 16.371010 * myVar[threadIdx.x];
	myVar[3] = 19.487553 * myVar[threadIdx.x];
	myVar[3] = 36.774980 * myVar[threadIdx.x];
	myVar[5] = 28.604668 * myVar[threadIdx.x];
	myVar[2] = 15.117050 * myVar[threadIdx.x];
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
	var_6_10[10] = myVar[10];
	var_6_11[11] = myVar[11];
	var_6_12[12] = myVar[12];
	var_6_13[13] = myVar[13];
	var_6_14[14] = myVar[14];
	var_6_15[15] = myVar[15];
	var_6_16[16] = myVar[16];
	var_6_17[17] = myVar[17];
	var_6_18[18] = myVar[18];
	var_6_19[19] = myVar[19];
	
}

__global__ void kernel_7(float * var_7_0, float * var_7_1, float * var_7_2, float * var_7_3, float * var_7_4, float * var_7_5, float * var_7_6, float * var_7_7, float * var_7_8, float * var_7_9, float * var_7_10, float * var_7_11, float * var_7_12, float * var_7_13, float * var_7_14, float * var_7_15, float * var_7_16, float * var_7_17, float * var_7_18, float * var_7_19) {
	__shared__ float myVar[1024];
	myVar[4] = 9.570039 * myVar[threadIdx.x];
	myVar[9] = 7.446622 * myVar[threadIdx.x];
	myVar[8] = 4.543599 * myVar[threadIdx.x];
	myVar[7] = 26.174945 * myVar[threadIdx.x];
	myVar[4] = 13.540677 * myVar[threadIdx.x];
	myVar[9] = 33.803400 * myVar[threadIdx.x];
	myVar[8] = 24.045981 * myVar[threadIdx.x];
	myVar[3] = 22.198640 * myVar[threadIdx.x];
	myVar[9] = 22.882328 * myVar[threadIdx.x];
	myVar[3] = 9.353756 * myVar[threadIdx.x];
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
	var_7_10[10] = myVar[10];
	var_7_11[11] = myVar[11];
	var_7_12[12] = myVar[12];
	var_7_13[13] = myVar[13];
	var_7_14[14] = myVar[14];
	var_7_15[15] = myVar[15];
	var_7_16[16] = myVar[16];
	var_7_17[17] = myVar[17];
	var_7_18[18] = myVar[18];
	var_7_19[19] = myVar[19];
	
}

__global__ void kernel_8(float * var_8_0, float * var_8_1, float * var_8_2, float * var_8_3, float * var_8_4, float * var_8_5, float * var_8_6, float * var_8_7, float * var_8_8, float * var_8_9, float * var_8_10, float * var_8_11, float * var_8_12, float * var_8_13, float * var_8_14, float * var_8_15, float * var_8_16, float * var_8_17, float * var_8_18, float * var_8_19) {
	__shared__ float myVar[1024];
	myVar[7] = 15.547479 * myVar[threadIdx.x];
	myVar[3] = 48.901650 * myVar[threadIdx.x];
	myVar[5] = 26.480024 * myVar[threadIdx.x];
	myVar[6] = 3.905452 * myVar[threadIdx.x];
	myVar[8] = 10.110801 * myVar[threadIdx.x];
	myVar[5] = 39.252634 * myVar[threadIdx.x];
	myVar[0] = 40.038305 * myVar[threadIdx.x];
	myVar[7] = 35.166664 * myVar[threadIdx.x];
	myVar[2] = 37.185844 * myVar[threadIdx.x];
	myVar[5] = 18.754616 * myVar[threadIdx.x];
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
	var_8_10[10] = myVar[10];
	var_8_11[11] = myVar[11];
	var_8_12[12] = myVar[12];
	var_8_13[13] = myVar[13];
	var_8_14[14] = myVar[14];
	var_8_15[15] = myVar[15];
	var_8_16[16] = myVar[16];
	var_8_17[17] = myVar[17];
	var_8_18[18] = myVar[18];
	var_8_19[19] = myVar[19];
	
}

__global__ void kernel_9(float * var_9_0, float * var_9_1, float * var_9_2, float * var_9_3, float * var_9_4, float * var_9_5, float * var_9_6, float * var_9_7, float * var_9_8, float * var_9_9, float * var_9_10, float * var_9_11, float * var_9_12, float * var_9_13, float * var_9_14, float * var_9_15, float * var_9_16, float * var_9_17, float * var_9_18, float * var_9_19) {
	__shared__ float myVar[1024];
	myVar[6] = 14.290958 * myVar[threadIdx.x];
	myVar[8] = 42.139091 * myVar[threadIdx.x];
	myVar[8] = 36.378596 * myVar[threadIdx.x];
	myVar[3] = 46.441771 * myVar[threadIdx.x];
	myVar[6] = 8.854676 * myVar[threadIdx.x];
	myVar[8] = 20.100396 * myVar[threadIdx.x];
	myVar[6] = 8.781566 * myVar[threadIdx.x];
	myVar[8] = 44.446736 * myVar[threadIdx.x];
	myVar[5] = 7.496880 * myVar[threadIdx.x];
	myVar[8] = 40.785167 * myVar[threadIdx.x];
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
	var_9_10[10] = myVar[10];
	var_9_11[11] = myVar[11];
	var_9_12[12] = myVar[12];
	var_9_13[13] = myVar[13];
	var_9_14[14] = myVar[14];
	var_9_15[15] = myVar[15];
	var_9_16[16] = myVar[16];
	var_9_17[17] = myVar[17];
	var_9_18[18] = myVar[18];
	var_9_19[19] = myVar[19];
	
}

__global__ void kernel_10(float * var_10_0, float * var_10_1, float * var_10_2, float * var_10_3, float * var_10_4, float * var_10_5, float * var_10_6, float * var_10_7, float * var_10_8, float * var_10_9, float * var_10_10, float * var_10_11, float * var_10_12, float * var_10_13, float * var_10_14, float * var_10_15, float * var_10_16, float * var_10_17, float * var_10_18, float * var_10_19) {
	__shared__ float myVar[1024];
	myVar[6] = 41.374924 * myVar[threadIdx.x];
	myVar[7] = 5.583937 * myVar[threadIdx.x];
	myVar[1] = 11.415106 * myVar[threadIdx.x];
	myVar[7] = 16.040032 * myVar[threadIdx.x];
	myVar[9] = 49.003678 * myVar[threadIdx.x];
	myVar[9] = 33.529291 * myVar[threadIdx.x];
	myVar[9] = 3.725300 * myVar[threadIdx.x];
	myVar[9] = 36.318961 * myVar[threadIdx.x];
	myVar[4] = 34.676808 * myVar[threadIdx.x];
	myVar[8] = 0.789490 * myVar[threadIdx.x];
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
	var_10_10[10] = myVar[10];
	var_10_11[11] = myVar[11];
	var_10_12[12] = myVar[12];
	var_10_13[13] = myVar[13];
	var_10_14[14] = myVar[14];
	var_10_15[15] = myVar[15];
	var_10_16[16] = myVar[16];
	var_10_17[17] = myVar[17];
	var_10_18[18] = myVar[18];
	var_10_19[19] = myVar[19];
	
}

__global__ void kernel_11(float * var_11_0, float * var_11_1, float * var_11_2, float * var_11_3, float * var_11_4, float * var_11_5, float * var_11_6, float * var_11_7, float * var_11_8, float * var_11_9, float * var_11_10, float * var_11_11, float * var_11_12, float * var_11_13, float * var_11_14, float * var_11_15, float * var_11_16, float * var_11_17, float * var_11_18, float * var_11_19) {
	__shared__ float myVar[1024];
	myVar[5] = 27.667212 * myVar[threadIdx.x];
	myVar[2] = 12.702892 * myVar[threadIdx.x];
	myVar[2] = 18.730612 * myVar[threadIdx.x];
	myVar[9] = 8.009668 * myVar[threadIdx.x];
	myVar[9] = 1.903979 * myVar[threadIdx.x];
	myVar[9] = 20.042565 * myVar[threadIdx.x];
	myVar[0] = 34.292619 * myVar[threadIdx.x];
	myVar[9] = 39.456481 * myVar[threadIdx.x];
	myVar[1] = 25.805260 * myVar[threadIdx.x];
	myVar[7] = 10.881045 * myVar[threadIdx.x];
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
	var_11_10[10] = myVar[10];
	var_11_11[11] = myVar[11];
	var_11_12[12] = myVar[12];
	var_11_13[13] = myVar[13];
	var_11_14[14] = myVar[14];
	var_11_15[15] = myVar[15];
	var_11_16[16] = myVar[16];
	var_11_17[17] = myVar[17];
	var_11_18[18] = myVar[18];
	var_11_19[19] = myVar[19];
	
}

__global__ void kernel_12(float * var_12_0, float * var_12_1, float * var_12_2, float * var_12_3, float * var_12_4, float * var_12_5, float * var_12_6, float * var_12_7, float * var_12_8, float * var_12_9, float * var_12_10, float * var_12_11, float * var_12_12, float * var_12_13, float * var_12_14, float * var_12_15, float * var_12_16, float * var_12_17, float * var_12_18, float * var_12_19) {
	__shared__ float myVar[1024];
	myVar[4] = 25.536327 * myVar[threadIdx.x];
	myVar[6] = 5.530078 * myVar[threadIdx.x];
	myVar[4] = 1.930924 * myVar[threadIdx.x];
	myVar[9] = 38.945469 * myVar[threadIdx.x];
	myVar[9] = 35.199808 * myVar[threadIdx.x];
	myVar[4] = 45.394917 * myVar[threadIdx.x];
	myVar[1] = 25.106426 * myVar[threadIdx.x];
	myVar[3] = 43.623141 * myVar[threadIdx.x];
	myVar[5] = 39.114022 * myVar[threadIdx.x];
	myVar[9] = 33.734836 * myVar[threadIdx.x];
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
	var_12_10[10] = myVar[10];
	var_12_11[11] = myVar[11];
	var_12_12[12] = myVar[12];
	var_12_13[13] = myVar[13];
	var_12_14[14] = myVar[14];
	var_12_15[15] = myVar[15];
	var_12_16[16] = myVar[16];
	var_12_17[17] = myVar[17];
	var_12_18[18] = myVar[18];
	var_12_19[19] = myVar[19];
	
}

__global__ void kernel_13(float * var_13_0, float * var_13_1, float * var_13_2, float * var_13_3, float * var_13_4, float * var_13_5, float * var_13_6, float * var_13_7, float * var_13_8, float * var_13_9, float * var_13_10, float * var_13_11, float * var_13_12, float * var_13_13, float * var_13_14, float * var_13_15, float * var_13_16, float * var_13_17, float * var_13_18, float * var_13_19) {
	__shared__ float myVar[1024];
	myVar[9] = 37.548477 * myVar[threadIdx.x];
	myVar[2] = 42.389733 * myVar[threadIdx.x];
	myVar[2] = 9.754293 * myVar[threadIdx.x];
	myVar[1] = 24.123587 * myVar[threadIdx.x];
	myVar[2] = 23.260944 * myVar[threadIdx.x];
	myVar[9] = 18.619336 * myVar[threadIdx.x];
	myVar[2] = 17.496481 * myVar[threadIdx.x];
	myVar[1] = 40.139043 * myVar[threadIdx.x];
	myVar[5] = 22.620901 * myVar[threadIdx.x];
	myVar[8] = 29.155860 * myVar[threadIdx.x];
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
	var_13_10[10] = myVar[10];
	var_13_11[11] = myVar[11];
	var_13_12[12] = myVar[12];
	var_13_13[13] = myVar[13];
	var_13_14[14] = myVar[14];
	var_13_15[15] = myVar[15];
	var_13_16[16] = myVar[16];
	var_13_17[17] = myVar[17];
	var_13_18[18] = myVar[18];
	var_13_19[19] = myVar[19];
	
}

__global__ void kernel_14(float * var_14_0, float * var_14_1, float * var_14_2, float * var_14_3, float * var_14_4, float * var_14_5, float * var_14_6, float * var_14_7, float * var_14_8, float * var_14_9, float * var_14_10, float * var_14_11, float * var_14_12, float * var_14_13, float * var_14_14, float * var_14_15, float * var_14_16, float * var_14_17, float * var_14_18, float * var_14_19) {
	__shared__ float myVar[1024];
	myVar[8] = 20.535383 * myVar[threadIdx.x];
	myVar[1] = 35.714406 * myVar[threadIdx.x];
	myVar[9] = 29.199587 * myVar[threadIdx.x];
	myVar[4] = 33.618211 * myVar[threadIdx.x];
	myVar[8] = 9.594275 * myVar[threadIdx.x];
	myVar[9] = 7.102031 * myVar[threadIdx.x];
	myVar[9] = 5.373745 * myVar[threadIdx.x];
	myVar[6] = 33.548882 * myVar[threadIdx.x];
	myVar[7] = 25.948322 * myVar[threadIdx.x];
	myVar[9] = 19.674437 * myVar[threadIdx.x];
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
	var_14_10[10] = myVar[10];
	var_14_11[11] = myVar[11];
	var_14_12[12] = myVar[12];
	var_14_13[13] = myVar[13];
	var_14_14[14] = myVar[14];
	var_14_15[15] = myVar[15];
	var_14_16[16] = myVar[16];
	var_14_17[17] = myVar[17];
	var_14_18[18] = myVar[18];
	var_14_19[19] = myVar[19];
	
}

__global__ void kernel_15(float * var_15_0, float * var_15_1, float * var_15_2, float * var_15_3, float * var_15_4, float * var_15_5, float * var_15_6, float * var_15_7, float * var_15_8, float * var_15_9, float * var_15_10, float * var_15_11, float * var_15_12, float * var_15_13, float * var_15_14, float * var_15_15, float * var_15_16, float * var_15_17, float * var_15_18, float * var_15_19) {
	__shared__ float myVar[1024];
	myVar[4] = 16.821004 * myVar[threadIdx.x];
	myVar[1] = 40.474564 * myVar[threadIdx.x];
	myVar[0] = 4.896632 * myVar[threadIdx.x];
	myVar[0] = 49.157457 * myVar[threadIdx.x];
	myVar[1] = 43.538341 * myVar[threadIdx.x];
	myVar[5] = 3.270043 * myVar[threadIdx.x];
	myVar[5] = 10.193788 * myVar[threadIdx.x];
	myVar[1] = 2.723051 * myVar[threadIdx.x];
	myVar[3] = 14.915672 * myVar[threadIdx.x];
	myVar[8] = 30.460698 * myVar[threadIdx.x];
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
	var_15_10[10] = myVar[10];
	var_15_11[11] = myVar[11];
	var_15_12[12] = myVar[12];
	var_15_13[13] = myVar[13];
	var_15_14[14] = myVar[14];
	var_15_15[15] = myVar[15];
	var_15_16[16] = myVar[16];
	var_15_17[17] = myVar[17];
	var_15_18[18] = myVar[18];
	var_15_19[19] = myVar[19];
	
}

__global__ void kernel_16(float * var_16_0, float * var_16_1, float * var_16_2, float * var_16_3, float * var_16_4, float * var_16_5, float * var_16_6, float * var_16_7, float * var_16_8, float * var_16_9, float * var_16_10, float * var_16_11, float * var_16_12, float * var_16_13, float * var_16_14, float * var_16_15, float * var_16_16, float * var_16_17, float * var_16_18, float * var_16_19) {
	__shared__ float myVar[1024];
	myVar[1] = 30.125380 * myVar[threadIdx.x];
	myVar[5] = 49.279292 * myVar[threadIdx.x];
	myVar[4] = 40.110493 * myVar[threadIdx.x];
	myVar[6] = 1.210120 * myVar[threadIdx.x];
	myVar[5] = 6.750555 * myVar[threadIdx.x];
	myVar[3] = 41.840500 * myVar[threadIdx.x];
	myVar[8] = 32.901153 * myVar[threadIdx.x];
	myVar[3] = 28.661612 * myVar[threadIdx.x];
	myVar[4] = 2.114233 * myVar[threadIdx.x];
	myVar[0] = 41.587771 * myVar[threadIdx.x];
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
	var_16_10[10] = myVar[10];
	var_16_11[11] = myVar[11];
	var_16_12[12] = myVar[12];
	var_16_13[13] = myVar[13];
	var_16_14[14] = myVar[14];
	var_16_15[15] = myVar[15];
	var_16_16[16] = myVar[16];
	var_16_17[17] = myVar[17];
	var_16_18[18] = myVar[18];
	var_16_19[19] = myVar[19];
	
}

__global__ void kernel_17(float * var_17_0, float * var_17_1, float * var_17_2, float * var_17_3, float * var_17_4, float * var_17_5, float * var_17_6, float * var_17_7, float * var_17_8, float * var_17_9, float * var_17_10, float * var_17_11, float * var_17_12, float * var_17_13, float * var_17_14, float * var_17_15, float * var_17_16, float * var_17_17, float * var_17_18, float * var_17_19) {
	__shared__ float myVar[1024];
	myVar[5] = 42.657163 * myVar[threadIdx.x];
	myVar[7] = 46.068280 * myVar[threadIdx.x];
	myVar[4] = 9.105916 * myVar[threadIdx.x];
	myVar[5] = 30.973747 * myVar[threadIdx.x];
	myVar[2] = 17.915047 * myVar[threadIdx.x];
	myVar[7] = 26.762380 * myVar[threadIdx.x];
	myVar[4] = 33.421623 * myVar[threadIdx.x];
	myVar[1] = 44.503851 * myVar[threadIdx.x];
	myVar[5] = 25.264571 * myVar[threadIdx.x];
	myVar[6] = 33.079366 * myVar[threadIdx.x];
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
	var_17_10[10] = myVar[10];
	var_17_11[11] = myVar[11];
	var_17_12[12] = myVar[12];
	var_17_13[13] = myVar[13];
	var_17_14[14] = myVar[14];
	var_17_15[15] = myVar[15];
	var_17_16[16] = myVar[16];
	var_17_17[17] = myVar[17];
	var_17_18[18] = myVar[18];
	var_17_19[19] = myVar[19];
	
}

__global__ void kernel_18(float * var_18_0, float * var_18_1, float * var_18_2, float * var_18_3, float * var_18_4, float * var_18_5, float * var_18_6, float * var_18_7, float * var_18_8, float * var_18_9, float * var_18_10, float * var_18_11, float * var_18_12, float * var_18_13, float * var_18_14, float * var_18_15, float * var_18_16, float * var_18_17, float * var_18_18, float * var_18_19) {
	__shared__ float myVar[1024];
	myVar[7] = 47.140548 * myVar[threadIdx.x];
	myVar[0] = 46.028238 * myVar[threadIdx.x];
	myVar[7] = 6.932360 * myVar[threadIdx.x];
	myVar[8] = 29.252615 * myVar[threadIdx.x];
	myVar[6] = 8.795276 * myVar[threadIdx.x];
	myVar[1] = 25.034969 * myVar[threadIdx.x];
	myVar[0] = 33.216461 * myVar[threadIdx.x];
	myVar[0] = 11.724631 * myVar[threadIdx.x];
	myVar[0] = 2.454614 * myVar[threadIdx.x];
	myVar[4] = 6.795019 * myVar[threadIdx.x];
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
	var_18_10[10] = myVar[10];
	var_18_11[11] = myVar[11];
	var_18_12[12] = myVar[12];
	var_18_13[13] = myVar[13];
	var_18_14[14] = myVar[14];
	var_18_15[15] = myVar[15];
	var_18_16[16] = myVar[16];
	var_18_17[17] = myVar[17];
	var_18_18[18] = myVar[18];
	var_18_19[19] = myVar[19];
	
}

__global__ void kernel_19(float * var_19_0, float * var_19_1, float * var_19_2, float * var_19_3, float * var_19_4, float * var_19_5, float * var_19_6, float * var_19_7, float * var_19_8, float * var_19_9, float * var_19_10, float * var_19_11, float * var_19_12, float * var_19_13, float * var_19_14, float * var_19_15, float * var_19_16, float * var_19_17, float * var_19_18, float * var_19_19) {
	__shared__ float myVar[1024];
	myVar[5] = 44.514315 * myVar[threadIdx.x];
	myVar[5] = 15.645175 * myVar[threadIdx.x];
	myVar[5] = 31.812628 * myVar[threadIdx.x];
	myVar[0] = 5.528910 * myVar[threadIdx.x];
	myVar[5] = 40.822349 * myVar[threadIdx.x];
	myVar[9] = 19.084764 * myVar[threadIdx.x];
	myVar[4] = 22.711739 * myVar[threadIdx.x];
	myVar[0] = 12.284228 * myVar[threadIdx.x];
	myVar[6] = 8.482777 * myVar[threadIdx.x];
	myVar[7] = 40.246930 * myVar[threadIdx.x];
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
	var_19_10[10] = myVar[10];
	var_19_11[11] = myVar[11];
	var_19_12[12] = myVar[12];
	var_19_13[13] = myVar[13];
	var_19_14[14] = myVar[14];
	var_19_15[15] = myVar[15];
	var_19_16[16] = myVar[16];
	var_19_17[17] = myVar[17];
	var_19_18[18] = myVar[18];
	var_19_19[19] = myVar[19];
	
}

__global__ void kernel_20(float * var_20_0, float * var_20_1, float * var_20_2, float * var_20_3, float * var_20_4, float * var_20_5, float * var_20_6, float * var_20_7, float * var_20_8, float * var_20_9, float * var_20_10, float * var_20_11, float * var_20_12, float * var_20_13, float * var_20_14, float * var_20_15, float * var_20_16, float * var_20_17, float * var_20_18, float * var_20_19) {
	__shared__ float myVar[1024];
	myVar[3] = 8.920762 * myVar[threadIdx.x];
	myVar[7] = 37.353696 * myVar[threadIdx.x];
	myVar[3] = 32.099913 * myVar[threadIdx.x];
	myVar[8] = 24.096614 * myVar[threadIdx.x];
	myVar[9] = 14.495044 * myVar[threadIdx.x];
	myVar[6] = 35.974989 * myVar[threadIdx.x];
	myVar[7] = 21.908123 * myVar[threadIdx.x];
	myVar[0] = 41.261727 * myVar[threadIdx.x];
	myVar[4] = 21.221434 * myVar[threadIdx.x];
	myVar[5] = 0.046441 * myVar[threadIdx.x];
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
	var_20_10[10] = myVar[10];
	var_20_11[11] = myVar[11];
	var_20_12[12] = myVar[12];
	var_20_13[13] = myVar[13];
	var_20_14[14] = myVar[14];
	var_20_15[15] = myVar[15];
	var_20_16[16] = myVar[16];
	var_20_17[17] = myVar[17];
	var_20_18[18] = myVar[18];
	var_20_19[19] = myVar[19];
	
}

__global__ void kernel_21(float * var_21_0, float * var_21_1, float * var_21_2, float * var_21_3, float * var_21_4, float * var_21_5, float * var_21_6, float * var_21_7, float * var_21_8, float * var_21_9, float * var_21_10, float * var_21_11, float * var_21_12, float * var_21_13, float * var_21_14, float * var_21_15, float * var_21_16, float * var_21_17, float * var_21_18, float * var_21_19) {
	__shared__ float myVar[1024];
	myVar[8] = 22.112567 * myVar[threadIdx.x];
	myVar[3] = 27.653067 * myVar[threadIdx.x];
	myVar[6] = 16.924127 * myVar[threadIdx.x];
	myVar[1] = 33.412665 * myVar[threadIdx.x];
	myVar[7] = 2.221997 * myVar[threadIdx.x];
	myVar[7] = 45.256766 * myVar[threadIdx.x];
	myVar[6] = 37.572069 * myVar[threadIdx.x];
	myVar[6] = 25.978406 * myVar[threadIdx.x];
	myVar[5] = 29.277489 * myVar[threadIdx.x];
	myVar[0] = 46.767500 * myVar[threadIdx.x];
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
	var_21_10[10] = myVar[10];
	var_21_11[11] = myVar[11];
	var_21_12[12] = myVar[12];
	var_21_13[13] = myVar[13];
	var_21_14[14] = myVar[14];
	var_21_15[15] = myVar[15];
	var_21_16[16] = myVar[16];
	var_21_17[17] = myVar[17];
	var_21_18[18] = myVar[18];
	var_21_19[19] = myVar[19];
	
}

__global__ void kernel_22(float * var_22_0, float * var_22_1, float * var_22_2, float * var_22_3, float * var_22_4, float * var_22_5, float * var_22_6, float * var_22_7, float * var_22_8, float * var_22_9, float * var_22_10, float * var_22_11, float * var_22_12, float * var_22_13, float * var_22_14, float * var_22_15, float * var_22_16, float * var_22_17, float * var_22_18, float * var_22_19) {
	__shared__ float myVar[1024];
	myVar[4] = 31.285070 * myVar[threadIdx.x];
	myVar[7] = 0.863628 * myVar[threadIdx.x];
	myVar[8] = 25.720817 * myVar[threadIdx.x];
	myVar[0] = 12.004407 * myVar[threadIdx.x];
	myVar[0] = 45.033315 * myVar[threadIdx.x];
	myVar[8] = 40.415036 * myVar[threadIdx.x];
	myVar[4] = 25.509417 * myVar[threadIdx.x];
	myVar[6] = 34.767809 * myVar[threadIdx.x];
	myVar[0] = 12.637889 * myVar[threadIdx.x];
	myVar[6] = 10.133237 * myVar[threadIdx.x];
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
	var_22_10[10] = myVar[10];
	var_22_11[11] = myVar[11];
	var_22_12[12] = myVar[12];
	var_22_13[13] = myVar[13];
	var_22_14[14] = myVar[14];
	var_22_15[15] = myVar[15];
	var_22_16[16] = myVar[16];
	var_22_17[17] = myVar[17];
	var_22_18[18] = myVar[18];
	var_22_19[19] = myVar[19];
	
}

__global__ void kernel_23(float * var_23_0, float * var_23_1, float * var_23_2, float * var_23_3, float * var_23_4, float * var_23_5, float * var_23_6, float * var_23_7, float * var_23_8, float * var_23_9, float * var_23_10, float * var_23_11, float * var_23_12, float * var_23_13, float * var_23_14, float * var_23_15, float * var_23_16, float * var_23_17, float * var_23_18, float * var_23_19) {
	__shared__ float myVar[1024];
	myVar[5] = 21.524582 * myVar[threadIdx.x];
	myVar[6] = 29.706542 * myVar[threadIdx.x];
	myVar[6] = 31.447616 * myVar[threadIdx.x];
	myVar[7] = 32.046018 * myVar[threadIdx.x];
	myVar[1] = 31.068229 * myVar[threadIdx.x];
	myVar[7] = 5.766474 * myVar[threadIdx.x];
	myVar[7] = 24.443588 * myVar[threadIdx.x];
	myVar[3] = 40.997058 * myVar[threadIdx.x];
	myVar[0] = 22.817930 * myVar[threadIdx.x];
	myVar[1] = 8.522397 * myVar[threadIdx.x];
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
	var_23_10[10] = myVar[10];
	var_23_11[11] = myVar[11];
	var_23_12[12] = myVar[12];
	var_23_13[13] = myVar[13];
	var_23_14[14] = myVar[14];
	var_23_15[15] = myVar[15];
	var_23_16[16] = myVar[16];
	var_23_17[17] = myVar[17];
	var_23_18[18] = myVar[18];
	var_23_19[19] = myVar[19];
	
}

__global__ void kernel_24(float * var_24_0, float * var_24_1, float * var_24_2, float * var_24_3, float * var_24_4, float * var_24_5, float * var_24_6, float * var_24_7, float * var_24_8, float * var_24_9, float * var_24_10, float * var_24_11, float * var_24_12, float * var_24_13, float * var_24_14, float * var_24_15, float * var_24_16, float * var_24_17, float * var_24_18, float * var_24_19) {
	__shared__ float myVar[1024];
	myVar[2] = 17.014051 * myVar[threadIdx.x];
	myVar[0] = 3.181015 * myVar[threadIdx.x];
	myVar[3] = 29.333601 * myVar[threadIdx.x];
	myVar[1] = 39.439184 * myVar[threadIdx.x];
	myVar[7] = 25.280476 * myVar[threadIdx.x];
	myVar[3] = 19.974617 * myVar[threadIdx.x];
	myVar[0] = 38.258641 * myVar[threadIdx.x];
	myVar[2] = 10.066041 * myVar[threadIdx.x];
	myVar[3] = 31.378324 * myVar[threadIdx.x];
	myVar[3] = 16.652854 * myVar[threadIdx.x];
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
	var_24_10[10] = myVar[10];
	var_24_11[11] = myVar[11];
	var_24_12[12] = myVar[12];
	var_24_13[13] = myVar[13];
	var_24_14[14] = myVar[14];
	var_24_15[15] = myVar[15];
	var_24_16[16] = myVar[16];
	var_24_17[17] = myVar[17];
	var_24_18[18] = myVar[18];
	var_24_19[19] = myVar[19];
	
}

__global__ void kernel_25(float * var_25_0, float * var_25_1, float * var_25_2, float * var_25_3, float * var_25_4, float * var_25_5, float * var_25_6, float * var_25_7, float * var_25_8, float * var_25_9, float * var_25_10, float * var_25_11, float * var_25_12, float * var_25_13, float * var_25_14, float * var_25_15, float * var_25_16, float * var_25_17, float * var_25_18, float * var_25_19) {
	__shared__ float myVar[1024];
	myVar[8] = 7.517236 * myVar[threadIdx.x];
	myVar[6] = 28.264222 * myVar[threadIdx.x];
	myVar[8] = 4.411520 * myVar[threadIdx.x];
	myVar[1] = 7.778072 * myVar[threadIdx.x];
	myVar[5] = 32.653238 * myVar[threadIdx.x];
	myVar[1] = 15.025972 * myVar[threadIdx.x];
	myVar[3] = 27.817416 * myVar[threadIdx.x];
	myVar[2] = 30.711862 * myVar[threadIdx.x];
	myVar[9] = 34.396261 * myVar[threadIdx.x];
	myVar[7] = 44.214172 * myVar[threadIdx.x];
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
	var_25_10[10] = myVar[10];
	var_25_11[11] = myVar[11];
	var_25_12[12] = myVar[12];
	var_25_13[13] = myVar[13];
	var_25_14[14] = myVar[14];
	var_25_15[15] = myVar[15];
	var_25_16[16] = myVar[16];
	var_25_17[17] = myVar[17];
	var_25_18[18] = myVar[18];
	var_25_19[19] = myVar[19];
	
}

__global__ void kernel_26(float * var_26_0, float * var_26_1, float * var_26_2, float * var_26_3, float * var_26_4, float * var_26_5, float * var_26_6, float * var_26_7, float * var_26_8, float * var_26_9, float * var_26_10, float * var_26_11, float * var_26_12, float * var_26_13, float * var_26_14, float * var_26_15, float * var_26_16, float * var_26_17, float * var_26_18, float * var_26_19) {
	__shared__ float myVar[1024];
	myVar[4] = 48.641031 * myVar[threadIdx.x];
	myVar[9] = 43.952846 * myVar[threadIdx.x];
	myVar[8] = 26.538523 * myVar[threadIdx.x];
	myVar[2] = 8.435853 * myVar[threadIdx.x];
	myVar[1] = 36.593866 * myVar[threadIdx.x];
	myVar[5] = 17.569830 * myVar[threadIdx.x];
	myVar[6] = 42.516282 * myVar[threadIdx.x];
	myVar[4] = 47.853700 * myVar[threadIdx.x];
	myVar[5] = 41.775186 * myVar[threadIdx.x];
	myVar[4] = 29.159559 * myVar[threadIdx.x];
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
	var_26_10[10] = myVar[10];
	var_26_11[11] = myVar[11];
	var_26_12[12] = myVar[12];
	var_26_13[13] = myVar[13];
	var_26_14[14] = myVar[14];
	var_26_15[15] = myVar[15];
	var_26_16[16] = myVar[16];
	var_26_17[17] = myVar[17];
	var_26_18[18] = myVar[18];
	var_26_19[19] = myVar[19];
	
}

__global__ void kernel_27(float * var_27_0, float * var_27_1, float * var_27_2, float * var_27_3, float * var_27_4, float * var_27_5, float * var_27_6, float * var_27_7, float * var_27_8, float * var_27_9, float * var_27_10, float * var_27_11, float * var_27_12, float * var_27_13, float * var_27_14, float * var_27_15, float * var_27_16, float * var_27_17, float * var_27_18, float * var_27_19) {
	__shared__ float myVar[1024];
	myVar[9] = 44.593467 * myVar[threadIdx.x];
	myVar[5] = 4.774136 * myVar[threadIdx.x];
	myVar[5] = 0.608903 * myVar[threadIdx.x];
	myVar[5] = 27.921276 * myVar[threadIdx.x];
	myVar[6] = 23.472995 * myVar[threadIdx.x];
	myVar[4] = 45.549882 * myVar[threadIdx.x];
	myVar[8] = 30.662462 * myVar[threadIdx.x];
	myVar[3] = 13.045959 * myVar[threadIdx.x];
	myVar[2] = 45.326496 * myVar[threadIdx.x];
	myVar[2] = 30.163615 * myVar[threadIdx.x];
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
	var_27_10[10] = myVar[10];
	var_27_11[11] = myVar[11];
	var_27_12[12] = myVar[12];
	var_27_13[13] = myVar[13];
	var_27_14[14] = myVar[14];
	var_27_15[15] = myVar[15];
	var_27_16[16] = myVar[16];
	var_27_17[17] = myVar[17];
	var_27_18[18] = myVar[18];
	var_27_19[19] = myVar[19];
	
}

__global__ void kernel_28(float * var_28_0, float * var_28_1, float * var_28_2, float * var_28_3, float * var_28_4, float * var_28_5, float * var_28_6, float * var_28_7, float * var_28_8, float * var_28_9, float * var_28_10, float * var_28_11, float * var_28_12, float * var_28_13, float * var_28_14, float * var_28_15, float * var_28_16, float * var_28_17, float * var_28_18, float * var_28_19) {
	__shared__ float myVar[1024];
	myVar[4] = 21.657435 * myVar[threadIdx.x];
	myVar[9] = 29.884629 * myVar[threadIdx.x];
	myVar[8] = 21.673294 * myVar[threadIdx.x];
	myVar[6] = 3.731673 * myVar[threadIdx.x];
	myVar[2] = 22.881731 * myVar[threadIdx.x];
	myVar[9] = 19.223812 * myVar[threadIdx.x];
	myVar[9] = 36.169111 * myVar[threadIdx.x];
	myVar[8] = 34.115668 * myVar[threadIdx.x];
	myVar[3] = 4.904825 * myVar[threadIdx.x];
	myVar[2] = 0.995835 * myVar[threadIdx.x];
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
	var_28_10[10] = myVar[10];
	var_28_11[11] = myVar[11];
	var_28_12[12] = myVar[12];
	var_28_13[13] = myVar[13];
	var_28_14[14] = myVar[14];
	var_28_15[15] = myVar[15];
	var_28_16[16] = myVar[16];
	var_28_17[17] = myVar[17];
	var_28_18[18] = myVar[18];
	var_28_19[19] = myVar[19];
	
}

__global__ void kernel_29(float * var_29_0, float * var_29_1, float * var_29_2, float * var_29_3, float * var_29_4, float * var_29_5, float * var_29_6, float * var_29_7, float * var_29_8, float * var_29_9, float * var_29_10, float * var_29_11, float * var_29_12, float * var_29_13, float * var_29_14, float * var_29_15, float * var_29_16, float * var_29_17, float * var_29_18, float * var_29_19) {
	__shared__ float myVar[1024];
	myVar[7] = 25.151348 * myVar[threadIdx.x];
	myVar[3] = 5.073383 * myVar[threadIdx.x];
	myVar[2] = 49.057405 * myVar[threadIdx.x];
	myVar[5] = 10.375361 * myVar[threadIdx.x];
	myVar[5] = 34.815777 * myVar[threadIdx.x];
	myVar[6] = 19.558450 * myVar[threadIdx.x];
	myVar[4] = 23.350865 * myVar[threadIdx.x];
	myVar[9] = 5.091217 * myVar[threadIdx.x];
	myVar[0] = 22.062347 * myVar[threadIdx.x];
	myVar[7] = 39.179353 * myVar[threadIdx.x];
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
	var_29_10[10] = myVar[10];
	var_29_11[11] = myVar[11];
	var_29_12[12] = myVar[12];
	var_29_13[13] = myVar[13];
	var_29_14[14] = myVar[14];
	var_29_15[15] = myVar[15];
	var_29_16[16] = myVar[16];
	var_29_17[17] = myVar[17];
	var_29_18[18] = myVar[18];
	var_29_19[19] = myVar[19];
	
}

__global__ void kernel_30(float * var_30_0, float * var_30_1, float * var_30_2, float * var_30_3, float * var_30_4, float * var_30_5, float * var_30_6, float * var_30_7, float * var_30_8, float * var_30_9, float * var_30_10, float * var_30_11, float * var_30_12, float * var_30_13, float * var_30_14, float * var_30_15, float * var_30_16, float * var_30_17, float * var_30_18, float * var_30_19) {
	__shared__ float myVar[1024];
	myVar[5] = 36.046011 * myVar[threadIdx.x];
	myVar[3] = 14.885873 * myVar[threadIdx.x];
	myVar[7] = 38.248691 * myVar[threadIdx.x];
	myVar[1] = 43.093572 * myVar[threadIdx.x];
	myVar[5] = 19.913632 * myVar[threadIdx.x];
	myVar[6] = 6.243649 * myVar[threadIdx.x];
	myVar[7] = 23.822577 * myVar[threadIdx.x];
	myVar[7] = 32.748343 * myVar[threadIdx.x];
	myVar[1] = 0.240827 * myVar[threadIdx.x];
	myVar[2] = 12.217483 * myVar[threadIdx.x];
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
	var_30_10[10] = myVar[10];
	var_30_11[11] = myVar[11];
	var_30_12[12] = myVar[12];
	var_30_13[13] = myVar[13];
	var_30_14[14] = myVar[14];
	var_30_15[15] = myVar[15];
	var_30_16[16] = myVar[16];
	var_30_17[17] = myVar[17];
	var_30_18[18] = myVar[18];
	var_30_19[19] = myVar[19];
	
}

__global__ void kernel_31(float * var_31_0, float * var_31_1, float * var_31_2, float * var_31_3, float * var_31_4, float * var_31_5, float * var_31_6, float * var_31_7, float * var_31_8, float * var_31_9, float * var_31_10, float * var_31_11, float * var_31_12, float * var_31_13, float * var_31_14, float * var_31_15, float * var_31_16, float * var_31_17, float * var_31_18, float * var_31_19) {
	__shared__ float myVar[1024];
	myVar[1] = 5.874735 * myVar[threadIdx.x];
	myVar[3] = 37.347456 * myVar[threadIdx.x];
	myVar[2] = 37.747604 * myVar[threadIdx.x];
	myVar[4] = 31.348833 * myVar[threadIdx.x];
	myVar[2] = 34.267739 * myVar[threadIdx.x];
	myVar[7] = 28.779658 * myVar[threadIdx.x];
	myVar[7] = 38.259609 * myVar[threadIdx.x];
	myVar[4] = 40.641591 * myVar[threadIdx.x];
	myVar[8] = 25.630688 * myVar[threadIdx.x];
	myVar[2] = 8.221034 * myVar[threadIdx.x];
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
	var_31_10[10] = myVar[10];
	var_31_11[11] = myVar[11];
	var_31_12[12] = myVar[12];
	var_31_13[13] = myVar[13];
	var_31_14[14] = myVar[14];
	var_31_15[15] = myVar[15];
	var_31_16[16] = myVar[16];
	var_31_17[17] = myVar[17];
	var_31_18[18] = myVar[18];
	var_31_19[19] = myVar[19];
	
}

__global__ void kernel_32(float * var_32_0, float * var_32_1, float * var_32_2, float * var_32_3, float * var_32_4, float * var_32_5, float * var_32_6, float * var_32_7, float * var_32_8, float * var_32_9, float * var_32_10, float * var_32_11, float * var_32_12, float * var_32_13, float * var_32_14, float * var_32_15, float * var_32_16, float * var_32_17, float * var_32_18, float * var_32_19) {
	__shared__ float myVar[1024];
	myVar[3] = 37.831150 * myVar[threadIdx.x];
	myVar[5] = 28.793004 * myVar[threadIdx.x];
	myVar[4] = 19.871804 * myVar[threadIdx.x];
	myVar[2] = 39.019470 * myVar[threadIdx.x];
	myVar[0] = 26.222847 * myVar[threadIdx.x];
	myVar[2] = 12.296851 * myVar[threadIdx.x];
	myVar[2] = 32.653467 * myVar[threadIdx.x];
	myVar[6] = 21.242219 * myVar[threadIdx.x];
	myVar[3] = 47.590289 * myVar[threadIdx.x];
	myVar[3] = 39.070860 * myVar[threadIdx.x];
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
	var_32_10[10] = myVar[10];
	var_32_11[11] = myVar[11];
	var_32_12[12] = myVar[12];
	var_32_13[13] = myVar[13];
	var_32_14[14] = myVar[14];
	var_32_15[15] = myVar[15];
	var_32_16[16] = myVar[16];
	var_32_17[17] = myVar[17];
	var_32_18[18] = myVar[18];
	var_32_19[19] = myVar[19];
	
}

__global__ void kernel_33(float * var_33_0, float * var_33_1, float * var_33_2, float * var_33_3, float * var_33_4, float * var_33_5, float * var_33_6, float * var_33_7, float * var_33_8, float * var_33_9, float * var_33_10, float * var_33_11, float * var_33_12, float * var_33_13, float * var_33_14, float * var_33_15, float * var_33_16, float * var_33_17, float * var_33_18, float * var_33_19) {
	__shared__ float myVar[1024];
	myVar[5] = 38.030299 * myVar[threadIdx.x];
	myVar[5] = 44.122182 * myVar[threadIdx.x];
	myVar[3] = 34.982739 * myVar[threadIdx.x];
	myVar[7] = 31.233982 * myVar[threadIdx.x];
	myVar[9] = 41.721021 * myVar[threadIdx.x];
	myVar[4] = 46.965980 * myVar[threadIdx.x];
	myVar[4] = 35.483172 * myVar[threadIdx.x];
	myVar[0] = 20.314613 * myVar[threadIdx.x];
	myVar[0] = 23.829995 * myVar[threadIdx.x];
	myVar[0] = 34.938981 * myVar[threadIdx.x];
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
	var_33_10[10] = myVar[10];
	var_33_11[11] = myVar[11];
	var_33_12[12] = myVar[12];
	var_33_13[13] = myVar[13];
	var_33_14[14] = myVar[14];
	var_33_15[15] = myVar[15];
	var_33_16[16] = myVar[16];
	var_33_17[17] = myVar[17];
	var_33_18[18] = myVar[18];
	var_33_19[19] = myVar[19];
	
}

__global__ void kernel_34(float * var_34_0, float * var_34_1, float * var_34_2, float * var_34_3, float * var_34_4, float * var_34_5, float * var_34_6, float * var_34_7, float * var_34_8, float * var_34_9, float * var_34_10, float * var_34_11, float * var_34_12, float * var_34_13, float * var_34_14, float * var_34_15, float * var_34_16, float * var_34_17, float * var_34_18, float * var_34_19) {
	__shared__ float myVar[1024];
	myVar[1] = 4.848555 * myVar[threadIdx.x];
	myVar[9] = 45.594005 * myVar[threadIdx.x];
	myVar[4] = 5.275042 * myVar[threadIdx.x];
	myVar[6] = 47.467082 * myVar[threadIdx.x];
	myVar[0] = 43.599883 * myVar[threadIdx.x];
	myVar[0] = 22.543226 * myVar[threadIdx.x];
	myVar[3] = 10.500562 * myVar[threadIdx.x];
	myVar[9] = 7.076448 * myVar[threadIdx.x];
	myVar[6] = 37.155668 * myVar[threadIdx.x];
	myVar[6] = 39.351688 * myVar[threadIdx.x];
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
	var_34_10[10] = myVar[10];
	var_34_11[11] = myVar[11];
	var_34_12[12] = myVar[12];
	var_34_13[13] = myVar[13];
	var_34_14[14] = myVar[14];
	var_34_15[15] = myVar[15];
	var_34_16[16] = myVar[16];
	var_34_17[17] = myVar[17];
	var_34_18[18] = myVar[18];
	var_34_19[19] = myVar[19];
	
}

__global__ void kernel_35(float * var_35_0, float * var_35_1, float * var_35_2, float * var_35_3, float * var_35_4, float * var_35_5, float * var_35_6, float * var_35_7, float * var_35_8, float * var_35_9, float * var_35_10, float * var_35_11, float * var_35_12, float * var_35_13, float * var_35_14, float * var_35_15, float * var_35_16, float * var_35_17, float * var_35_18, float * var_35_19) {
	__shared__ float myVar[1024];
	myVar[2] = 20.230487 * myVar[threadIdx.x];
	myVar[5] = 22.713707 * myVar[threadIdx.x];
	myVar[7] = 35.011226 * myVar[threadIdx.x];
	myVar[8] = 4.429307 * myVar[threadIdx.x];
	myVar[4] = 32.732229 * myVar[threadIdx.x];
	myVar[7] = 43.417915 * myVar[threadIdx.x];
	myVar[7] = 8.217305 * myVar[threadIdx.x];
	myVar[1] = 39.014612 * myVar[threadIdx.x];
	myVar[0] = 14.273283 * myVar[threadIdx.x];
	myVar[9] = 11.301961 * myVar[threadIdx.x];
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
	var_35_10[10] = myVar[10];
	var_35_11[11] = myVar[11];
	var_35_12[12] = myVar[12];
	var_35_13[13] = myVar[13];
	var_35_14[14] = myVar[14];
	var_35_15[15] = myVar[15];
	var_35_16[16] = myVar[16];
	var_35_17[17] = myVar[17];
	var_35_18[18] = myVar[18];
	var_35_19[19] = myVar[19];
	
}

__global__ void kernel_36(float * var_36_0, float * var_36_1, float * var_36_2, float * var_36_3, float * var_36_4, float * var_36_5, float * var_36_6, float * var_36_7, float * var_36_8, float * var_36_9, float * var_36_10, float * var_36_11, float * var_36_12, float * var_36_13, float * var_36_14, float * var_36_15, float * var_36_16, float * var_36_17, float * var_36_18, float * var_36_19) {
	__shared__ float myVar[1024];
	myVar[5] = 45.179389 * myVar[threadIdx.x];
	myVar[2] = 20.411022 * myVar[threadIdx.x];
	myVar[3] = 10.367868 * myVar[threadIdx.x];
	myVar[5] = 41.563758 * myVar[threadIdx.x];
	myVar[0] = 8.981381 * myVar[threadIdx.x];
	myVar[1] = 14.762939 * myVar[threadIdx.x];
	myVar[9] = 31.785718 * myVar[threadIdx.x];
	myVar[9] = 12.968729 * myVar[threadIdx.x];
	myVar[2] = 0.609264 * myVar[threadIdx.x];
	myVar[6] = 44.362367 * myVar[threadIdx.x];
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
	var_36_10[10] = myVar[10];
	var_36_11[11] = myVar[11];
	var_36_12[12] = myVar[12];
	var_36_13[13] = myVar[13];
	var_36_14[14] = myVar[14];
	var_36_15[15] = myVar[15];
	var_36_16[16] = myVar[16];
	var_36_17[17] = myVar[17];
	var_36_18[18] = myVar[18];
	var_36_19[19] = myVar[19];
	
}

__global__ void kernel_37(float * var_37_0, float * var_37_1, float * var_37_2, float * var_37_3, float * var_37_4, float * var_37_5, float * var_37_6, float * var_37_7, float * var_37_8, float * var_37_9, float * var_37_10, float * var_37_11, float * var_37_12, float * var_37_13, float * var_37_14, float * var_37_15, float * var_37_16, float * var_37_17, float * var_37_18, float * var_37_19) {
	__shared__ float myVar[1024];
	myVar[3] = 35.093701 * myVar[threadIdx.x];
	myVar[2] = 42.819633 * myVar[threadIdx.x];
	myVar[7] = 14.836877 * myVar[threadIdx.x];
	myVar[8] = 25.653325 * myVar[threadIdx.x];
	myVar[4] = 26.962135 * myVar[threadIdx.x];
	myVar[5] = 39.779576 * myVar[threadIdx.x];
	myVar[1] = 39.045629 * myVar[threadIdx.x];
	myVar[1] = 12.076513 * myVar[threadIdx.x];
	myVar[3] = 49.070441 * myVar[threadIdx.x];
	myVar[5] = 44.290816 * myVar[threadIdx.x];
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
	var_37_10[10] = myVar[10];
	var_37_11[11] = myVar[11];
	var_37_12[12] = myVar[12];
	var_37_13[13] = myVar[13];
	var_37_14[14] = myVar[14];
	var_37_15[15] = myVar[15];
	var_37_16[16] = myVar[16];
	var_37_17[17] = myVar[17];
	var_37_18[18] = myVar[18];
	var_37_19[19] = myVar[19];
	
}

__global__ void kernel_38(float * var_38_0, float * var_38_1, float * var_38_2, float * var_38_3, float * var_38_4, float * var_38_5, float * var_38_6, float * var_38_7, float * var_38_8, float * var_38_9, float * var_38_10, float * var_38_11, float * var_38_12, float * var_38_13, float * var_38_14, float * var_38_15, float * var_38_16, float * var_38_17, float * var_38_18, float * var_38_19) {
	__shared__ float myVar[1024];
	myVar[8] = 16.950942 * myVar[threadIdx.x];
	myVar[6] = 16.723500 * myVar[threadIdx.x];
	myVar[1] = 0.501504 * myVar[threadIdx.x];
	myVar[8] = 15.969000 * myVar[threadIdx.x];
	myVar[9] = 21.267720 * myVar[threadIdx.x];
	myVar[4] = 49.429729 * myVar[threadIdx.x];
	myVar[1] = 19.745463 * myVar[threadIdx.x];
	myVar[0] = 6.291363 * myVar[threadIdx.x];
	myVar[5] = 1.115146 * myVar[threadIdx.x];
	myVar[0] = 30.363812 * myVar[threadIdx.x];
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
	var_38_10[10] = myVar[10];
	var_38_11[11] = myVar[11];
	var_38_12[12] = myVar[12];
	var_38_13[13] = myVar[13];
	var_38_14[14] = myVar[14];
	var_38_15[15] = myVar[15];
	var_38_16[16] = myVar[16];
	var_38_17[17] = myVar[17];
	var_38_18[18] = myVar[18];
	var_38_19[19] = myVar[19];
	
}

__global__ void kernel_39(float * var_39_0, float * var_39_1, float * var_39_2, float * var_39_3, float * var_39_4, float * var_39_5, float * var_39_6, float * var_39_7, float * var_39_8, float * var_39_9, float * var_39_10, float * var_39_11, float * var_39_12, float * var_39_13, float * var_39_14, float * var_39_15, float * var_39_16, float * var_39_17, float * var_39_18, float * var_39_19) {
	__shared__ float myVar[1024];
	myVar[4] = 48.490515 * myVar[threadIdx.x];
	myVar[9] = 12.608231 * myVar[threadIdx.x];
	myVar[2] = 35.157848 * myVar[threadIdx.x];
	myVar[0] = 1.188302 * myVar[threadIdx.x];
	myVar[5] = 45.364279 * myVar[threadIdx.x];
	myVar[3] = 34.068832 * myVar[threadIdx.x];
	myVar[2] = 21.128017 * myVar[threadIdx.x];
	myVar[5] = 14.244563 * myVar[threadIdx.x];
	myVar[3] = 3.151376 * myVar[threadIdx.x];
	myVar[6] = 2.305583 * myVar[threadIdx.x];
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
	var_39_10[10] = myVar[10];
	var_39_11[11] = myVar[11];
	var_39_12[12] = myVar[12];
	var_39_13[13] = myVar[13];
	var_39_14[14] = myVar[14];
	var_39_15[15] = myVar[15];
	var_39_16[16] = myVar[16];
	var_39_17[17] = myVar[17];
	var_39_18[18] = myVar[18];
	var_39_19[19] = myVar[19];
	
}

__global__ void kernel_40(float * var_40_0, float * var_40_1, float * var_40_2, float * var_40_3, float * var_40_4, float * var_40_5, float * var_40_6, float * var_40_7, float * var_40_8, float * var_40_9, float * var_40_10, float * var_40_11, float * var_40_12, float * var_40_13, float * var_40_14, float * var_40_15, float * var_40_16, float * var_40_17, float * var_40_18, float * var_40_19) {
	__shared__ float myVar[1024];
	myVar[6] = 38.743765 * myVar[threadIdx.x];
	myVar[0] = 3.914831 * myVar[threadIdx.x];
	myVar[0] = 45.867457 * myVar[threadIdx.x];
	myVar[8] = 15.480244 * myVar[threadIdx.x];
	myVar[6] = 7.260518 * myVar[threadIdx.x];
	myVar[3] = 7.096993 * myVar[threadIdx.x];
	myVar[5] = 5.880275 * myVar[threadIdx.x];
	myVar[3] = 22.825388 * myVar[threadIdx.x];
	myVar[2] = 16.359728 * myVar[threadIdx.x];
	myVar[3] = 15.126936 * myVar[threadIdx.x];
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
	var_40_10[10] = myVar[10];
	var_40_11[11] = myVar[11];
	var_40_12[12] = myVar[12];
	var_40_13[13] = myVar[13];
	var_40_14[14] = myVar[14];
	var_40_15[15] = myVar[15];
	var_40_16[16] = myVar[16];
	var_40_17[17] = myVar[17];
	var_40_18[18] = myVar[18];
	var_40_19[19] = myVar[19];
	
}

__global__ void kernel_41(float * var_41_0, float * var_41_1, float * var_41_2, float * var_41_3, float * var_41_4, float * var_41_5, float * var_41_6, float * var_41_7, float * var_41_8, float * var_41_9, float * var_41_10, float * var_41_11, float * var_41_12, float * var_41_13, float * var_41_14, float * var_41_15, float * var_41_16, float * var_41_17, float * var_41_18, float * var_41_19) {
	__shared__ float myVar[1024];
	myVar[3] = 20.568324 * myVar[threadIdx.x];
	myVar[1] = 21.021676 * myVar[threadIdx.x];
	myVar[1] = 40.620338 * myVar[threadIdx.x];
	myVar[9] = 36.677789 * myVar[threadIdx.x];
	myVar[7] = 27.680906 * myVar[threadIdx.x];
	myVar[2] = 44.656317 * myVar[threadIdx.x];
	myVar[9] = 22.003000 * myVar[threadIdx.x];
	myVar[4] = 16.280718 * myVar[threadIdx.x];
	myVar[9] = 16.620876 * myVar[threadIdx.x];
	myVar[8] = 0.349996 * myVar[threadIdx.x];
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
	var_41_10[10] = myVar[10];
	var_41_11[11] = myVar[11];
	var_41_12[12] = myVar[12];
	var_41_13[13] = myVar[13];
	var_41_14[14] = myVar[14];
	var_41_15[15] = myVar[15];
	var_41_16[16] = myVar[16];
	var_41_17[17] = myVar[17];
	var_41_18[18] = myVar[18];
	var_41_19[19] = myVar[19];
	
}

__global__ void kernel_42(float * var_42_0, float * var_42_1, float * var_42_2, float * var_42_3, float * var_42_4, float * var_42_5, float * var_42_6, float * var_42_7, float * var_42_8, float * var_42_9, float * var_42_10, float * var_42_11, float * var_42_12, float * var_42_13, float * var_42_14, float * var_42_15, float * var_42_16, float * var_42_17, float * var_42_18, float * var_42_19) {
	__shared__ float myVar[1024];
	myVar[6] = 42.111438 * myVar[threadIdx.x];
	myVar[8] = 3.365143 * myVar[threadIdx.x];
	myVar[2] = 33.891024 * myVar[threadIdx.x];
	myVar[4] = 41.766899 * myVar[threadIdx.x];
	myVar[2] = 7.180711 * myVar[threadIdx.x];
	myVar[9] = 40.939054 * myVar[threadIdx.x];
	myVar[2] = 27.350946 * myVar[threadIdx.x];
	myVar[2] = 39.810918 * myVar[threadIdx.x];
	myVar[0] = 37.526751 * myVar[threadIdx.x];
	myVar[4] = 26.537539 * myVar[threadIdx.x];
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
	var_42_10[10] = myVar[10];
	var_42_11[11] = myVar[11];
	var_42_12[12] = myVar[12];
	var_42_13[13] = myVar[13];
	var_42_14[14] = myVar[14];
	var_42_15[15] = myVar[15];
	var_42_16[16] = myVar[16];
	var_42_17[17] = myVar[17];
	var_42_18[18] = myVar[18];
	var_42_19[19] = myVar[19];
	
}

__global__ void kernel_43(float * var_43_0, float * var_43_1, float * var_43_2, float * var_43_3, float * var_43_4, float * var_43_5, float * var_43_6, float * var_43_7, float * var_43_8, float * var_43_9, float * var_43_10, float * var_43_11, float * var_43_12, float * var_43_13, float * var_43_14, float * var_43_15, float * var_43_16, float * var_43_17, float * var_43_18, float * var_43_19) {
	__shared__ float myVar[1024];
	myVar[2] = 36.382057 * myVar[threadIdx.x];
	myVar[7] = 23.767897 * myVar[threadIdx.x];
	myVar[0] = 49.106766 * myVar[threadIdx.x];
	myVar[8] = 28.979968 * myVar[threadIdx.x];
	myVar[1] = 21.884905 * myVar[threadIdx.x];
	myVar[2] = 24.992381 * myVar[threadIdx.x];
	myVar[6] = 47.545519 * myVar[threadIdx.x];
	myVar[3] = 0.659092 * myVar[threadIdx.x];
	myVar[3] = 41.286335 * myVar[threadIdx.x];
	myVar[8] = 22.498220 * myVar[threadIdx.x];
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
	var_43_10[10] = myVar[10];
	var_43_11[11] = myVar[11];
	var_43_12[12] = myVar[12];
	var_43_13[13] = myVar[13];
	var_43_14[14] = myVar[14];
	var_43_15[15] = myVar[15];
	var_43_16[16] = myVar[16];
	var_43_17[17] = myVar[17];
	var_43_18[18] = myVar[18];
	var_43_19[19] = myVar[19];
	
}

__global__ void kernel_44(float * var_44_0, float * var_44_1, float * var_44_2, float * var_44_3, float * var_44_4, float * var_44_5, float * var_44_6, float * var_44_7, float * var_44_8, float * var_44_9, float * var_44_10, float * var_44_11, float * var_44_12, float * var_44_13, float * var_44_14, float * var_44_15, float * var_44_16, float * var_44_17, float * var_44_18, float * var_44_19) {
	__shared__ float myVar[1024];
	myVar[1] = 22.696796 * myVar[threadIdx.x];
	myVar[4] = 7.228871 * myVar[threadIdx.x];
	myVar[6] = 32.201492 * myVar[threadIdx.x];
	myVar[9] = 23.192476 * myVar[threadIdx.x];
	myVar[7] = 29.049439 * myVar[threadIdx.x];
	myVar[4] = 41.982081 * myVar[threadIdx.x];
	myVar[5] = 32.711130 * myVar[threadIdx.x];
	myVar[9] = 43.816981 * myVar[threadIdx.x];
	myVar[4] = 34.609433 * myVar[threadIdx.x];
	myVar[9] = 28.135683 * myVar[threadIdx.x];
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
	var_44_10[10] = myVar[10];
	var_44_11[11] = myVar[11];
	var_44_12[12] = myVar[12];
	var_44_13[13] = myVar[13];
	var_44_14[14] = myVar[14];
	var_44_15[15] = myVar[15];
	var_44_16[16] = myVar[16];
	var_44_17[17] = myVar[17];
	var_44_18[18] = myVar[18];
	var_44_19[19] = myVar[19];
	
}

__global__ void kernel_45(float * var_45_0, float * var_45_1, float * var_45_2, float * var_45_3, float * var_45_4, float * var_45_5, float * var_45_6, float * var_45_7, float * var_45_8, float * var_45_9, float * var_45_10, float * var_45_11, float * var_45_12, float * var_45_13, float * var_45_14, float * var_45_15, float * var_45_16, float * var_45_17, float * var_45_18, float * var_45_19) {
	__shared__ float myVar[1024];
	myVar[1] = 5.264738 * myVar[threadIdx.x];
	myVar[8] = 26.466403 * myVar[threadIdx.x];
	myVar[2] = 6.018990 * myVar[threadIdx.x];
	myVar[8] = 8.928184 * myVar[threadIdx.x];
	myVar[7] = 16.665097 * myVar[threadIdx.x];
	myVar[5] = 42.861306 * myVar[threadIdx.x];
	myVar[9] = 31.633341 * myVar[threadIdx.x];
	myVar[2] = 27.710102 * myVar[threadIdx.x];
	myVar[8] = 39.455016 * myVar[threadIdx.x];
	myVar[1] = 19.087801 * myVar[threadIdx.x];
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
	var_45_10[10] = myVar[10];
	var_45_11[11] = myVar[11];
	var_45_12[12] = myVar[12];
	var_45_13[13] = myVar[13];
	var_45_14[14] = myVar[14];
	var_45_15[15] = myVar[15];
	var_45_16[16] = myVar[16];
	var_45_17[17] = myVar[17];
	var_45_18[18] = myVar[18];
	var_45_19[19] = myVar[19];
	
}

__global__ void kernel_46(float * var_46_0, float * var_46_1, float * var_46_2, float * var_46_3, float * var_46_4, float * var_46_5, float * var_46_6, float * var_46_7, float * var_46_8, float * var_46_9, float * var_46_10, float * var_46_11, float * var_46_12, float * var_46_13, float * var_46_14, float * var_46_15, float * var_46_16, float * var_46_17, float * var_46_18, float * var_46_19) {
	__shared__ float myVar[1024];
	myVar[6] = 29.320805 * myVar[threadIdx.x];
	myVar[3] = 13.928934 * myVar[threadIdx.x];
	myVar[0] = 40.096413 * myVar[threadIdx.x];
	myVar[2] = 44.771034 * myVar[threadIdx.x];
	myVar[9] = 19.382410 * myVar[threadIdx.x];
	myVar[2] = 5.395163 * myVar[threadIdx.x];
	myVar[1] = 18.947185 * myVar[threadIdx.x];
	myVar[5] = 9.527862 * myVar[threadIdx.x];
	myVar[2] = 42.415643 * myVar[threadIdx.x];
	myVar[7] = 38.430310 * myVar[threadIdx.x];
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
	var_46_10[10] = myVar[10];
	var_46_11[11] = myVar[11];
	var_46_12[12] = myVar[12];
	var_46_13[13] = myVar[13];
	var_46_14[14] = myVar[14];
	var_46_15[15] = myVar[15];
	var_46_16[16] = myVar[16];
	var_46_17[17] = myVar[17];
	var_46_18[18] = myVar[18];
	var_46_19[19] = myVar[19];
	
}

__global__ void kernel_47(float * var_47_0, float * var_47_1, float * var_47_2, float * var_47_3, float * var_47_4, float * var_47_5, float * var_47_6, float * var_47_7, float * var_47_8, float * var_47_9, float * var_47_10, float * var_47_11, float * var_47_12, float * var_47_13, float * var_47_14, float * var_47_15, float * var_47_16, float * var_47_17, float * var_47_18, float * var_47_19) {
	__shared__ float myVar[1024];
	myVar[4] = 5.982213 * myVar[threadIdx.x];
	myVar[3] = 37.258891 * myVar[threadIdx.x];
	myVar[9] = 25.699217 * myVar[threadIdx.x];
	myVar[3] = 40.365154 * myVar[threadIdx.x];
	myVar[5] = 14.262341 * myVar[threadIdx.x];
	myVar[4] = 40.991364 * myVar[threadIdx.x];
	myVar[3] = 8.103409 * myVar[threadIdx.x];
	myVar[1] = 40.528052 * myVar[threadIdx.x];
	myVar[5] = 21.207895 * myVar[threadIdx.x];
	myVar[5] = 5.529244 * myVar[threadIdx.x];
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
	var_47_10[10] = myVar[10];
	var_47_11[11] = myVar[11];
	var_47_12[12] = myVar[12];
	var_47_13[13] = myVar[13];
	var_47_14[14] = myVar[14];
	var_47_15[15] = myVar[15];
	var_47_16[16] = myVar[16];
	var_47_17[17] = myVar[17];
	var_47_18[18] = myVar[18];
	var_47_19[19] = myVar[19];
	
}

__global__ void kernel_48(float * var_48_0, float * var_48_1, float * var_48_2, float * var_48_3, float * var_48_4, float * var_48_5, float * var_48_6, float * var_48_7, float * var_48_8, float * var_48_9, float * var_48_10, float * var_48_11, float * var_48_12, float * var_48_13, float * var_48_14, float * var_48_15, float * var_48_16, float * var_48_17, float * var_48_18, float * var_48_19) {
	__shared__ float myVar[1024];
	myVar[6] = 19.114624 * myVar[threadIdx.x];
	myVar[2] = 12.542276 * myVar[threadIdx.x];
	myVar[3] = 1.622599 * myVar[threadIdx.x];
	myVar[3] = 39.816754 * myVar[threadIdx.x];
	myVar[6] = 23.014758 * myVar[threadIdx.x];
	myVar[1] = 45.052447 * myVar[threadIdx.x];
	myVar[9] = 7.343654 * myVar[threadIdx.x];
	myVar[8] = 19.386823 * myVar[threadIdx.x];
	myVar[9] = 32.406551 * myVar[threadIdx.x];
	myVar[5] = 31.336219 * myVar[threadIdx.x];
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
	var_48_10[10] = myVar[10];
	var_48_11[11] = myVar[11];
	var_48_12[12] = myVar[12];
	var_48_13[13] = myVar[13];
	var_48_14[14] = myVar[14];
	var_48_15[15] = myVar[15];
	var_48_16[16] = myVar[16];
	var_48_17[17] = myVar[17];
	var_48_18[18] = myVar[18];
	var_48_19[19] = myVar[19];
	
}

__global__ void kernel_49(float * var_49_0, float * var_49_1, float * var_49_2, float * var_49_3, float * var_49_4, float * var_49_5, float * var_49_6, float * var_49_7, float * var_49_8, float * var_49_9, float * var_49_10, float * var_49_11, float * var_49_12, float * var_49_13, float * var_49_14, float * var_49_15, float * var_49_16, float * var_49_17, float * var_49_18, float * var_49_19) {
	__shared__ float myVar[1024];
	myVar[3] = 9.497627 * myVar[threadIdx.x];
	myVar[3] = 38.111004 * myVar[threadIdx.x];
	myVar[1] = 20.507056 * myVar[threadIdx.x];
	myVar[2] = 42.317821 * myVar[threadIdx.x];
	myVar[1] = 48.548658 * myVar[threadIdx.x];
	myVar[4] = 16.948474 * myVar[threadIdx.x];
	myVar[5] = 9.081909 * myVar[threadIdx.x];
	myVar[5] = 16.551171 * myVar[threadIdx.x];
	myVar[7] = 42.262653 * myVar[threadIdx.x];
	myVar[3] = 40.563470 * myVar[threadIdx.x];
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
	var_49_10[10] = myVar[10];
	var_49_11[11] = myVar[11];
	var_49_12[12] = myVar[12];
	var_49_13[13] = myVar[13];
	var_49_14[14] = myVar[14];
	var_49_15[15] = myVar[15];
	var_49_16[16] = myVar[16];
	var_49_17[17] = myVar[17];
	var_49_18[18] = myVar[18];
	var_49_19[19] = myVar[19];
	
}

__global__ void kernel_50(float * var_50_0, float * var_50_1, float * var_50_2, float * var_50_3, float * var_50_4, float * var_50_5, float * var_50_6, float * var_50_7, float * var_50_8, float * var_50_9, float * var_50_10, float * var_50_11, float * var_50_12, float * var_50_13, float * var_50_14, float * var_50_15, float * var_50_16, float * var_50_17, float * var_50_18, float * var_50_19) {
	__shared__ float myVar[1024];
	myVar[6] = 28.012459 * myVar[threadIdx.x];
	myVar[6] = 25.991875 * myVar[threadIdx.x];
	myVar[3] = 13.713698 * myVar[threadIdx.x];
	myVar[4] = 13.887336 * myVar[threadIdx.x];
	myVar[9] = 41.417406 * myVar[threadIdx.x];
	myVar[3] = 5.307227 * myVar[threadIdx.x];
	myVar[0] = 0.364875 * myVar[threadIdx.x];
	myVar[6] = 18.413486 * myVar[threadIdx.x];
	myVar[4] = 9.831031 * myVar[threadIdx.x];
	myVar[2] = 41.817142 * myVar[threadIdx.x];
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
	var_50_10[10] = myVar[10];
	var_50_11[11] = myVar[11];
	var_50_12[12] = myVar[12];
	var_50_13[13] = myVar[13];
	var_50_14[14] = myVar[14];
	var_50_15[15] = myVar[15];
	var_50_16[16] = myVar[16];
	var_50_17[17] = myVar[17];
	var_50_18[18] = myVar[18];
	var_50_19[19] = myVar[19];
	
}

__global__ void kernel_51(float * var_51_0, float * var_51_1, float * var_51_2, float * var_51_3, float * var_51_4, float * var_51_5, float * var_51_6, float * var_51_7, float * var_51_8, float * var_51_9, float * var_51_10, float * var_51_11, float * var_51_12, float * var_51_13, float * var_51_14, float * var_51_15, float * var_51_16, float * var_51_17, float * var_51_18, float * var_51_19) {
	__shared__ float myVar[1024];
	myVar[4] = 22.490023 * myVar[threadIdx.x];
	myVar[3] = 5.457193 * myVar[threadIdx.x];
	myVar[1] = 21.186693 * myVar[threadIdx.x];
	myVar[7] = 17.845219 * myVar[threadIdx.x];
	myVar[9] = 18.021808 * myVar[threadIdx.x];
	myVar[5] = 27.833284 * myVar[threadIdx.x];
	myVar[0] = 10.576937 * myVar[threadIdx.x];
	myVar[2] = 1.478394 * myVar[threadIdx.x];
	myVar[1] = 33.419164 * myVar[threadIdx.x];
	myVar[0] = 28.783970 * myVar[threadIdx.x];
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
	var_51_10[10] = myVar[10];
	var_51_11[11] = myVar[11];
	var_51_12[12] = myVar[12];
	var_51_13[13] = myVar[13];
	var_51_14[14] = myVar[14];
	var_51_15[15] = myVar[15];
	var_51_16[16] = myVar[16];
	var_51_17[17] = myVar[17];
	var_51_18[18] = myVar[18];
	var_51_19[19] = myVar[19];
	
}

__global__ void kernel_52(float * var_52_0, float * var_52_1, float * var_52_2, float * var_52_3, float * var_52_4, float * var_52_5, float * var_52_6, float * var_52_7, float * var_52_8, float * var_52_9, float * var_52_10, float * var_52_11, float * var_52_12, float * var_52_13, float * var_52_14, float * var_52_15, float * var_52_16, float * var_52_17, float * var_52_18, float * var_52_19) {
	__shared__ float myVar[1024];
	myVar[2] = 7.235504 * myVar[threadIdx.x];
	myVar[9] = 32.571032 * myVar[threadIdx.x];
	myVar[7] = 34.190828 * myVar[threadIdx.x];
	myVar[1] = 26.437176 * myVar[threadIdx.x];
	myVar[9] = 1.664443 * myVar[threadIdx.x];
	myVar[7] = 28.140066 * myVar[threadIdx.x];
	myVar[2] = 4.530189 * myVar[threadIdx.x];
	myVar[5] = 37.755731 * myVar[threadIdx.x];
	myVar[8] = 37.563695 * myVar[threadIdx.x];
	myVar[6] = 23.034860 * myVar[threadIdx.x];
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
	var_52_10[10] = myVar[10];
	var_52_11[11] = myVar[11];
	var_52_12[12] = myVar[12];
	var_52_13[13] = myVar[13];
	var_52_14[14] = myVar[14];
	var_52_15[15] = myVar[15];
	var_52_16[16] = myVar[16];
	var_52_17[17] = myVar[17];
	var_52_18[18] = myVar[18];
	var_52_19[19] = myVar[19];
	
}

__global__ void kernel_53(float * var_53_0, float * var_53_1, float * var_53_2, float * var_53_3, float * var_53_4, float * var_53_5, float * var_53_6, float * var_53_7, float * var_53_8, float * var_53_9, float * var_53_10, float * var_53_11, float * var_53_12, float * var_53_13, float * var_53_14, float * var_53_15, float * var_53_16, float * var_53_17, float * var_53_18, float * var_53_19) {
	__shared__ float myVar[1024];
	myVar[0] = 35.640143 * myVar[threadIdx.x];
	myVar[9] = 49.773588 * myVar[threadIdx.x];
	myVar[9] = 11.500338 * myVar[threadIdx.x];
	myVar[5] = 19.826921 * myVar[threadIdx.x];
	myVar[3] = 0.677061 * myVar[threadIdx.x];
	myVar[7] = 44.915955 * myVar[threadIdx.x];
	myVar[3] = 30.448106 * myVar[threadIdx.x];
	myVar[7] = 4.748795 * myVar[threadIdx.x];
	myVar[6] = 8.851894 * myVar[threadIdx.x];
	myVar[7] = 5.527967 * myVar[threadIdx.x];
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
	var_53_10[10] = myVar[10];
	var_53_11[11] = myVar[11];
	var_53_12[12] = myVar[12];
	var_53_13[13] = myVar[13];
	var_53_14[14] = myVar[14];
	var_53_15[15] = myVar[15];
	var_53_16[16] = myVar[16];
	var_53_17[17] = myVar[17];
	var_53_18[18] = myVar[18];
	var_53_19[19] = myVar[19];
	
}

__global__ void kernel_54(float * var_54_0, float * var_54_1, float * var_54_2, float * var_54_3, float * var_54_4, float * var_54_5, float * var_54_6, float * var_54_7, float * var_54_8, float * var_54_9, float * var_54_10, float * var_54_11, float * var_54_12, float * var_54_13, float * var_54_14, float * var_54_15, float * var_54_16, float * var_54_17, float * var_54_18, float * var_54_19) {
	__shared__ float myVar[1024];
	myVar[2] = 19.902653 * myVar[threadIdx.x];
	myVar[1] = 47.270573 * myVar[threadIdx.x];
	myVar[9] = 8.612166 * myVar[threadIdx.x];
	myVar[0] = 13.656687 * myVar[threadIdx.x];
	myVar[3] = 34.048088 * myVar[threadIdx.x];
	myVar[2] = 20.745672 * myVar[threadIdx.x];
	myVar[4] = 4.449649 * myVar[threadIdx.x];
	myVar[1] = 34.471773 * myVar[threadIdx.x];
	myVar[1] = 29.653638 * myVar[threadIdx.x];
	myVar[2] = 3.453407 * myVar[threadIdx.x];
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
	var_54_10[10] = myVar[10];
	var_54_11[11] = myVar[11];
	var_54_12[12] = myVar[12];
	var_54_13[13] = myVar[13];
	var_54_14[14] = myVar[14];
	var_54_15[15] = myVar[15];
	var_54_16[16] = myVar[16];
	var_54_17[17] = myVar[17];
	var_54_18[18] = myVar[18];
	var_54_19[19] = myVar[19];
	
}

__global__ void kernel_55(float * var_55_0, float * var_55_1, float * var_55_2, float * var_55_3, float * var_55_4, float * var_55_5, float * var_55_6, float * var_55_7, float * var_55_8, float * var_55_9, float * var_55_10, float * var_55_11, float * var_55_12, float * var_55_13, float * var_55_14, float * var_55_15, float * var_55_16, float * var_55_17, float * var_55_18, float * var_55_19) {
	__shared__ float myVar[1024];
	myVar[6] = 37.034285 * myVar[threadIdx.x];
	myVar[3] = 12.716498 * myVar[threadIdx.x];
	myVar[6] = 8.490564 * myVar[threadIdx.x];
	myVar[0] = 43.948535 * myVar[threadIdx.x];
	myVar[2] = 0.320381 * myVar[threadIdx.x];
	myVar[3] = 5.822891 * myVar[threadIdx.x];
	myVar[3] = 21.174635 * myVar[threadIdx.x];
	myVar[2] = 13.974690 * myVar[threadIdx.x];
	myVar[1] = 39.717704 * myVar[threadIdx.x];
	myVar[4] = 46.594515 * myVar[threadIdx.x];
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
	var_55_10[10] = myVar[10];
	var_55_11[11] = myVar[11];
	var_55_12[12] = myVar[12];
	var_55_13[13] = myVar[13];
	var_55_14[14] = myVar[14];
	var_55_15[15] = myVar[15];
	var_55_16[16] = myVar[16];
	var_55_17[17] = myVar[17];
	var_55_18[18] = myVar[18];
	var_55_19[19] = myVar[19];
	
}

__global__ void kernel_56(float * var_56_0, float * var_56_1, float * var_56_2, float * var_56_3, float * var_56_4, float * var_56_5, float * var_56_6, float * var_56_7, float * var_56_8, float * var_56_9, float * var_56_10, float * var_56_11, float * var_56_12, float * var_56_13, float * var_56_14, float * var_56_15, float * var_56_16, float * var_56_17, float * var_56_18, float * var_56_19) {
	__shared__ float myVar[1024];
	myVar[4] = 14.354722 * myVar[threadIdx.x];
	myVar[8] = 19.445647 * myVar[threadIdx.x];
	myVar[6] = 6.975940 * myVar[threadIdx.x];
	myVar[1] = 1.841344 * myVar[threadIdx.x];
	myVar[0] = 25.022314 * myVar[threadIdx.x];
	myVar[8] = 10.115960 * myVar[threadIdx.x];
	myVar[8] = 26.694995 * myVar[threadIdx.x];
	myVar[2] = 27.048145 * myVar[threadIdx.x];
	myVar[9] = 40.191317 * myVar[threadIdx.x];
	myVar[4] = 48.592553 * myVar[threadIdx.x];
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
	var_56_10[10] = myVar[10];
	var_56_11[11] = myVar[11];
	var_56_12[12] = myVar[12];
	var_56_13[13] = myVar[13];
	var_56_14[14] = myVar[14];
	var_56_15[15] = myVar[15];
	var_56_16[16] = myVar[16];
	var_56_17[17] = myVar[17];
	var_56_18[18] = myVar[18];
	var_56_19[19] = myVar[19];
	
}

__global__ void kernel_57(float * var_57_0, float * var_57_1, float * var_57_2, float * var_57_3, float * var_57_4, float * var_57_5, float * var_57_6, float * var_57_7, float * var_57_8, float * var_57_9, float * var_57_10, float * var_57_11, float * var_57_12, float * var_57_13, float * var_57_14, float * var_57_15, float * var_57_16, float * var_57_17, float * var_57_18, float * var_57_19) {
	__shared__ float myVar[1024];
	myVar[8] = 1.034066 * myVar[threadIdx.x];
	myVar[8] = 17.991719 * myVar[threadIdx.x];
	myVar[7] = 33.456486 * myVar[threadIdx.x];
	myVar[9] = 22.782415 * myVar[threadIdx.x];
	myVar[7] = 43.160783 * myVar[threadIdx.x];
	myVar[3] = 47.987453 * myVar[threadIdx.x];
	myVar[8] = 19.269042 * myVar[threadIdx.x];
	myVar[1] = 38.047578 * myVar[threadIdx.x];
	myVar[3] = 23.153168 * myVar[threadIdx.x];
	myVar[3] = 5.201573 * myVar[threadIdx.x];
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
	var_57_10[10] = myVar[10];
	var_57_11[11] = myVar[11];
	var_57_12[12] = myVar[12];
	var_57_13[13] = myVar[13];
	var_57_14[14] = myVar[14];
	var_57_15[15] = myVar[15];
	var_57_16[16] = myVar[16];
	var_57_17[17] = myVar[17];
	var_57_18[18] = myVar[18];
	var_57_19[19] = myVar[19];
	
}

__global__ void kernel_58(float * var_58_0, float * var_58_1, float * var_58_2, float * var_58_3, float * var_58_4, float * var_58_5, float * var_58_6, float * var_58_7, float * var_58_8, float * var_58_9, float * var_58_10, float * var_58_11, float * var_58_12, float * var_58_13, float * var_58_14, float * var_58_15, float * var_58_16, float * var_58_17, float * var_58_18, float * var_58_19) {
	__shared__ float myVar[1024];
	myVar[1] = 41.541838 * myVar[threadIdx.x];
	myVar[9] = 37.748709 * myVar[threadIdx.x];
	myVar[4] = 16.368320 * myVar[threadIdx.x];
	myVar[8] = 31.205332 * myVar[threadIdx.x];
	myVar[0] = 37.657988 * myVar[threadIdx.x];
	myVar[2] = 22.547574 * myVar[threadIdx.x];
	myVar[9] = 1.961046 * myVar[threadIdx.x];
	myVar[0] = 34.180845 * myVar[threadIdx.x];
	myVar[1] = 3.071738 * myVar[threadIdx.x];
	myVar[0] = 15.505802 * myVar[threadIdx.x];
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
	var_58_10[10] = myVar[10];
	var_58_11[11] = myVar[11];
	var_58_12[12] = myVar[12];
	var_58_13[13] = myVar[13];
	var_58_14[14] = myVar[14];
	var_58_15[15] = myVar[15];
	var_58_16[16] = myVar[16];
	var_58_17[17] = myVar[17];
	var_58_18[18] = myVar[18];
	var_58_19[19] = myVar[19];
	
}

__global__ void kernel_59(float * var_59_0, float * var_59_1, float * var_59_2, float * var_59_3, float * var_59_4, float * var_59_5, float * var_59_6, float * var_59_7, float * var_59_8, float * var_59_9, float * var_59_10, float * var_59_11, float * var_59_12, float * var_59_13, float * var_59_14, float * var_59_15, float * var_59_16, float * var_59_17, float * var_59_18, float * var_59_19) {
	__shared__ float myVar[1024];
	myVar[0] = 36.742748 * myVar[threadIdx.x];
	myVar[1] = 25.209210 * myVar[threadIdx.x];
	myVar[4] = 42.259580 * myVar[threadIdx.x];
	myVar[3] = 16.190616 * myVar[threadIdx.x];
	myVar[3] = 1.853530 * myVar[threadIdx.x];
	myVar[0] = 14.906392 * myVar[threadIdx.x];
	myVar[3] = 11.083243 * myVar[threadIdx.x];
	myVar[8] = 49.082613 * myVar[threadIdx.x];
	myVar[9] = 30.891738 * myVar[threadIdx.x];
	myVar[3] = 26.238603 * myVar[threadIdx.x];
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
	var_59_10[10] = myVar[10];
	var_59_11[11] = myVar[11];
	var_59_12[12] = myVar[12];
	var_59_13[13] = myVar[13];
	var_59_14[14] = myVar[14];
	var_59_15[15] = myVar[15];
	var_59_16[16] = myVar[16];
	var_59_17[17] = myVar[17];
	var_59_18[18] = myVar[18];
	var_59_19[19] = myVar[19];
	
}

__global__ void kernel_60(float * var_60_0, float * var_60_1, float * var_60_2, float * var_60_3, float * var_60_4, float * var_60_5, float * var_60_6, float * var_60_7, float * var_60_8, float * var_60_9, float * var_60_10, float * var_60_11, float * var_60_12, float * var_60_13, float * var_60_14, float * var_60_15, float * var_60_16, float * var_60_17, float * var_60_18, float * var_60_19) {
	__shared__ float myVar[1024];
	myVar[2] = 22.409327 * myVar[threadIdx.x];
	myVar[1] = 19.211186 * myVar[threadIdx.x];
	myVar[5] = 49.698355 * myVar[threadIdx.x];
	myVar[2] = 14.036316 * myVar[threadIdx.x];
	myVar[3] = 45.807214 * myVar[threadIdx.x];
	myVar[5] = 9.665548 * myVar[threadIdx.x];
	myVar[5] = 37.161092 * myVar[threadIdx.x];
	myVar[0] = 16.517638 * myVar[threadIdx.x];
	myVar[1] = 41.304760 * myVar[threadIdx.x];
	myVar[9] = 45.870091 * myVar[threadIdx.x];
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
	var_60_10[10] = myVar[10];
	var_60_11[11] = myVar[11];
	var_60_12[12] = myVar[12];
	var_60_13[13] = myVar[13];
	var_60_14[14] = myVar[14];
	var_60_15[15] = myVar[15];
	var_60_16[16] = myVar[16];
	var_60_17[17] = myVar[17];
	var_60_18[18] = myVar[18];
	var_60_19[19] = myVar[19];
	
}

__global__ void kernel_61(float * var_61_0, float * var_61_1, float * var_61_2, float * var_61_3, float * var_61_4, float * var_61_5, float * var_61_6, float * var_61_7, float * var_61_8, float * var_61_9, float * var_61_10, float * var_61_11, float * var_61_12, float * var_61_13, float * var_61_14, float * var_61_15, float * var_61_16, float * var_61_17, float * var_61_18, float * var_61_19) {
	__shared__ float myVar[1024];
	myVar[7] = 32.471986 * myVar[threadIdx.x];
	myVar[5] = 19.454519 * myVar[threadIdx.x];
	myVar[8] = 22.159774 * myVar[threadIdx.x];
	myVar[0] = 10.080446 * myVar[threadIdx.x];
	myVar[3] = 18.016182 * myVar[threadIdx.x];
	myVar[3] = 39.915484 * myVar[threadIdx.x];
	myVar[7] = 6.411692 * myVar[threadIdx.x];
	myVar[3] = 19.649969 * myVar[threadIdx.x];
	myVar[7] = 29.673918 * myVar[threadIdx.x];
	myVar[8] = 37.867687 * myVar[threadIdx.x];
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
	var_61_10[10] = myVar[10];
	var_61_11[11] = myVar[11];
	var_61_12[12] = myVar[12];
	var_61_13[13] = myVar[13];
	var_61_14[14] = myVar[14];
	var_61_15[15] = myVar[15];
	var_61_16[16] = myVar[16];
	var_61_17[17] = myVar[17];
	var_61_18[18] = myVar[18];
	var_61_19[19] = myVar[19];
	
}

__global__ void kernel_62(float * var_62_0, float * var_62_1, float * var_62_2, float * var_62_3, float * var_62_4, float * var_62_5, float * var_62_6, float * var_62_7, float * var_62_8, float * var_62_9, float * var_62_10, float * var_62_11, float * var_62_12, float * var_62_13, float * var_62_14, float * var_62_15, float * var_62_16, float * var_62_17, float * var_62_18, float * var_62_19) {
	__shared__ float myVar[1024];
	myVar[2] = 4.829668 * myVar[threadIdx.x];
	myVar[9] = 43.684248 * myVar[threadIdx.x];
	myVar[0] = 28.267032 * myVar[threadIdx.x];
	myVar[8] = 0.731295 * myVar[threadIdx.x];
	myVar[9] = 28.262118 * myVar[threadIdx.x];
	myVar[1] = 43.415830 * myVar[threadIdx.x];
	myVar[1] = 45.989398 * myVar[threadIdx.x];
	myVar[7] = 4.517703 * myVar[threadIdx.x];
	myVar[5] = 12.983002 * myVar[threadIdx.x];
	myVar[8] = 5.414934 * myVar[threadIdx.x];
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
	var_62_10[10] = myVar[10];
	var_62_11[11] = myVar[11];
	var_62_12[12] = myVar[12];
	var_62_13[13] = myVar[13];
	var_62_14[14] = myVar[14];
	var_62_15[15] = myVar[15];
	var_62_16[16] = myVar[16];
	var_62_17[17] = myVar[17];
	var_62_18[18] = myVar[18];
	var_62_19[19] = myVar[19];
	
}

__global__ void kernel_63(float * var_63_0, float * var_63_1, float * var_63_2, float * var_63_3, float * var_63_4, float * var_63_5, float * var_63_6, float * var_63_7, float * var_63_8, float * var_63_9, float * var_63_10, float * var_63_11, float * var_63_12, float * var_63_13, float * var_63_14, float * var_63_15, float * var_63_16, float * var_63_17, float * var_63_18, float * var_63_19) {
	__shared__ float myVar[1024];
	myVar[0] = 6.895080 * myVar[threadIdx.x];
	myVar[8] = 9.215553 * myVar[threadIdx.x];
	myVar[6] = 23.706782 * myVar[threadIdx.x];
	myVar[2] = 10.256461 * myVar[threadIdx.x];
	myVar[3] = 9.793091 * myVar[threadIdx.x];
	myVar[4] = 8.968549 * myVar[threadIdx.x];
	myVar[5] = 42.267950 * myVar[threadIdx.x];
	myVar[3] = 4.966480 * myVar[threadIdx.x];
	myVar[6] = 22.996148 * myVar[threadIdx.x];
	myVar[0] = 2.050984 * myVar[threadIdx.x];
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
	var_63_10[10] = myVar[10];
	var_63_11[11] = myVar[11];
	var_63_12[12] = myVar[12];
	var_63_13[13] = myVar[13];
	var_63_14[14] = myVar[14];
	var_63_15[15] = myVar[15];
	var_63_16[16] = myVar[16];
	var_63_17[17] = myVar[17];
	var_63_18[18] = myVar[18];
	var_63_19[19] = myVar[19];
	
}

__global__ void kernel_64(float * var_64_0, float * var_64_1, float * var_64_2, float * var_64_3, float * var_64_4, float * var_64_5, float * var_64_6, float * var_64_7, float * var_64_8, float * var_64_9, float * var_64_10, float * var_64_11, float * var_64_12, float * var_64_13, float * var_64_14, float * var_64_15, float * var_64_16, float * var_64_17, float * var_64_18, float * var_64_19) {
	__shared__ float myVar[1024];
	myVar[8] = 36.110596 * myVar[threadIdx.x];
	myVar[5] = 37.148806 * myVar[threadIdx.x];
	myVar[2] = 49.935301 * myVar[threadIdx.x];
	myVar[7] = 16.487061 * myVar[threadIdx.x];
	myVar[2] = 28.602171 * myVar[threadIdx.x];
	myVar[1] = 29.314083 * myVar[threadIdx.x];
	myVar[4] = 28.166662 * myVar[threadIdx.x];
	myVar[1] = 15.242463 * myVar[threadIdx.x];
	myVar[4] = 11.502013 * myVar[threadIdx.x];
	myVar[2] = 14.171394 * myVar[threadIdx.x];
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
	var_64_10[10] = myVar[10];
	var_64_11[11] = myVar[11];
	var_64_12[12] = myVar[12];
	var_64_13[13] = myVar[13];
	var_64_14[14] = myVar[14];
	var_64_15[15] = myVar[15];
	var_64_16[16] = myVar[16];
	var_64_17[17] = myVar[17];
	var_64_18[18] = myVar[18];
	var_64_19[19] = myVar[19];
	
}

__global__ void kernel_65(float * var_65_0, float * var_65_1, float * var_65_2, float * var_65_3, float * var_65_4, float * var_65_5, float * var_65_6, float * var_65_7, float * var_65_8, float * var_65_9, float * var_65_10, float * var_65_11, float * var_65_12, float * var_65_13, float * var_65_14, float * var_65_15, float * var_65_16, float * var_65_17, float * var_65_18, float * var_65_19) {
	__shared__ float myVar[1024];
	myVar[7] = 39.313026 * myVar[threadIdx.x];
	myVar[1] = 28.653257 * myVar[threadIdx.x];
	myVar[5] = 18.814101 * myVar[threadIdx.x];
	myVar[0] = 32.994610 * myVar[threadIdx.x];
	myVar[2] = 39.752569 * myVar[threadIdx.x];
	myVar[2] = 26.272723 * myVar[threadIdx.x];
	myVar[5] = 20.636778 * myVar[threadIdx.x];
	myVar[5] = 0.615425 * myVar[threadIdx.x];
	myVar[2] = 14.548679 * myVar[threadIdx.x];
	myVar[2] = 4.017999 * myVar[threadIdx.x];
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
	var_65_10[10] = myVar[10];
	var_65_11[11] = myVar[11];
	var_65_12[12] = myVar[12];
	var_65_13[13] = myVar[13];
	var_65_14[14] = myVar[14];
	var_65_15[15] = myVar[15];
	var_65_16[16] = myVar[16];
	var_65_17[17] = myVar[17];
	var_65_18[18] = myVar[18];
	var_65_19[19] = myVar[19];
	
}

__global__ void kernel_66(float * var_66_0, float * var_66_1, float * var_66_2, float * var_66_3, float * var_66_4, float * var_66_5, float * var_66_6, float * var_66_7, float * var_66_8, float * var_66_9, float * var_66_10, float * var_66_11, float * var_66_12, float * var_66_13, float * var_66_14, float * var_66_15, float * var_66_16, float * var_66_17, float * var_66_18, float * var_66_19) {
	__shared__ float myVar[1024];
	myVar[2] = 28.201326 * myVar[threadIdx.x];
	myVar[5] = 41.466212 * myVar[threadIdx.x];
	myVar[1] = 14.679026 * myVar[threadIdx.x];
	myVar[8] = 1.499039 * myVar[threadIdx.x];
	myVar[0] = 38.744664 * myVar[threadIdx.x];
	myVar[1] = 6.954847 * myVar[threadIdx.x];
	myVar[8] = 7.584151 * myVar[threadIdx.x];
	myVar[0] = 6.001016 * myVar[threadIdx.x];
	myVar[6] = 2.057322 * myVar[threadIdx.x];
	myVar[2] = 19.877629 * myVar[threadIdx.x];
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
	var_66_10[10] = myVar[10];
	var_66_11[11] = myVar[11];
	var_66_12[12] = myVar[12];
	var_66_13[13] = myVar[13];
	var_66_14[14] = myVar[14];
	var_66_15[15] = myVar[15];
	var_66_16[16] = myVar[16];
	var_66_17[17] = myVar[17];
	var_66_18[18] = myVar[18];
	var_66_19[19] = myVar[19];
	
}

__global__ void kernel_67(float * var_67_0, float * var_67_1, float * var_67_2, float * var_67_3, float * var_67_4, float * var_67_5, float * var_67_6, float * var_67_7, float * var_67_8, float * var_67_9, float * var_67_10, float * var_67_11, float * var_67_12, float * var_67_13, float * var_67_14, float * var_67_15, float * var_67_16, float * var_67_17, float * var_67_18, float * var_67_19) {
	__shared__ float myVar[1024];
	myVar[9] = 13.140397 * myVar[threadIdx.x];
	myVar[9] = 46.523538 * myVar[threadIdx.x];
	myVar[7] = 3.033182 * myVar[threadIdx.x];
	myVar[5] = 26.795130 * myVar[threadIdx.x];
	myVar[9] = 30.995732 * myVar[threadIdx.x];
	myVar[8] = 38.365689 * myVar[threadIdx.x];
	myVar[5] = 23.620947 * myVar[threadIdx.x];
	myVar[0] = 15.542619 * myVar[threadIdx.x];
	myVar[8] = 38.233037 * myVar[threadIdx.x];
	myVar[9] = 19.422601 * myVar[threadIdx.x];
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
	var_67_10[10] = myVar[10];
	var_67_11[11] = myVar[11];
	var_67_12[12] = myVar[12];
	var_67_13[13] = myVar[13];
	var_67_14[14] = myVar[14];
	var_67_15[15] = myVar[15];
	var_67_16[16] = myVar[16];
	var_67_17[17] = myVar[17];
	var_67_18[18] = myVar[18];
	var_67_19[19] = myVar[19];
	
}

__global__ void kernel_68(float * var_68_0, float * var_68_1, float * var_68_2, float * var_68_3, float * var_68_4, float * var_68_5, float * var_68_6, float * var_68_7, float * var_68_8, float * var_68_9, float * var_68_10, float * var_68_11, float * var_68_12, float * var_68_13, float * var_68_14, float * var_68_15, float * var_68_16, float * var_68_17, float * var_68_18, float * var_68_19) {
	__shared__ float myVar[1024];
	myVar[8] = 43.766329 * myVar[threadIdx.x];
	myVar[6] = 27.192649 * myVar[threadIdx.x];
	myVar[8] = 23.785973 * myVar[threadIdx.x];
	myVar[9] = 41.367051 * myVar[threadIdx.x];
	myVar[3] = 9.792684 * myVar[threadIdx.x];
	myVar[6] = 31.583700 * myVar[threadIdx.x];
	myVar[9] = 40.443538 * myVar[threadIdx.x];
	myVar[9] = 49.320981 * myVar[threadIdx.x];
	myVar[9] = 34.924821 * myVar[threadIdx.x];
	myVar[8] = 48.775670 * myVar[threadIdx.x];
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
	var_68_10[10] = myVar[10];
	var_68_11[11] = myVar[11];
	var_68_12[12] = myVar[12];
	var_68_13[13] = myVar[13];
	var_68_14[14] = myVar[14];
	var_68_15[15] = myVar[15];
	var_68_16[16] = myVar[16];
	var_68_17[17] = myVar[17];
	var_68_18[18] = myVar[18];
	var_68_19[19] = myVar[19];
	
}

__global__ void kernel_69(float * var_69_0, float * var_69_1, float * var_69_2, float * var_69_3, float * var_69_4, float * var_69_5, float * var_69_6, float * var_69_7, float * var_69_8, float * var_69_9, float * var_69_10, float * var_69_11, float * var_69_12, float * var_69_13, float * var_69_14, float * var_69_15, float * var_69_16, float * var_69_17, float * var_69_18, float * var_69_19) {
	__shared__ float myVar[1024];
	myVar[9] = 7.707027 * myVar[threadIdx.x];
	myVar[7] = 21.683945 * myVar[threadIdx.x];
	myVar[6] = 10.428954 * myVar[threadIdx.x];
	myVar[2] = 24.683253 * myVar[threadIdx.x];
	myVar[6] = 2.906943 * myVar[threadIdx.x];
	myVar[0] = 9.477013 * myVar[threadIdx.x];
	myVar[6] = 46.410466 * myVar[threadIdx.x];
	myVar[7] = 39.701940 * myVar[threadIdx.x];
	myVar[6] = 29.047030 * myVar[threadIdx.x];
	myVar[7] = 8.740714 * myVar[threadIdx.x];
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
	var_69_10[10] = myVar[10];
	var_69_11[11] = myVar[11];
	var_69_12[12] = myVar[12];
	var_69_13[13] = myVar[13];
	var_69_14[14] = myVar[14];
	var_69_15[15] = myVar[15];
	var_69_16[16] = myVar[16];
	var_69_17[17] = myVar[17];
	var_69_18[18] = myVar[18];
	var_69_19[19] = myVar[19];
	
}

__global__ void kernel_70(float * var_70_0, float * var_70_1, float * var_70_2, float * var_70_3, float * var_70_4, float * var_70_5, float * var_70_6, float * var_70_7, float * var_70_8, float * var_70_9, float * var_70_10, float * var_70_11, float * var_70_12, float * var_70_13, float * var_70_14, float * var_70_15, float * var_70_16, float * var_70_17, float * var_70_18, float * var_70_19) {
	__shared__ float myVar[1024];
	myVar[0] = 32.466671 * myVar[threadIdx.x];
	myVar[6] = 4.534523 * myVar[threadIdx.x];
	myVar[3] = 11.770629 * myVar[threadIdx.x];
	myVar[8] = 46.817250 * myVar[threadIdx.x];
	myVar[1] = 10.812564 * myVar[threadIdx.x];
	myVar[4] = 1.846516 * myVar[threadIdx.x];
	myVar[9] = 35.385064 * myVar[threadIdx.x];
	myVar[6] = 2.934052 * myVar[threadIdx.x];
	myVar[7] = 4.579234 * myVar[threadIdx.x];
	myVar[3] = 4.102144 * myVar[threadIdx.x];
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
	var_70_10[10] = myVar[10];
	var_70_11[11] = myVar[11];
	var_70_12[12] = myVar[12];
	var_70_13[13] = myVar[13];
	var_70_14[14] = myVar[14];
	var_70_15[15] = myVar[15];
	var_70_16[16] = myVar[16];
	var_70_17[17] = myVar[17];
	var_70_18[18] = myVar[18];
	var_70_19[19] = myVar[19];
	
}

__global__ void kernel_71(float * var_71_0, float * var_71_1, float * var_71_2, float * var_71_3, float * var_71_4, float * var_71_5, float * var_71_6, float * var_71_7, float * var_71_8, float * var_71_9, float * var_71_10, float * var_71_11, float * var_71_12, float * var_71_13, float * var_71_14, float * var_71_15, float * var_71_16, float * var_71_17, float * var_71_18, float * var_71_19) {
	__shared__ float myVar[1024];
	myVar[4] = 41.170601 * myVar[threadIdx.x];
	myVar[4] = 13.651976 * myVar[threadIdx.x];
	myVar[6] = 6.834099 * myVar[threadIdx.x];
	myVar[2] = 26.910055 * myVar[threadIdx.x];
	myVar[9] = 43.342289 * myVar[threadIdx.x];
	myVar[8] = 10.524127 * myVar[threadIdx.x];
	myVar[7] = 40.450485 * myVar[threadIdx.x];
	myVar[4] = 2.319119 * myVar[threadIdx.x];
	myVar[3] = 34.342359 * myVar[threadIdx.x];
	myVar[0] = 30.794013 * myVar[threadIdx.x];
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
	var_71_10[10] = myVar[10];
	var_71_11[11] = myVar[11];
	var_71_12[12] = myVar[12];
	var_71_13[13] = myVar[13];
	var_71_14[14] = myVar[14];
	var_71_15[15] = myVar[15];
	var_71_16[16] = myVar[16];
	var_71_17[17] = myVar[17];
	var_71_18[18] = myVar[18];
	var_71_19[19] = myVar[19];
	
}

__global__ void kernel_72(float * var_72_0, float * var_72_1, float * var_72_2, float * var_72_3, float * var_72_4, float * var_72_5, float * var_72_6, float * var_72_7, float * var_72_8, float * var_72_9, float * var_72_10, float * var_72_11, float * var_72_12, float * var_72_13, float * var_72_14, float * var_72_15, float * var_72_16, float * var_72_17, float * var_72_18, float * var_72_19) {
	__shared__ float myVar[1024];
	myVar[4] = 21.942838 * myVar[threadIdx.x];
	myVar[7] = 7.979056 * myVar[threadIdx.x];
	myVar[9] = 42.630924 * myVar[threadIdx.x];
	myVar[8] = 26.467586 * myVar[threadIdx.x];
	myVar[7] = 3.136713 * myVar[threadIdx.x];
	myVar[6] = 29.039205 * myVar[threadIdx.x];
	myVar[7] = 15.514938 * myVar[threadIdx.x];
	myVar[1] = 24.760805 * myVar[threadIdx.x];
	myVar[5] = 14.885079 * myVar[threadIdx.x];
	myVar[2] = 4.947861 * myVar[threadIdx.x];
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
	var_72_10[10] = myVar[10];
	var_72_11[11] = myVar[11];
	var_72_12[12] = myVar[12];
	var_72_13[13] = myVar[13];
	var_72_14[14] = myVar[14];
	var_72_15[15] = myVar[15];
	var_72_16[16] = myVar[16];
	var_72_17[17] = myVar[17];
	var_72_18[18] = myVar[18];
	var_72_19[19] = myVar[19];
	
}

__global__ void kernel_73(float * var_73_0, float * var_73_1, float * var_73_2, float * var_73_3, float * var_73_4, float * var_73_5, float * var_73_6, float * var_73_7, float * var_73_8, float * var_73_9, float * var_73_10, float * var_73_11, float * var_73_12, float * var_73_13, float * var_73_14, float * var_73_15, float * var_73_16, float * var_73_17, float * var_73_18, float * var_73_19) {
	__shared__ float myVar[1024];
	myVar[1] = 19.735558 * myVar[threadIdx.x];
	myVar[9] = 18.724792 * myVar[threadIdx.x];
	myVar[1] = 41.933294 * myVar[threadIdx.x];
	myVar[2] = 33.499581 * myVar[threadIdx.x];
	myVar[9] = 17.797468 * myVar[threadIdx.x];
	myVar[9] = 5.813515 * myVar[threadIdx.x];
	myVar[2] = 8.649189 * myVar[threadIdx.x];
	myVar[9] = 13.243289 * myVar[threadIdx.x];
	myVar[7] = 32.770060 * myVar[threadIdx.x];
	myVar[4] = 5.658495 * myVar[threadIdx.x];
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
	var_73_10[10] = myVar[10];
	var_73_11[11] = myVar[11];
	var_73_12[12] = myVar[12];
	var_73_13[13] = myVar[13];
	var_73_14[14] = myVar[14];
	var_73_15[15] = myVar[15];
	var_73_16[16] = myVar[16];
	var_73_17[17] = myVar[17];
	var_73_18[18] = myVar[18];
	var_73_19[19] = myVar[19];
	
}

__global__ void kernel_74(float * var_74_0, float * var_74_1, float * var_74_2, float * var_74_3, float * var_74_4, float * var_74_5, float * var_74_6, float * var_74_7, float * var_74_8, float * var_74_9, float * var_74_10, float * var_74_11, float * var_74_12, float * var_74_13, float * var_74_14, float * var_74_15, float * var_74_16, float * var_74_17, float * var_74_18, float * var_74_19) {
	__shared__ float myVar[1024];
	myVar[2] = 22.888231 * myVar[threadIdx.x];
	myVar[0] = 19.859960 * myVar[threadIdx.x];
	myVar[7] = 47.655515 * myVar[threadIdx.x];
	myVar[8] = 15.599701 * myVar[threadIdx.x];
	myVar[5] = 25.612808 * myVar[threadIdx.x];
	myVar[9] = 22.018348 * myVar[threadIdx.x];
	myVar[1] = 7.647660 * myVar[threadIdx.x];
	myVar[9] = 49.856509 * myVar[threadIdx.x];
	myVar[3] = 18.859440 * myVar[threadIdx.x];
	myVar[1] = 36.744651 * myVar[threadIdx.x];
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
	var_74_10[10] = myVar[10];
	var_74_11[11] = myVar[11];
	var_74_12[12] = myVar[12];
	var_74_13[13] = myVar[13];
	var_74_14[14] = myVar[14];
	var_74_15[15] = myVar[15];
	var_74_16[16] = myVar[16];
	var_74_17[17] = myVar[17];
	var_74_18[18] = myVar[18];
	var_74_19[19] = myVar[19];
	
}

__global__ void kernel_75(float * var_75_0, float * var_75_1, float * var_75_2, float * var_75_3, float * var_75_4, float * var_75_5, float * var_75_6, float * var_75_7, float * var_75_8, float * var_75_9, float * var_75_10, float * var_75_11, float * var_75_12, float * var_75_13, float * var_75_14, float * var_75_15, float * var_75_16, float * var_75_17, float * var_75_18, float * var_75_19) {
	__shared__ float myVar[1024];
	myVar[0] = 28.702369 * myVar[threadIdx.x];
	myVar[3] = 33.381361 * myVar[threadIdx.x];
	myVar[7] = 12.768239 * myVar[threadIdx.x];
	myVar[3] = 48.437948 * myVar[threadIdx.x];
	myVar[0] = 26.398561 * myVar[threadIdx.x];
	myVar[7] = 49.402374 * myVar[threadIdx.x];
	myVar[5] = 44.292255 * myVar[threadIdx.x];
	myVar[1] = 28.579125 * myVar[threadIdx.x];
	myVar[0] = 12.116632 * myVar[threadIdx.x];
	myVar[6] = 12.440771 * myVar[threadIdx.x];
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
	var_75_10[10] = myVar[10];
	var_75_11[11] = myVar[11];
	var_75_12[12] = myVar[12];
	var_75_13[13] = myVar[13];
	var_75_14[14] = myVar[14];
	var_75_15[15] = myVar[15];
	var_75_16[16] = myVar[16];
	var_75_17[17] = myVar[17];
	var_75_18[18] = myVar[18];
	var_75_19[19] = myVar[19];
	
}

__global__ void kernel_76(float * var_76_0, float * var_76_1, float * var_76_2, float * var_76_3, float * var_76_4, float * var_76_5, float * var_76_6, float * var_76_7, float * var_76_8, float * var_76_9, float * var_76_10, float * var_76_11, float * var_76_12, float * var_76_13, float * var_76_14, float * var_76_15, float * var_76_16, float * var_76_17, float * var_76_18, float * var_76_19) {
	__shared__ float myVar[1024];
	myVar[5] = 32.013261 * myVar[threadIdx.x];
	myVar[6] = 14.655572 * myVar[threadIdx.x];
	myVar[6] = 24.448350 * myVar[threadIdx.x];
	myVar[5] = 26.084300 * myVar[threadIdx.x];
	myVar[2] = 2.995143 * myVar[threadIdx.x];
	myVar[9] = 12.297336 * myVar[threadIdx.x];
	myVar[3] = 13.592696 * myVar[threadIdx.x];
	myVar[7] = 30.036508 * myVar[threadIdx.x];
	myVar[6] = 34.314084 * myVar[threadIdx.x];
	myVar[8] = 3.278087 * myVar[threadIdx.x];
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
	var_76_10[10] = myVar[10];
	var_76_11[11] = myVar[11];
	var_76_12[12] = myVar[12];
	var_76_13[13] = myVar[13];
	var_76_14[14] = myVar[14];
	var_76_15[15] = myVar[15];
	var_76_16[16] = myVar[16];
	var_76_17[17] = myVar[17];
	var_76_18[18] = myVar[18];
	var_76_19[19] = myVar[19];
	
}

__global__ void kernel_77(float * var_77_0, float * var_77_1, float * var_77_2, float * var_77_3, float * var_77_4, float * var_77_5, float * var_77_6, float * var_77_7, float * var_77_8, float * var_77_9, float * var_77_10, float * var_77_11, float * var_77_12, float * var_77_13, float * var_77_14, float * var_77_15, float * var_77_16, float * var_77_17, float * var_77_18, float * var_77_19) {
	__shared__ float myVar[1024];
	myVar[6] = 0.586754 * myVar[threadIdx.x];
	myVar[2] = 21.242784 * myVar[threadIdx.x];
	myVar[6] = 19.633715 * myVar[threadIdx.x];
	myVar[3] = 7.058163 * myVar[threadIdx.x];
	myVar[5] = 17.066796 * myVar[threadIdx.x];
	myVar[9] = 49.404883 * myVar[threadIdx.x];
	myVar[7] = 5.806399 * myVar[threadIdx.x];
	myVar[4] = 24.515104 * myVar[threadIdx.x];
	myVar[9] = 44.272751 * myVar[threadIdx.x];
	myVar[0] = 24.247686 * myVar[threadIdx.x];
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
	var_77_10[10] = myVar[10];
	var_77_11[11] = myVar[11];
	var_77_12[12] = myVar[12];
	var_77_13[13] = myVar[13];
	var_77_14[14] = myVar[14];
	var_77_15[15] = myVar[15];
	var_77_16[16] = myVar[16];
	var_77_17[17] = myVar[17];
	var_77_18[18] = myVar[18];
	var_77_19[19] = myVar[19];
	
}

__global__ void kernel_78(float * var_78_0, float * var_78_1, float * var_78_2, float * var_78_3, float * var_78_4, float * var_78_5, float * var_78_6, float * var_78_7, float * var_78_8, float * var_78_9, float * var_78_10, float * var_78_11, float * var_78_12, float * var_78_13, float * var_78_14, float * var_78_15, float * var_78_16, float * var_78_17, float * var_78_18, float * var_78_19) {
	__shared__ float myVar[1024];
	myVar[0] = 1.064089 * myVar[threadIdx.x];
	myVar[4] = 19.377686 * myVar[threadIdx.x];
	myVar[1] = 41.891596 * myVar[threadIdx.x];
	myVar[2] = 32.112862 * myVar[threadIdx.x];
	myVar[2] = 47.966346 * myVar[threadIdx.x];
	myVar[7] = 13.487199 * myVar[threadIdx.x];
	myVar[2] = 9.251429 * myVar[threadIdx.x];
	myVar[8] = 20.397124 * myVar[threadIdx.x];
	myVar[8] = 12.054116 * myVar[threadIdx.x];
	myVar[1] = 30.061273 * myVar[threadIdx.x];
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
	var_78_10[10] = myVar[10];
	var_78_11[11] = myVar[11];
	var_78_12[12] = myVar[12];
	var_78_13[13] = myVar[13];
	var_78_14[14] = myVar[14];
	var_78_15[15] = myVar[15];
	var_78_16[16] = myVar[16];
	var_78_17[17] = myVar[17];
	var_78_18[18] = myVar[18];
	var_78_19[19] = myVar[19];
	
}

__global__ void kernel_79(float * var_79_0, float * var_79_1, float * var_79_2, float * var_79_3, float * var_79_4, float * var_79_5, float * var_79_6, float * var_79_7, float * var_79_8, float * var_79_9, float * var_79_10, float * var_79_11, float * var_79_12, float * var_79_13, float * var_79_14, float * var_79_15, float * var_79_16, float * var_79_17, float * var_79_18, float * var_79_19) {
	__shared__ float myVar[1024];
	myVar[3] = 18.196824 * myVar[threadIdx.x];
	myVar[7] = 43.837447 * myVar[threadIdx.x];
	myVar[4] = 23.153390 * myVar[threadIdx.x];
	myVar[4] = 27.992319 * myVar[threadIdx.x];
	myVar[5] = 9.024027 * myVar[threadIdx.x];
	myVar[1] = 48.804173 * myVar[threadIdx.x];
	myVar[0] = 7.408689 * myVar[threadIdx.x];
	myVar[7] = 23.778323 * myVar[threadIdx.x];
	myVar[6] = 42.920944 * myVar[threadIdx.x];
	myVar[7] = 29.065020 * myVar[threadIdx.x];
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
	var_79_10[10] = myVar[10];
	var_79_11[11] = myVar[11];
	var_79_12[12] = myVar[12];
	var_79_13[13] = myVar[13];
	var_79_14[14] = myVar[14];
	var_79_15[15] = myVar[15];
	var_79_16[16] = myVar[16];
	var_79_17[17] = myVar[17];
	var_79_18[18] = myVar[18];
	var_79_19[19] = myVar[19];
	
}

__global__ void kernel_80(float * var_80_0, float * var_80_1, float * var_80_2, float * var_80_3, float * var_80_4, float * var_80_5, float * var_80_6, float * var_80_7, float * var_80_8, float * var_80_9, float * var_80_10, float * var_80_11, float * var_80_12, float * var_80_13, float * var_80_14, float * var_80_15, float * var_80_16, float * var_80_17, float * var_80_18, float * var_80_19) {
	__shared__ float myVar[1024];
	myVar[6] = 29.986274 * myVar[threadIdx.x];
	myVar[8] = 6.206306 * myVar[threadIdx.x];
	myVar[5] = 13.512669 * myVar[threadIdx.x];
	myVar[5] = 39.159626 * myVar[threadIdx.x];
	myVar[4] = 34.758343 * myVar[threadIdx.x];
	myVar[1] = 37.380999 * myVar[threadIdx.x];
	myVar[1] = 49.354852 * myVar[threadIdx.x];
	myVar[9] = 38.446218 * myVar[threadIdx.x];
	myVar[3] = 39.416359 * myVar[threadIdx.x];
	myVar[8] = 42.874535 * myVar[threadIdx.x];
	var_80_0[0] = myVar[0];
	var_80_1[1] = myVar[1];
	var_80_2[2] = myVar[2];
	var_80_3[3] = myVar[3];
	var_80_4[4] = myVar[4];
	var_80_5[5] = myVar[5];
	var_80_6[6] = myVar[6];
	var_80_7[7] = myVar[7];
	var_80_8[8] = myVar[8];
	var_80_9[9] = myVar[9];
	var_80_10[10] = myVar[10];
	var_80_11[11] = myVar[11];
	var_80_12[12] = myVar[12];
	var_80_13[13] = myVar[13];
	var_80_14[14] = myVar[14];
	var_80_15[15] = myVar[15];
	var_80_16[16] = myVar[16];
	var_80_17[17] = myVar[17];
	var_80_18[18] = myVar[18];
	var_80_19[19] = myVar[19];
	
}

__global__ void kernel_81(float * var_81_0, float * var_81_1, float * var_81_2, float * var_81_3, float * var_81_4, float * var_81_5, float * var_81_6, float * var_81_7, float * var_81_8, float * var_81_9, float * var_81_10, float * var_81_11, float * var_81_12, float * var_81_13, float * var_81_14, float * var_81_15, float * var_81_16, float * var_81_17, float * var_81_18, float * var_81_19) {
	__shared__ float myVar[1024];
	myVar[8] = 47.171056 * myVar[threadIdx.x];
	myVar[9] = 34.012280 * myVar[threadIdx.x];
	myVar[0] = 48.939174 * myVar[threadIdx.x];
	myVar[2] = 23.415897 * myVar[threadIdx.x];
	myVar[9] = 11.547523 * myVar[threadIdx.x];
	myVar[9] = 46.820279 * myVar[threadIdx.x];
	myVar[7] = 30.271263 * myVar[threadIdx.x];
	myVar[4] = 27.460999 * myVar[threadIdx.x];
	myVar[8] = 41.792915 * myVar[threadIdx.x];
	myVar[0] = 3.939068 * myVar[threadIdx.x];
	var_81_0[0] = myVar[0];
	var_81_1[1] = myVar[1];
	var_81_2[2] = myVar[2];
	var_81_3[3] = myVar[3];
	var_81_4[4] = myVar[4];
	var_81_5[5] = myVar[5];
	var_81_6[6] = myVar[6];
	var_81_7[7] = myVar[7];
	var_81_8[8] = myVar[8];
	var_81_9[9] = myVar[9];
	var_81_10[10] = myVar[10];
	var_81_11[11] = myVar[11];
	var_81_12[12] = myVar[12];
	var_81_13[13] = myVar[13];
	var_81_14[14] = myVar[14];
	var_81_15[15] = myVar[15];
	var_81_16[16] = myVar[16];
	var_81_17[17] = myVar[17];
	var_81_18[18] = myVar[18];
	var_81_19[19] = myVar[19];
	
}

__global__ void kernel_82(float * var_82_0, float * var_82_1, float * var_82_2, float * var_82_3, float * var_82_4, float * var_82_5, float * var_82_6, float * var_82_7, float * var_82_8, float * var_82_9, float * var_82_10, float * var_82_11, float * var_82_12, float * var_82_13, float * var_82_14, float * var_82_15, float * var_82_16, float * var_82_17, float * var_82_18, float * var_82_19) {
	__shared__ float myVar[1024];
	myVar[1] = 24.598735 * myVar[threadIdx.x];
	myVar[8] = 27.287896 * myVar[threadIdx.x];
	myVar[8] = 44.845122 * myVar[threadIdx.x];
	myVar[1] = 20.370291 * myVar[threadIdx.x];
	myVar[1] = 17.560660 * myVar[threadIdx.x];
	myVar[9] = 36.935529 * myVar[threadIdx.x];
	myVar[9] = 36.543273 * myVar[threadIdx.x];
	myVar[2] = 1.067267 * myVar[threadIdx.x];
	myVar[4] = 24.287814 * myVar[threadIdx.x];
	myVar[0] = 48.009908 * myVar[threadIdx.x];
	var_82_0[0] = myVar[0];
	var_82_1[1] = myVar[1];
	var_82_2[2] = myVar[2];
	var_82_3[3] = myVar[3];
	var_82_4[4] = myVar[4];
	var_82_5[5] = myVar[5];
	var_82_6[6] = myVar[6];
	var_82_7[7] = myVar[7];
	var_82_8[8] = myVar[8];
	var_82_9[9] = myVar[9];
	var_82_10[10] = myVar[10];
	var_82_11[11] = myVar[11];
	var_82_12[12] = myVar[12];
	var_82_13[13] = myVar[13];
	var_82_14[14] = myVar[14];
	var_82_15[15] = myVar[15];
	var_82_16[16] = myVar[16];
	var_82_17[17] = myVar[17];
	var_82_18[18] = myVar[18];
	var_82_19[19] = myVar[19];
	
}

__global__ void kernel_83(float * var_83_0, float * var_83_1, float * var_83_2, float * var_83_3, float * var_83_4, float * var_83_5, float * var_83_6, float * var_83_7, float * var_83_8, float * var_83_9, float * var_83_10, float * var_83_11, float * var_83_12, float * var_83_13, float * var_83_14, float * var_83_15, float * var_83_16, float * var_83_17, float * var_83_18, float * var_83_19) {
	__shared__ float myVar[1024];
	myVar[9] = 36.390445 * myVar[threadIdx.x];
	myVar[0] = 39.153191 * myVar[threadIdx.x];
	myVar[5] = 17.985216 * myVar[threadIdx.x];
	myVar[1] = 22.930816 * myVar[threadIdx.x];
	myVar[1] = 6.413215 * myVar[threadIdx.x];
	myVar[3] = 19.841041 * myVar[threadIdx.x];
	myVar[6] = 28.020421 * myVar[threadIdx.x];
	myVar[8] = 26.730542 * myVar[threadIdx.x];
	myVar[7] = 23.492608 * myVar[threadIdx.x];
	myVar[1] = 32.477826 * myVar[threadIdx.x];
	var_83_0[0] = myVar[0];
	var_83_1[1] = myVar[1];
	var_83_2[2] = myVar[2];
	var_83_3[3] = myVar[3];
	var_83_4[4] = myVar[4];
	var_83_5[5] = myVar[5];
	var_83_6[6] = myVar[6];
	var_83_7[7] = myVar[7];
	var_83_8[8] = myVar[8];
	var_83_9[9] = myVar[9];
	var_83_10[10] = myVar[10];
	var_83_11[11] = myVar[11];
	var_83_12[12] = myVar[12];
	var_83_13[13] = myVar[13];
	var_83_14[14] = myVar[14];
	var_83_15[15] = myVar[15];
	var_83_16[16] = myVar[16];
	var_83_17[17] = myVar[17];
	var_83_18[18] = myVar[18];
	var_83_19[19] = myVar[19];
	
}

__global__ void kernel_84(float * var_84_0, float * var_84_1, float * var_84_2, float * var_84_3, float * var_84_4, float * var_84_5, float * var_84_6, float * var_84_7, float * var_84_8, float * var_84_9, float * var_84_10, float * var_84_11, float * var_84_12, float * var_84_13, float * var_84_14, float * var_84_15, float * var_84_16, float * var_84_17, float * var_84_18, float * var_84_19) {
	__shared__ float myVar[1024];
	myVar[1] = 15.828457 * myVar[threadIdx.x];
	myVar[8] = 11.603154 * myVar[threadIdx.x];
	myVar[7] = 23.479446 * myVar[threadIdx.x];
	myVar[9] = 40.390499 * myVar[threadIdx.x];
	myVar[0] = 49.498116 * myVar[threadIdx.x];
	myVar[5] = 5.547645 * myVar[threadIdx.x];
	myVar[6] = 32.120135 * myVar[threadIdx.x];
	myVar[8] = 13.189183 * myVar[threadIdx.x];
	myVar[4] = 5.747827 * myVar[threadIdx.x];
	myVar[7] = 13.207244 * myVar[threadIdx.x];
	var_84_0[0] = myVar[0];
	var_84_1[1] = myVar[1];
	var_84_2[2] = myVar[2];
	var_84_3[3] = myVar[3];
	var_84_4[4] = myVar[4];
	var_84_5[5] = myVar[5];
	var_84_6[6] = myVar[6];
	var_84_7[7] = myVar[7];
	var_84_8[8] = myVar[8];
	var_84_9[9] = myVar[9];
	var_84_10[10] = myVar[10];
	var_84_11[11] = myVar[11];
	var_84_12[12] = myVar[12];
	var_84_13[13] = myVar[13];
	var_84_14[14] = myVar[14];
	var_84_15[15] = myVar[15];
	var_84_16[16] = myVar[16];
	var_84_17[17] = myVar[17];
	var_84_18[18] = myVar[18];
	var_84_19[19] = myVar[19];
	
}

__global__ void kernel_85(float * var_85_0, float * var_85_1, float * var_85_2, float * var_85_3, float * var_85_4, float * var_85_5, float * var_85_6, float * var_85_7, float * var_85_8, float * var_85_9, float * var_85_10, float * var_85_11, float * var_85_12, float * var_85_13, float * var_85_14, float * var_85_15, float * var_85_16, float * var_85_17, float * var_85_18, float * var_85_19) {
	__shared__ float myVar[1024];
	myVar[5] = 29.469157 * myVar[threadIdx.x];
	myVar[1] = 8.046192 * myVar[threadIdx.x];
	myVar[6] = 23.251429 * myVar[threadIdx.x];
	myVar[3] = 38.798927 * myVar[threadIdx.x];
	myVar[1] = 5.437214 * myVar[threadIdx.x];
	myVar[9] = 16.948765 * myVar[threadIdx.x];
	myVar[6] = 38.654682 * myVar[threadIdx.x];
	myVar[0] = 39.937615 * myVar[threadIdx.x];
	myVar[1] = 46.182269 * myVar[threadIdx.x];
	myVar[2] = 10.417832 * myVar[threadIdx.x];
	var_85_0[0] = myVar[0];
	var_85_1[1] = myVar[1];
	var_85_2[2] = myVar[2];
	var_85_3[3] = myVar[3];
	var_85_4[4] = myVar[4];
	var_85_5[5] = myVar[5];
	var_85_6[6] = myVar[6];
	var_85_7[7] = myVar[7];
	var_85_8[8] = myVar[8];
	var_85_9[9] = myVar[9];
	var_85_10[10] = myVar[10];
	var_85_11[11] = myVar[11];
	var_85_12[12] = myVar[12];
	var_85_13[13] = myVar[13];
	var_85_14[14] = myVar[14];
	var_85_15[15] = myVar[15];
	var_85_16[16] = myVar[16];
	var_85_17[17] = myVar[17];
	var_85_18[18] = myVar[18];
	var_85_19[19] = myVar[19];
	
}

__global__ void kernel_86(float * var_86_0, float * var_86_1, float * var_86_2, float * var_86_3, float * var_86_4, float * var_86_5, float * var_86_6, float * var_86_7, float * var_86_8, float * var_86_9, float * var_86_10, float * var_86_11, float * var_86_12, float * var_86_13, float * var_86_14, float * var_86_15, float * var_86_16, float * var_86_17, float * var_86_18, float * var_86_19) {
	__shared__ float myVar[1024];
	myVar[7] = 20.686764 * myVar[threadIdx.x];
	myVar[8] = 14.382144 * myVar[threadIdx.x];
	myVar[4] = 2.592520 * myVar[threadIdx.x];
	myVar[8] = 32.843433 * myVar[threadIdx.x];
	myVar[7] = 20.987655 * myVar[threadIdx.x];
	myVar[3] = 19.882539 * myVar[threadIdx.x];
	myVar[8] = 29.850287 * myVar[threadIdx.x];
	myVar[2] = 37.142193 * myVar[threadIdx.x];
	myVar[7] = 15.355836 * myVar[threadIdx.x];
	myVar[2] = 31.991174 * myVar[threadIdx.x];
	var_86_0[0] = myVar[0];
	var_86_1[1] = myVar[1];
	var_86_2[2] = myVar[2];
	var_86_3[3] = myVar[3];
	var_86_4[4] = myVar[4];
	var_86_5[5] = myVar[5];
	var_86_6[6] = myVar[6];
	var_86_7[7] = myVar[7];
	var_86_8[8] = myVar[8];
	var_86_9[9] = myVar[9];
	var_86_10[10] = myVar[10];
	var_86_11[11] = myVar[11];
	var_86_12[12] = myVar[12];
	var_86_13[13] = myVar[13];
	var_86_14[14] = myVar[14];
	var_86_15[15] = myVar[15];
	var_86_16[16] = myVar[16];
	var_86_17[17] = myVar[17];
	var_86_18[18] = myVar[18];
	var_86_19[19] = myVar[19];
	
}

__global__ void kernel_87(float * var_87_0, float * var_87_1, float * var_87_2, float * var_87_3, float * var_87_4, float * var_87_5, float * var_87_6, float * var_87_7, float * var_87_8, float * var_87_9, float * var_87_10, float * var_87_11, float * var_87_12, float * var_87_13, float * var_87_14, float * var_87_15, float * var_87_16, float * var_87_17, float * var_87_18, float * var_87_19) {
	__shared__ float myVar[1024];
	myVar[0] = 17.818198 * myVar[threadIdx.x];
	myVar[7] = 17.292375 * myVar[threadIdx.x];
	myVar[3] = 9.408437 * myVar[threadIdx.x];
	myVar[0] = 35.787921 * myVar[threadIdx.x];
	myVar[7] = 31.576850 * myVar[threadIdx.x];
	myVar[0] = 43.302253 * myVar[threadIdx.x];
	myVar[5] = 45.580790 * myVar[threadIdx.x];
	myVar[5] = 38.798559 * myVar[threadIdx.x];
	myVar[0] = 16.410893 * myVar[threadIdx.x];
	myVar[6] = 8.753178 * myVar[threadIdx.x];
	var_87_0[0] = myVar[0];
	var_87_1[1] = myVar[1];
	var_87_2[2] = myVar[2];
	var_87_3[3] = myVar[3];
	var_87_4[4] = myVar[4];
	var_87_5[5] = myVar[5];
	var_87_6[6] = myVar[6];
	var_87_7[7] = myVar[7];
	var_87_8[8] = myVar[8];
	var_87_9[9] = myVar[9];
	var_87_10[10] = myVar[10];
	var_87_11[11] = myVar[11];
	var_87_12[12] = myVar[12];
	var_87_13[13] = myVar[13];
	var_87_14[14] = myVar[14];
	var_87_15[15] = myVar[15];
	var_87_16[16] = myVar[16];
	var_87_17[17] = myVar[17];
	var_87_18[18] = myVar[18];
	var_87_19[19] = myVar[19];
	
}

__global__ void kernel_88(float * var_88_0, float * var_88_1, float * var_88_2, float * var_88_3, float * var_88_4, float * var_88_5, float * var_88_6, float * var_88_7, float * var_88_8, float * var_88_9, float * var_88_10, float * var_88_11, float * var_88_12, float * var_88_13, float * var_88_14, float * var_88_15, float * var_88_16, float * var_88_17, float * var_88_18, float * var_88_19) {
	__shared__ float myVar[1024];
	myVar[7] = 3.302447 * myVar[threadIdx.x];
	myVar[8] = 32.859062 * myVar[threadIdx.x];
	myVar[5] = 6.183310 * myVar[threadIdx.x];
	myVar[8] = 34.710643 * myVar[threadIdx.x];
	myVar[1] = 37.096195 * myVar[threadIdx.x];
	myVar[1] = 43.159629 * myVar[threadIdx.x];
	myVar[8] = 10.165780 * myVar[threadIdx.x];
	myVar[9] = 46.500045 * myVar[threadIdx.x];
	myVar[3] = 16.006992 * myVar[threadIdx.x];
	myVar[8] = 8.909091 * myVar[threadIdx.x];
	var_88_0[0] = myVar[0];
	var_88_1[1] = myVar[1];
	var_88_2[2] = myVar[2];
	var_88_3[3] = myVar[3];
	var_88_4[4] = myVar[4];
	var_88_5[5] = myVar[5];
	var_88_6[6] = myVar[6];
	var_88_7[7] = myVar[7];
	var_88_8[8] = myVar[8];
	var_88_9[9] = myVar[9];
	var_88_10[10] = myVar[10];
	var_88_11[11] = myVar[11];
	var_88_12[12] = myVar[12];
	var_88_13[13] = myVar[13];
	var_88_14[14] = myVar[14];
	var_88_15[15] = myVar[15];
	var_88_16[16] = myVar[16];
	var_88_17[17] = myVar[17];
	var_88_18[18] = myVar[18];
	var_88_19[19] = myVar[19];
	
}

__global__ void kernel_89(float * var_89_0, float * var_89_1, float * var_89_2, float * var_89_3, float * var_89_4, float * var_89_5, float * var_89_6, float * var_89_7, float * var_89_8, float * var_89_9, float * var_89_10, float * var_89_11, float * var_89_12, float * var_89_13, float * var_89_14, float * var_89_15, float * var_89_16, float * var_89_17, float * var_89_18, float * var_89_19) {
	__shared__ float myVar[1024];
	myVar[2] = 8.884512 * myVar[threadIdx.x];
	myVar[2] = 26.835709 * myVar[threadIdx.x];
	myVar[9] = 24.787851 * myVar[threadIdx.x];
	myVar[9] = 11.447755 * myVar[threadIdx.x];
	myVar[5] = 46.979558 * myVar[threadIdx.x];
	myVar[5] = 10.651160 * myVar[threadIdx.x];
	myVar[7] = 3.243080 * myVar[threadIdx.x];
	myVar[2] = 31.164741 * myVar[threadIdx.x];
	myVar[5] = 36.848732 * myVar[threadIdx.x];
	myVar[8] = 10.391745 * myVar[threadIdx.x];
	var_89_0[0] = myVar[0];
	var_89_1[1] = myVar[1];
	var_89_2[2] = myVar[2];
	var_89_3[3] = myVar[3];
	var_89_4[4] = myVar[4];
	var_89_5[5] = myVar[5];
	var_89_6[6] = myVar[6];
	var_89_7[7] = myVar[7];
	var_89_8[8] = myVar[8];
	var_89_9[9] = myVar[9];
	var_89_10[10] = myVar[10];
	var_89_11[11] = myVar[11];
	var_89_12[12] = myVar[12];
	var_89_13[13] = myVar[13];
	var_89_14[14] = myVar[14];
	var_89_15[15] = myVar[15];
	var_89_16[16] = myVar[16];
	var_89_17[17] = myVar[17];
	var_89_18[18] = myVar[18];
	var_89_19[19] = myVar[19];
	
}

__global__ void kernel_90(float * var_90_0, float * var_90_1, float * var_90_2, float * var_90_3, float * var_90_4, float * var_90_5, float * var_90_6, float * var_90_7, float * var_90_8, float * var_90_9, float * var_90_10, float * var_90_11, float * var_90_12, float * var_90_13, float * var_90_14, float * var_90_15, float * var_90_16, float * var_90_17, float * var_90_18, float * var_90_19) {
	__shared__ float myVar[1024];
	myVar[8] = 20.418554 * myVar[threadIdx.x];
	myVar[9] = 8.802054 * myVar[threadIdx.x];
	myVar[7] = 43.815346 * myVar[threadIdx.x];
	myVar[4] = 45.765129 * myVar[threadIdx.x];
	myVar[6] = 8.567715 * myVar[threadIdx.x];
	myVar[0] = 27.280536 * myVar[threadIdx.x];
	myVar[3] = 44.621018 * myVar[threadIdx.x];
	myVar[8] = 40.014008 * myVar[threadIdx.x];
	myVar[6] = 7.637198 * myVar[threadIdx.x];
	myVar[4] = 10.862993 * myVar[threadIdx.x];
	var_90_0[0] = myVar[0];
	var_90_1[1] = myVar[1];
	var_90_2[2] = myVar[2];
	var_90_3[3] = myVar[3];
	var_90_4[4] = myVar[4];
	var_90_5[5] = myVar[5];
	var_90_6[6] = myVar[6];
	var_90_7[7] = myVar[7];
	var_90_8[8] = myVar[8];
	var_90_9[9] = myVar[9];
	var_90_10[10] = myVar[10];
	var_90_11[11] = myVar[11];
	var_90_12[12] = myVar[12];
	var_90_13[13] = myVar[13];
	var_90_14[14] = myVar[14];
	var_90_15[15] = myVar[15];
	var_90_16[16] = myVar[16];
	var_90_17[17] = myVar[17];
	var_90_18[18] = myVar[18];
	var_90_19[19] = myVar[19];
	
}

__global__ void kernel_91(float * var_91_0, float * var_91_1, float * var_91_2, float * var_91_3, float * var_91_4, float * var_91_5, float * var_91_6, float * var_91_7, float * var_91_8, float * var_91_9, float * var_91_10, float * var_91_11, float * var_91_12, float * var_91_13, float * var_91_14, float * var_91_15, float * var_91_16, float * var_91_17, float * var_91_18, float * var_91_19) {
	__shared__ float myVar[1024];
	myVar[5] = 10.636806 * myVar[threadIdx.x];
	myVar[6] = 25.575829 * myVar[threadIdx.x];
	myVar[7] = 26.068188 * myVar[threadIdx.x];
	myVar[4] = 48.151045 * myVar[threadIdx.x];
	myVar[0] = 4.807278 * myVar[threadIdx.x];
	myVar[6] = 14.784279 * myVar[threadIdx.x];
	myVar[7] = 49.478457 * myVar[threadIdx.x];
	myVar[6] = 5.982118 * myVar[threadIdx.x];
	myVar[7] = 0.298905 * myVar[threadIdx.x];
	myVar[6] = 33.926429 * myVar[threadIdx.x];
	var_91_0[0] = myVar[0];
	var_91_1[1] = myVar[1];
	var_91_2[2] = myVar[2];
	var_91_3[3] = myVar[3];
	var_91_4[4] = myVar[4];
	var_91_5[5] = myVar[5];
	var_91_6[6] = myVar[6];
	var_91_7[7] = myVar[7];
	var_91_8[8] = myVar[8];
	var_91_9[9] = myVar[9];
	var_91_10[10] = myVar[10];
	var_91_11[11] = myVar[11];
	var_91_12[12] = myVar[12];
	var_91_13[13] = myVar[13];
	var_91_14[14] = myVar[14];
	var_91_15[15] = myVar[15];
	var_91_16[16] = myVar[16];
	var_91_17[17] = myVar[17];
	var_91_18[18] = myVar[18];
	var_91_19[19] = myVar[19];
	
}

__global__ void kernel_92(float * var_92_0, float * var_92_1, float * var_92_2, float * var_92_3, float * var_92_4, float * var_92_5, float * var_92_6, float * var_92_7, float * var_92_8, float * var_92_9, float * var_92_10, float * var_92_11, float * var_92_12, float * var_92_13, float * var_92_14, float * var_92_15, float * var_92_16, float * var_92_17, float * var_92_18, float * var_92_19) {
	__shared__ float myVar[1024];
	myVar[1] = 27.983791 * myVar[threadIdx.x];
	myVar[4] = 48.063810 * myVar[threadIdx.x];
	myVar[4] = 3.419796 * myVar[threadIdx.x];
	myVar[9] = 3.814167 * myVar[threadIdx.x];
	myVar[6] = 49.538755 * myVar[threadIdx.x];
	myVar[8] = 14.594070 * myVar[threadIdx.x];
	myVar[8] = 0.899012 * myVar[threadIdx.x];
	myVar[0] = 28.691193 * myVar[threadIdx.x];
	myVar[2] = 22.566337 * myVar[threadIdx.x];
	myVar[6] = 26.554205 * myVar[threadIdx.x];
	var_92_0[0] = myVar[0];
	var_92_1[1] = myVar[1];
	var_92_2[2] = myVar[2];
	var_92_3[3] = myVar[3];
	var_92_4[4] = myVar[4];
	var_92_5[5] = myVar[5];
	var_92_6[6] = myVar[6];
	var_92_7[7] = myVar[7];
	var_92_8[8] = myVar[8];
	var_92_9[9] = myVar[9];
	var_92_10[10] = myVar[10];
	var_92_11[11] = myVar[11];
	var_92_12[12] = myVar[12];
	var_92_13[13] = myVar[13];
	var_92_14[14] = myVar[14];
	var_92_15[15] = myVar[15];
	var_92_16[16] = myVar[16];
	var_92_17[17] = myVar[17];
	var_92_18[18] = myVar[18];
	var_92_19[19] = myVar[19];
	
}

__global__ void kernel_93(float * var_93_0, float * var_93_1, float * var_93_2, float * var_93_3, float * var_93_4, float * var_93_5, float * var_93_6, float * var_93_7, float * var_93_8, float * var_93_9, float * var_93_10, float * var_93_11, float * var_93_12, float * var_93_13, float * var_93_14, float * var_93_15, float * var_93_16, float * var_93_17, float * var_93_18, float * var_93_19) {
	__shared__ float myVar[1024];
	myVar[7] = 25.247132 * myVar[threadIdx.x];
	myVar[7] = 35.564602 * myVar[threadIdx.x];
	myVar[1] = 19.893224 * myVar[threadIdx.x];
	myVar[4] = 38.999653 * myVar[threadIdx.x];
	myVar[3] = 32.827499 * myVar[threadIdx.x];
	myVar[8] = 37.567465 * myVar[threadIdx.x];
	myVar[2] = 32.180819 * myVar[threadIdx.x];
	myVar[5] = 29.349211 * myVar[threadIdx.x];
	myVar[6] = 47.392912 * myVar[threadIdx.x];
	myVar[7] = 23.877738 * myVar[threadIdx.x];
	var_93_0[0] = myVar[0];
	var_93_1[1] = myVar[1];
	var_93_2[2] = myVar[2];
	var_93_3[3] = myVar[3];
	var_93_4[4] = myVar[4];
	var_93_5[5] = myVar[5];
	var_93_6[6] = myVar[6];
	var_93_7[7] = myVar[7];
	var_93_8[8] = myVar[8];
	var_93_9[9] = myVar[9];
	var_93_10[10] = myVar[10];
	var_93_11[11] = myVar[11];
	var_93_12[12] = myVar[12];
	var_93_13[13] = myVar[13];
	var_93_14[14] = myVar[14];
	var_93_15[15] = myVar[15];
	var_93_16[16] = myVar[16];
	var_93_17[17] = myVar[17];
	var_93_18[18] = myVar[18];
	var_93_19[19] = myVar[19];
	
}

__global__ void kernel_94(float * var_94_0, float * var_94_1, float * var_94_2, float * var_94_3, float * var_94_4, float * var_94_5, float * var_94_6, float * var_94_7, float * var_94_8, float * var_94_9, float * var_94_10, float * var_94_11, float * var_94_12, float * var_94_13, float * var_94_14, float * var_94_15, float * var_94_16, float * var_94_17, float * var_94_18, float * var_94_19) {
	__shared__ float myVar[1024];
	myVar[9] = 22.970190 * myVar[threadIdx.x];
	myVar[9] = 0.697030 * myVar[threadIdx.x];
	myVar[2] = 27.841112 * myVar[threadIdx.x];
	myVar[8] = 8.810656 * myVar[threadIdx.x];
	myVar[3] = 17.252632 * myVar[threadIdx.x];
	myVar[2] = 15.302823 * myVar[threadIdx.x];
	myVar[7] = 43.331670 * myVar[threadIdx.x];
	myVar[6] = 36.704199 * myVar[threadIdx.x];
	myVar[2] = 30.115334 * myVar[threadIdx.x];
	myVar[3] = 2.212627 * myVar[threadIdx.x];
	var_94_0[0] = myVar[0];
	var_94_1[1] = myVar[1];
	var_94_2[2] = myVar[2];
	var_94_3[3] = myVar[3];
	var_94_4[4] = myVar[4];
	var_94_5[5] = myVar[5];
	var_94_6[6] = myVar[6];
	var_94_7[7] = myVar[7];
	var_94_8[8] = myVar[8];
	var_94_9[9] = myVar[9];
	var_94_10[10] = myVar[10];
	var_94_11[11] = myVar[11];
	var_94_12[12] = myVar[12];
	var_94_13[13] = myVar[13];
	var_94_14[14] = myVar[14];
	var_94_15[15] = myVar[15];
	var_94_16[16] = myVar[16];
	var_94_17[17] = myVar[17];
	var_94_18[18] = myVar[18];
	var_94_19[19] = myVar[19];
	
}

__global__ void kernel_95(float * var_95_0, float * var_95_1, float * var_95_2, float * var_95_3, float * var_95_4, float * var_95_5, float * var_95_6, float * var_95_7, float * var_95_8, float * var_95_9, float * var_95_10, float * var_95_11, float * var_95_12, float * var_95_13, float * var_95_14, float * var_95_15, float * var_95_16, float * var_95_17, float * var_95_18, float * var_95_19) {
	__shared__ float myVar[1024];
	myVar[0] = 24.147069 * myVar[threadIdx.x];
	myVar[8] = 10.496619 * myVar[threadIdx.x];
	myVar[0] = 36.443158 * myVar[threadIdx.x];
	myVar[0] = 17.904213 * myVar[threadIdx.x];
	myVar[1] = 49.766844 * myVar[threadIdx.x];
	myVar[9] = 17.379044 * myVar[threadIdx.x];
	myVar[6] = 32.826730 * myVar[threadIdx.x];
	myVar[4] = 44.411972 * myVar[threadIdx.x];
	myVar[5] = 26.416494 * myVar[threadIdx.x];
	myVar[5] = 21.628386 * myVar[threadIdx.x];
	var_95_0[0] = myVar[0];
	var_95_1[1] = myVar[1];
	var_95_2[2] = myVar[2];
	var_95_3[3] = myVar[3];
	var_95_4[4] = myVar[4];
	var_95_5[5] = myVar[5];
	var_95_6[6] = myVar[6];
	var_95_7[7] = myVar[7];
	var_95_8[8] = myVar[8];
	var_95_9[9] = myVar[9];
	var_95_10[10] = myVar[10];
	var_95_11[11] = myVar[11];
	var_95_12[12] = myVar[12];
	var_95_13[13] = myVar[13];
	var_95_14[14] = myVar[14];
	var_95_15[15] = myVar[15];
	var_95_16[16] = myVar[16];
	var_95_17[17] = myVar[17];
	var_95_18[18] = myVar[18];
	var_95_19[19] = myVar[19];
	
}

__global__ void kernel_96(float * var_96_0, float * var_96_1, float * var_96_2, float * var_96_3, float * var_96_4, float * var_96_5, float * var_96_6, float * var_96_7, float * var_96_8, float * var_96_9, float * var_96_10, float * var_96_11, float * var_96_12, float * var_96_13, float * var_96_14, float * var_96_15, float * var_96_16, float * var_96_17, float * var_96_18, float * var_96_19) {
	__shared__ float myVar[1024];
	myVar[8] = 13.596222 * myVar[threadIdx.x];
	myVar[9] = 35.570629 * myVar[threadIdx.x];
	myVar[3] = 6.256915 * myVar[threadIdx.x];
	myVar[1] = 35.742467 * myVar[threadIdx.x];
	myVar[0] = 21.909653 * myVar[threadIdx.x];
	myVar[9] = 23.225823 * myVar[threadIdx.x];
	myVar[9] = 41.822623 * myVar[threadIdx.x];
	myVar[4] = 49.545857 * myVar[threadIdx.x];
	myVar[6] = 14.578220 * myVar[threadIdx.x];
	myVar[5] = 26.371621 * myVar[threadIdx.x];
	var_96_0[0] = myVar[0];
	var_96_1[1] = myVar[1];
	var_96_2[2] = myVar[2];
	var_96_3[3] = myVar[3];
	var_96_4[4] = myVar[4];
	var_96_5[5] = myVar[5];
	var_96_6[6] = myVar[6];
	var_96_7[7] = myVar[7];
	var_96_8[8] = myVar[8];
	var_96_9[9] = myVar[9];
	var_96_10[10] = myVar[10];
	var_96_11[11] = myVar[11];
	var_96_12[12] = myVar[12];
	var_96_13[13] = myVar[13];
	var_96_14[14] = myVar[14];
	var_96_15[15] = myVar[15];
	var_96_16[16] = myVar[16];
	var_96_17[17] = myVar[17];
	var_96_18[18] = myVar[18];
	var_96_19[19] = myVar[19];
	
}

__global__ void kernel_97(float * var_97_0, float * var_97_1, float * var_97_2, float * var_97_3, float * var_97_4, float * var_97_5, float * var_97_6, float * var_97_7, float * var_97_8, float * var_97_9, float * var_97_10, float * var_97_11, float * var_97_12, float * var_97_13, float * var_97_14, float * var_97_15, float * var_97_16, float * var_97_17, float * var_97_18, float * var_97_19) {
	__shared__ float myVar[1024];
	myVar[3] = 7.616703 * myVar[threadIdx.x];
	myVar[3] = 24.963788 * myVar[threadIdx.x];
	myVar[9] = 32.178852 * myVar[threadIdx.x];
	myVar[8] = 14.660428 * myVar[threadIdx.x];
	myVar[1] = 10.423802 * myVar[threadIdx.x];
	myVar[5] = 34.645808 * myVar[threadIdx.x];
	myVar[8] = 7.513570 * myVar[threadIdx.x];
	myVar[5] = 19.526371 * myVar[threadIdx.x];
	myVar[5] = 8.128375 * myVar[threadIdx.x];
	myVar[7] = 27.257317 * myVar[threadIdx.x];
	var_97_0[0] = myVar[0];
	var_97_1[1] = myVar[1];
	var_97_2[2] = myVar[2];
	var_97_3[3] = myVar[3];
	var_97_4[4] = myVar[4];
	var_97_5[5] = myVar[5];
	var_97_6[6] = myVar[6];
	var_97_7[7] = myVar[7];
	var_97_8[8] = myVar[8];
	var_97_9[9] = myVar[9];
	var_97_10[10] = myVar[10];
	var_97_11[11] = myVar[11];
	var_97_12[12] = myVar[12];
	var_97_13[13] = myVar[13];
	var_97_14[14] = myVar[14];
	var_97_15[15] = myVar[15];
	var_97_16[16] = myVar[16];
	var_97_17[17] = myVar[17];
	var_97_18[18] = myVar[18];
	var_97_19[19] = myVar[19];
	
}

__global__ void kernel_98(float * var_98_0, float * var_98_1, float * var_98_2, float * var_98_3, float * var_98_4, float * var_98_5, float * var_98_6, float * var_98_7, float * var_98_8, float * var_98_9, float * var_98_10, float * var_98_11, float * var_98_12, float * var_98_13, float * var_98_14, float * var_98_15, float * var_98_16, float * var_98_17, float * var_98_18, float * var_98_19) {
	__shared__ float myVar[1024];
	myVar[5] = 22.933224 * myVar[threadIdx.x];
	myVar[6] = 33.404707 * myVar[threadIdx.x];
	myVar[1] = 12.560293 * myVar[threadIdx.x];
	myVar[6] = 12.533825 * myVar[threadIdx.x];
	myVar[4] = 8.005213 * myVar[threadIdx.x];
	myVar[2] = 37.231474 * myVar[threadIdx.x];
	myVar[1] = 48.695888 * myVar[threadIdx.x];
	myVar[5] = 43.870667 * myVar[threadIdx.x];
	myVar[1] = 5.068447 * myVar[threadIdx.x];
	myVar[4] = 31.245133 * myVar[threadIdx.x];
	var_98_0[0] = myVar[0];
	var_98_1[1] = myVar[1];
	var_98_2[2] = myVar[2];
	var_98_3[3] = myVar[3];
	var_98_4[4] = myVar[4];
	var_98_5[5] = myVar[5];
	var_98_6[6] = myVar[6];
	var_98_7[7] = myVar[7];
	var_98_8[8] = myVar[8];
	var_98_9[9] = myVar[9];
	var_98_10[10] = myVar[10];
	var_98_11[11] = myVar[11];
	var_98_12[12] = myVar[12];
	var_98_13[13] = myVar[13];
	var_98_14[14] = myVar[14];
	var_98_15[15] = myVar[15];
	var_98_16[16] = myVar[16];
	var_98_17[17] = myVar[17];
	var_98_18[18] = myVar[18];
	var_98_19[19] = myVar[19];
	
}

__global__ void kernel_99(float * var_99_0, float * var_99_1, float * var_99_2, float * var_99_3, float * var_99_4, float * var_99_5, float * var_99_6, float * var_99_7, float * var_99_8, float * var_99_9, float * var_99_10, float * var_99_11, float * var_99_12, float * var_99_13, float * var_99_14, float * var_99_15, float * var_99_16, float * var_99_17, float * var_99_18, float * var_99_19) {
	__shared__ float myVar[1024];
	myVar[6] = 27.205712 * myVar[threadIdx.x];
	myVar[2] = 3.981201 * myVar[threadIdx.x];
	myVar[5] = 37.854242 * myVar[threadIdx.x];
	myVar[8] = 5.116412 * myVar[threadIdx.x];
	myVar[5] = 13.977419 * myVar[threadIdx.x];
	myVar[0] = 40.107187 * myVar[threadIdx.x];
	myVar[0] = 18.660288 * myVar[threadIdx.x];
	myVar[7] = 18.056329 * myVar[threadIdx.x];
	myVar[6] = 12.940238 * myVar[threadIdx.x];
	myVar[8] = 33.224260 * myVar[threadIdx.x];
	var_99_0[0] = myVar[0];
	var_99_1[1] = myVar[1];
	var_99_2[2] = myVar[2];
	var_99_3[3] = myVar[3];
	var_99_4[4] = myVar[4];
	var_99_5[5] = myVar[5];
	var_99_6[6] = myVar[6];
	var_99_7[7] = myVar[7];
	var_99_8[8] = myVar[8];
	var_99_9[9] = myVar[9];
	var_99_10[10] = myVar[10];
	var_99_11[11] = myVar[11];
	var_99_12[12] = myVar[12];
	var_99_13[13] = myVar[13];
	var_99_14[14] = myVar[14];
	var_99_15[15] = myVar[15];
	var_99_16[16] = myVar[16];
	var_99_17[17] = myVar[17];
	var_99_18[18] = myVar[18];
	var_99_19[19] = myVar[19];
	
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
	
	float * h_var_0_10 = (float *)malloc(sizeof(float *));
	float * d_var_0_10;
	cudaMalloc((void **)&d_var_0_10, sizeof(float *));
	
	float * h_var_0_11 = (float *)malloc(sizeof(float *));
	float * d_var_0_11;
	cudaMalloc((void **)&d_var_0_11, sizeof(float *));
	
	float * h_var_0_12 = (float *)malloc(sizeof(float *));
	float * d_var_0_12;
	cudaMalloc((void **)&d_var_0_12, sizeof(float *));
	
	float * h_var_0_13 = (float *)malloc(sizeof(float *));
	float * d_var_0_13;
	cudaMalloc((void **)&d_var_0_13, sizeof(float *));
	
	float * h_var_0_14 = (float *)malloc(sizeof(float *));
	float * d_var_0_14;
	cudaMalloc((void **)&d_var_0_14, sizeof(float *));
	
	float * h_var_0_15 = (float *)malloc(sizeof(float *));
	float * d_var_0_15;
	cudaMalloc((void **)&d_var_0_15, sizeof(float *));
	
	float * h_var_0_16 = (float *)malloc(sizeof(float *));
	float * d_var_0_16;
	cudaMalloc((void **)&d_var_0_16, sizeof(float *));
	
	float * h_var_0_17 = (float *)malloc(sizeof(float *));
	float * d_var_0_17;
	cudaMalloc((void **)&d_var_0_17, sizeof(float *));
	
	float * h_var_0_18 = (float *)malloc(sizeof(float *));
	float * d_var_0_18;
	cudaMalloc((void **)&d_var_0_18, sizeof(float *));
	
	float * h_var_0_19 = (float *)malloc(sizeof(float *));
	float * d_var_0_19;
	cudaMalloc((void **)&d_var_0_19, sizeof(float *));
	
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
	
	float * h_var_1_10 = (float *)malloc(sizeof(float *));
	float * d_var_1_10;
	cudaMalloc((void **)&d_var_1_10, sizeof(float *));
	
	float * h_var_1_11 = (float *)malloc(sizeof(float *));
	float * d_var_1_11;
	cudaMalloc((void **)&d_var_1_11, sizeof(float *));
	
	float * h_var_1_12 = (float *)malloc(sizeof(float *));
	float * d_var_1_12;
	cudaMalloc((void **)&d_var_1_12, sizeof(float *));
	
	float * h_var_1_13 = (float *)malloc(sizeof(float *));
	float * d_var_1_13;
	cudaMalloc((void **)&d_var_1_13, sizeof(float *));
	
	float * h_var_1_14 = (float *)malloc(sizeof(float *));
	float * d_var_1_14;
	cudaMalloc((void **)&d_var_1_14, sizeof(float *));
	
	float * h_var_1_15 = (float *)malloc(sizeof(float *));
	float * d_var_1_15;
	cudaMalloc((void **)&d_var_1_15, sizeof(float *));
	
	float * h_var_1_16 = (float *)malloc(sizeof(float *));
	float * d_var_1_16;
	cudaMalloc((void **)&d_var_1_16, sizeof(float *));
	
	float * h_var_1_17 = (float *)malloc(sizeof(float *));
	float * d_var_1_17;
	cudaMalloc((void **)&d_var_1_17, sizeof(float *));
	
	float * h_var_1_18 = (float *)malloc(sizeof(float *));
	float * d_var_1_18;
	cudaMalloc((void **)&d_var_1_18, sizeof(float *));
	
	float * h_var_1_19 = (float *)malloc(sizeof(float *));
	float * d_var_1_19;
	cudaMalloc((void **)&d_var_1_19, sizeof(float *));
	
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
	
	float * h_var_2_10 = (float *)malloc(sizeof(float *));
	float * d_var_2_10;
	cudaMalloc((void **)&d_var_2_10, sizeof(float *));
	
	float * h_var_2_11 = (float *)malloc(sizeof(float *));
	float * d_var_2_11;
	cudaMalloc((void **)&d_var_2_11, sizeof(float *));
	
	float * h_var_2_12 = (float *)malloc(sizeof(float *));
	float * d_var_2_12;
	cudaMalloc((void **)&d_var_2_12, sizeof(float *));
	
	float * h_var_2_13 = (float *)malloc(sizeof(float *));
	float * d_var_2_13;
	cudaMalloc((void **)&d_var_2_13, sizeof(float *));
	
	float * h_var_2_14 = (float *)malloc(sizeof(float *));
	float * d_var_2_14;
	cudaMalloc((void **)&d_var_2_14, sizeof(float *));
	
	float * h_var_2_15 = (float *)malloc(sizeof(float *));
	float * d_var_2_15;
	cudaMalloc((void **)&d_var_2_15, sizeof(float *));
	
	float * h_var_2_16 = (float *)malloc(sizeof(float *));
	float * d_var_2_16;
	cudaMalloc((void **)&d_var_2_16, sizeof(float *));
	
	float * h_var_2_17 = (float *)malloc(sizeof(float *));
	float * d_var_2_17;
	cudaMalloc((void **)&d_var_2_17, sizeof(float *));
	
	float * h_var_2_18 = (float *)malloc(sizeof(float *));
	float * d_var_2_18;
	cudaMalloc((void **)&d_var_2_18, sizeof(float *));
	
	float * h_var_2_19 = (float *)malloc(sizeof(float *));
	float * d_var_2_19;
	cudaMalloc((void **)&d_var_2_19, sizeof(float *));
	
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
	
	float * h_var_3_10 = (float *)malloc(sizeof(float *));
	float * d_var_3_10;
	cudaMalloc((void **)&d_var_3_10, sizeof(float *));
	
	float * h_var_3_11 = (float *)malloc(sizeof(float *));
	float * d_var_3_11;
	cudaMalloc((void **)&d_var_3_11, sizeof(float *));
	
	float * h_var_3_12 = (float *)malloc(sizeof(float *));
	float * d_var_3_12;
	cudaMalloc((void **)&d_var_3_12, sizeof(float *));
	
	float * h_var_3_13 = (float *)malloc(sizeof(float *));
	float * d_var_3_13;
	cudaMalloc((void **)&d_var_3_13, sizeof(float *));
	
	float * h_var_3_14 = (float *)malloc(sizeof(float *));
	float * d_var_3_14;
	cudaMalloc((void **)&d_var_3_14, sizeof(float *));
	
	float * h_var_3_15 = (float *)malloc(sizeof(float *));
	float * d_var_3_15;
	cudaMalloc((void **)&d_var_3_15, sizeof(float *));
	
	float * h_var_3_16 = (float *)malloc(sizeof(float *));
	float * d_var_3_16;
	cudaMalloc((void **)&d_var_3_16, sizeof(float *));
	
	float * h_var_3_17 = (float *)malloc(sizeof(float *));
	float * d_var_3_17;
	cudaMalloc((void **)&d_var_3_17, sizeof(float *));
	
	float * h_var_3_18 = (float *)malloc(sizeof(float *));
	float * d_var_3_18;
	cudaMalloc((void **)&d_var_3_18, sizeof(float *));
	
	float * h_var_3_19 = (float *)malloc(sizeof(float *));
	float * d_var_3_19;
	cudaMalloc((void **)&d_var_3_19, sizeof(float *));
	
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
	
	float * h_var_4_10 = (float *)malloc(sizeof(float *));
	float * d_var_4_10;
	cudaMalloc((void **)&d_var_4_10, sizeof(float *));
	
	float * h_var_4_11 = (float *)malloc(sizeof(float *));
	float * d_var_4_11;
	cudaMalloc((void **)&d_var_4_11, sizeof(float *));
	
	float * h_var_4_12 = (float *)malloc(sizeof(float *));
	float * d_var_4_12;
	cudaMalloc((void **)&d_var_4_12, sizeof(float *));
	
	float * h_var_4_13 = (float *)malloc(sizeof(float *));
	float * d_var_4_13;
	cudaMalloc((void **)&d_var_4_13, sizeof(float *));
	
	float * h_var_4_14 = (float *)malloc(sizeof(float *));
	float * d_var_4_14;
	cudaMalloc((void **)&d_var_4_14, sizeof(float *));
	
	float * h_var_4_15 = (float *)malloc(sizeof(float *));
	float * d_var_4_15;
	cudaMalloc((void **)&d_var_4_15, sizeof(float *));
	
	float * h_var_4_16 = (float *)malloc(sizeof(float *));
	float * d_var_4_16;
	cudaMalloc((void **)&d_var_4_16, sizeof(float *));
	
	float * h_var_4_17 = (float *)malloc(sizeof(float *));
	float * d_var_4_17;
	cudaMalloc((void **)&d_var_4_17, sizeof(float *));
	
	float * h_var_4_18 = (float *)malloc(sizeof(float *));
	float * d_var_4_18;
	cudaMalloc((void **)&d_var_4_18, sizeof(float *));
	
	float * h_var_4_19 = (float *)malloc(sizeof(float *));
	float * d_var_4_19;
	cudaMalloc((void **)&d_var_4_19, sizeof(float *));
	
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
	
	float * h_var_5_10 = (float *)malloc(sizeof(float *));
	float * d_var_5_10;
	cudaMalloc((void **)&d_var_5_10, sizeof(float *));
	
	float * h_var_5_11 = (float *)malloc(sizeof(float *));
	float * d_var_5_11;
	cudaMalloc((void **)&d_var_5_11, sizeof(float *));
	
	float * h_var_5_12 = (float *)malloc(sizeof(float *));
	float * d_var_5_12;
	cudaMalloc((void **)&d_var_5_12, sizeof(float *));
	
	float * h_var_5_13 = (float *)malloc(sizeof(float *));
	float * d_var_5_13;
	cudaMalloc((void **)&d_var_5_13, sizeof(float *));
	
	float * h_var_5_14 = (float *)malloc(sizeof(float *));
	float * d_var_5_14;
	cudaMalloc((void **)&d_var_5_14, sizeof(float *));
	
	float * h_var_5_15 = (float *)malloc(sizeof(float *));
	float * d_var_5_15;
	cudaMalloc((void **)&d_var_5_15, sizeof(float *));
	
	float * h_var_5_16 = (float *)malloc(sizeof(float *));
	float * d_var_5_16;
	cudaMalloc((void **)&d_var_5_16, sizeof(float *));
	
	float * h_var_5_17 = (float *)malloc(sizeof(float *));
	float * d_var_5_17;
	cudaMalloc((void **)&d_var_5_17, sizeof(float *));
	
	float * h_var_5_18 = (float *)malloc(sizeof(float *));
	float * d_var_5_18;
	cudaMalloc((void **)&d_var_5_18, sizeof(float *));
	
	float * h_var_5_19 = (float *)malloc(sizeof(float *));
	float * d_var_5_19;
	cudaMalloc((void **)&d_var_5_19, sizeof(float *));
	
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
	
	float * h_var_6_10 = (float *)malloc(sizeof(float *));
	float * d_var_6_10;
	cudaMalloc((void **)&d_var_6_10, sizeof(float *));
	
	float * h_var_6_11 = (float *)malloc(sizeof(float *));
	float * d_var_6_11;
	cudaMalloc((void **)&d_var_6_11, sizeof(float *));
	
	float * h_var_6_12 = (float *)malloc(sizeof(float *));
	float * d_var_6_12;
	cudaMalloc((void **)&d_var_6_12, sizeof(float *));
	
	float * h_var_6_13 = (float *)malloc(sizeof(float *));
	float * d_var_6_13;
	cudaMalloc((void **)&d_var_6_13, sizeof(float *));
	
	float * h_var_6_14 = (float *)malloc(sizeof(float *));
	float * d_var_6_14;
	cudaMalloc((void **)&d_var_6_14, sizeof(float *));
	
	float * h_var_6_15 = (float *)malloc(sizeof(float *));
	float * d_var_6_15;
	cudaMalloc((void **)&d_var_6_15, sizeof(float *));
	
	float * h_var_6_16 = (float *)malloc(sizeof(float *));
	float * d_var_6_16;
	cudaMalloc((void **)&d_var_6_16, sizeof(float *));
	
	float * h_var_6_17 = (float *)malloc(sizeof(float *));
	float * d_var_6_17;
	cudaMalloc((void **)&d_var_6_17, sizeof(float *));
	
	float * h_var_6_18 = (float *)malloc(sizeof(float *));
	float * d_var_6_18;
	cudaMalloc((void **)&d_var_6_18, sizeof(float *));
	
	float * h_var_6_19 = (float *)malloc(sizeof(float *));
	float * d_var_6_19;
	cudaMalloc((void **)&d_var_6_19, sizeof(float *));
	
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
	
	float * h_var_7_10 = (float *)malloc(sizeof(float *));
	float * d_var_7_10;
	cudaMalloc((void **)&d_var_7_10, sizeof(float *));
	
	float * h_var_7_11 = (float *)malloc(sizeof(float *));
	float * d_var_7_11;
	cudaMalloc((void **)&d_var_7_11, sizeof(float *));
	
	float * h_var_7_12 = (float *)malloc(sizeof(float *));
	float * d_var_7_12;
	cudaMalloc((void **)&d_var_7_12, sizeof(float *));
	
	float * h_var_7_13 = (float *)malloc(sizeof(float *));
	float * d_var_7_13;
	cudaMalloc((void **)&d_var_7_13, sizeof(float *));
	
	float * h_var_7_14 = (float *)malloc(sizeof(float *));
	float * d_var_7_14;
	cudaMalloc((void **)&d_var_7_14, sizeof(float *));
	
	float * h_var_7_15 = (float *)malloc(sizeof(float *));
	float * d_var_7_15;
	cudaMalloc((void **)&d_var_7_15, sizeof(float *));
	
	float * h_var_7_16 = (float *)malloc(sizeof(float *));
	float * d_var_7_16;
	cudaMalloc((void **)&d_var_7_16, sizeof(float *));
	
	float * h_var_7_17 = (float *)malloc(sizeof(float *));
	float * d_var_7_17;
	cudaMalloc((void **)&d_var_7_17, sizeof(float *));
	
	float * h_var_7_18 = (float *)malloc(sizeof(float *));
	float * d_var_7_18;
	cudaMalloc((void **)&d_var_7_18, sizeof(float *));
	
	float * h_var_7_19 = (float *)malloc(sizeof(float *));
	float * d_var_7_19;
	cudaMalloc((void **)&d_var_7_19, sizeof(float *));
	
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
	
	float * h_var_8_10 = (float *)malloc(sizeof(float *));
	float * d_var_8_10;
	cudaMalloc((void **)&d_var_8_10, sizeof(float *));
	
	float * h_var_8_11 = (float *)malloc(sizeof(float *));
	float * d_var_8_11;
	cudaMalloc((void **)&d_var_8_11, sizeof(float *));
	
	float * h_var_8_12 = (float *)malloc(sizeof(float *));
	float * d_var_8_12;
	cudaMalloc((void **)&d_var_8_12, sizeof(float *));
	
	float * h_var_8_13 = (float *)malloc(sizeof(float *));
	float * d_var_8_13;
	cudaMalloc((void **)&d_var_8_13, sizeof(float *));
	
	float * h_var_8_14 = (float *)malloc(sizeof(float *));
	float * d_var_8_14;
	cudaMalloc((void **)&d_var_8_14, sizeof(float *));
	
	float * h_var_8_15 = (float *)malloc(sizeof(float *));
	float * d_var_8_15;
	cudaMalloc((void **)&d_var_8_15, sizeof(float *));
	
	float * h_var_8_16 = (float *)malloc(sizeof(float *));
	float * d_var_8_16;
	cudaMalloc((void **)&d_var_8_16, sizeof(float *));
	
	float * h_var_8_17 = (float *)malloc(sizeof(float *));
	float * d_var_8_17;
	cudaMalloc((void **)&d_var_8_17, sizeof(float *));
	
	float * h_var_8_18 = (float *)malloc(sizeof(float *));
	float * d_var_8_18;
	cudaMalloc((void **)&d_var_8_18, sizeof(float *));
	
	float * h_var_8_19 = (float *)malloc(sizeof(float *));
	float * d_var_8_19;
	cudaMalloc((void **)&d_var_8_19, sizeof(float *));
	
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
	
	float * h_var_9_10 = (float *)malloc(sizeof(float *));
	float * d_var_9_10;
	cudaMalloc((void **)&d_var_9_10, sizeof(float *));
	
	float * h_var_9_11 = (float *)malloc(sizeof(float *));
	float * d_var_9_11;
	cudaMalloc((void **)&d_var_9_11, sizeof(float *));
	
	float * h_var_9_12 = (float *)malloc(sizeof(float *));
	float * d_var_9_12;
	cudaMalloc((void **)&d_var_9_12, sizeof(float *));
	
	float * h_var_9_13 = (float *)malloc(sizeof(float *));
	float * d_var_9_13;
	cudaMalloc((void **)&d_var_9_13, sizeof(float *));
	
	float * h_var_9_14 = (float *)malloc(sizeof(float *));
	float * d_var_9_14;
	cudaMalloc((void **)&d_var_9_14, sizeof(float *));
	
	float * h_var_9_15 = (float *)malloc(sizeof(float *));
	float * d_var_9_15;
	cudaMalloc((void **)&d_var_9_15, sizeof(float *));
	
	float * h_var_9_16 = (float *)malloc(sizeof(float *));
	float * d_var_9_16;
	cudaMalloc((void **)&d_var_9_16, sizeof(float *));
	
	float * h_var_9_17 = (float *)malloc(sizeof(float *));
	float * d_var_9_17;
	cudaMalloc((void **)&d_var_9_17, sizeof(float *));
	
	float * h_var_9_18 = (float *)malloc(sizeof(float *));
	float * d_var_9_18;
	cudaMalloc((void **)&d_var_9_18, sizeof(float *));
	
	float * h_var_9_19 = (float *)malloc(sizeof(float *));
	float * d_var_9_19;
	cudaMalloc((void **)&d_var_9_19, sizeof(float *));
	
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
	
	float * h_var_10_10 = (float *)malloc(sizeof(float *));
	float * d_var_10_10;
	cudaMalloc((void **)&d_var_10_10, sizeof(float *));
	
	float * h_var_10_11 = (float *)malloc(sizeof(float *));
	float * d_var_10_11;
	cudaMalloc((void **)&d_var_10_11, sizeof(float *));
	
	float * h_var_10_12 = (float *)malloc(sizeof(float *));
	float * d_var_10_12;
	cudaMalloc((void **)&d_var_10_12, sizeof(float *));
	
	float * h_var_10_13 = (float *)malloc(sizeof(float *));
	float * d_var_10_13;
	cudaMalloc((void **)&d_var_10_13, sizeof(float *));
	
	float * h_var_10_14 = (float *)malloc(sizeof(float *));
	float * d_var_10_14;
	cudaMalloc((void **)&d_var_10_14, sizeof(float *));
	
	float * h_var_10_15 = (float *)malloc(sizeof(float *));
	float * d_var_10_15;
	cudaMalloc((void **)&d_var_10_15, sizeof(float *));
	
	float * h_var_10_16 = (float *)malloc(sizeof(float *));
	float * d_var_10_16;
	cudaMalloc((void **)&d_var_10_16, sizeof(float *));
	
	float * h_var_10_17 = (float *)malloc(sizeof(float *));
	float * d_var_10_17;
	cudaMalloc((void **)&d_var_10_17, sizeof(float *));
	
	float * h_var_10_18 = (float *)malloc(sizeof(float *));
	float * d_var_10_18;
	cudaMalloc((void **)&d_var_10_18, sizeof(float *));
	
	float * h_var_10_19 = (float *)malloc(sizeof(float *));
	float * d_var_10_19;
	cudaMalloc((void **)&d_var_10_19, sizeof(float *));
	
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
	
	float * h_var_11_10 = (float *)malloc(sizeof(float *));
	float * d_var_11_10;
	cudaMalloc((void **)&d_var_11_10, sizeof(float *));
	
	float * h_var_11_11 = (float *)malloc(sizeof(float *));
	float * d_var_11_11;
	cudaMalloc((void **)&d_var_11_11, sizeof(float *));
	
	float * h_var_11_12 = (float *)malloc(sizeof(float *));
	float * d_var_11_12;
	cudaMalloc((void **)&d_var_11_12, sizeof(float *));
	
	float * h_var_11_13 = (float *)malloc(sizeof(float *));
	float * d_var_11_13;
	cudaMalloc((void **)&d_var_11_13, sizeof(float *));
	
	float * h_var_11_14 = (float *)malloc(sizeof(float *));
	float * d_var_11_14;
	cudaMalloc((void **)&d_var_11_14, sizeof(float *));
	
	float * h_var_11_15 = (float *)malloc(sizeof(float *));
	float * d_var_11_15;
	cudaMalloc((void **)&d_var_11_15, sizeof(float *));
	
	float * h_var_11_16 = (float *)malloc(sizeof(float *));
	float * d_var_11_16;
	cudaMalloc((void **)&d_var_11_16, sizeof(float *));
	
	float * h_var_11_17 = (float *)malloc(sizeof(float *));
	float * d_var_11_17;
	cudaMalloc((void **)&d_var_11_17, sizeof(float *));
	
	float * h_var_11_18 = (float *)malloc(sizeof(float *));
	float * d_var_11_18;
	cudaMalloc((void **)&d_var_11_18, sizeof(float *));
	
	float * h_var_11_19 = (float *)malloc(sizeof(float *));
	float * d_var_11_19;
	cudaMalloc((void **)&d_var_11_19, sizeof(float *));
	
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
	
	float * h_var_12_10 = (float *)malloc(sizeof(float *));
	float * d_var_12_10;
	cudaMalloc((void **)&d_var_12_10, sizeof(float *));
	
	float * h_var_12_11 = (float *)malloc(sizeof(float *));
	float * d_var_12_11;
	cudaMalloc((void **)&d_var_12_11, sizeof(float *));
	
	float * h_var_12_12 = (float *)malloc(sizeof(float *));
	float * d_var_12_12;
	cudaMalloc((void **)&d_var_12_12, sizeof(float *));
	
	float * h_var_12_13 = (float *)malloc(sizeof(float *));
	float * d_var_12_13;
	cudaMalloc((void **)&d_var_12_13, sizeof(float *));
	
	float * h_var_12_14 = (float *)malloc(sizeof(float *));
	float * d_var_12_14;
	cudaMalloc((void **)&d_var_12_14, sizeof(float *));
	
	float * h_var_12_15 = (float *)malloc(sizeof(float *));
	float * d_var_12_15;
	cudaMalloc((void **)&d_var_12_15, sizeof(float *));
	
	float * h_var_12_16 = (float *)malloc(sizeof(float *));
	float * d_var_12_16;
	cudaMalloc((void **)&d_var_12_16, sizeof(float *));
	
	float * h_var_12_17 = (float *)malloc(sizeof(float *));
	float * d_var_12_17;
	cudaMalloc((void **)&d_var_12_17, sizeof(float *));
	
	float * h_var_12_18 = (float *)malloc(sizeof(float *));
	float * d_var_12_18;
	cudaMalloc((void **)&d_var_12_18, sizeof(float *));
	
	float * h_var_12_19 = (float *)malloc(sizeof(float *));
	float * d_var_12_19;
	cudaMalloc((void **)&d_var_12_19, sizeof(float *));
	
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
	
	float * h_var_13_10 = (float *)malloc(sizeof(float *));
	float * d_var_13_10;
	cudaMalloc((void **)&d_var_13_10, sizeof(float *));
	
	float * h_var_13_11 = (float *)malloc(sizeof(float *));
	float * d_var_13_11;
	cudaMalloc((void **)&d_var_13_11, sizeof(float *));
	
	float * h_var_13_12 = (float *)malloc(sizeof(float *));
	float * d_var_13_12;
	cudaMalloc((void **)&d_var_13_12, sizeof(float *));
	
	float * h_var_13_13 = (float *)malloc(sizeof(float *));
	float * d_var_13_13;
	cudaMalloc((void **)&d_var_13_13, sizeof(float *));
	
	float * h_var_13_14 = (float *)malloc(sizeof(float *));
	float * d_var_13_14;
	cudaMalloc((void **)&d_var_13_14, sizeof(float *));
	
	float * h_var_13_15 = (float *)malloc(sizeof(float *));
	float * d_var_13_15;
	cudaMalloc((void **)&d_var_13_15, sizeof(float *));
	
	float * h_var_13_16 = (float *)malloc(sizeof(float *));
	float * d_var_13_16;
	cudaMalloc((void **)&d_var_13_16, sizeof(float *));
	
	float * h_var_13_17 = (float *)malloc(sizeof(float *));
	float * d_var_13_17;
	cudaMalloc((void **)&d_var_13_17, sizeof(float *));
	
	float * h_var_13_18 = (float *)malloc(sizeof(float *));
	float * d_var_13_18;
	cudaMalloc((void **)&d_var_13_18, sizeof(float *));
	
	float * h_var_13_19 = (float *)malloc(sizeof(float *));
	float * d_var_13_19;
	cudaMalloc((void **)&d_var_13_19, sizeof(float *));
	
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
	
	float * h_var_14_10 = (float *)malloc(sizeof(float *));
	float * d_var_14_10;
	cudaMalloc((void **)&d_var_14_10, sizeof(float *));
	
	float * h_var_14_11 = (float *)malloc(sizeof(float *));
	float * d_var_14_11;
	cudaMalloc((void **)&d_var_14_11, sizeof(float *));
	
	float * h_var_14_12 = (float *)malloc(sizeof(float *));
	float * d_var_14_12;
	cudaMalloc((void **)&d_var_14_12, sizeof(float *));
	
	float * h_var_14_13 = (float *)malloc(sizeof(float *));
	float * d_var_14_13;
	cudaMalloc((void **)&d_var_14_13, sizeof(float *));
	
	float * h_var_14_14 = (float *)malloc(sizeof(float *));
	float * d_var_14_14;
	cudaMalloc((void **)&d_var_14_14, sizeof(float *));
	
	float * h_var_14_15 = (float *)malloc(sizeof(float *));
	float * d_var_14_15;
	cudaMalloc((void **)&d_var_14_15, sizeof(float *));
	
	float * h_var_14_16 = (float *)malloc(sizeof(float *));
	float * d_var_14_16;
	cudaMalloc((void **)&d_var_14_16, sizeof(float *));
	
	float * h_var_14_17 = (float *)malloc(sizeof(float *));
	float * d_var_14_17;
	cudaMalloc((void **)&d_var_14_17, sizeof(float *));
	
	float * h_var_14_18 = (float *)malloc(sizeof(float *));
	float * d_var_14_18;
	cudaMalloc((void **)&d_var_14_18, sizeof(float *));
	
	float * h_var_14_19 = (float *)malloc(sizeof(float *));
	float * d_var_14_19;
	cudaMalloc((void **)&d_var_14_19, sizeof(float *));
	
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
	
	float * h_var_15_10 = (float *)malloc(sizeof(float *));
	float * d_var_15_10;
	cudaMalloc((void **)&d_var_15_10, sizeof(float *));
	
	float * h_var_15_11 = (float *)malloc(sizeof(float *));
	float * d_var_15_11;
	cudaMalloc((void **)&d_var_15_11, sizeof(float *));
	
	float * h_var_15_12 = (float *)malloc(sizeof(float *));
	float * d_var_15_12;
	cudaMalloc((void **)&d_var_15_12, sizeof(float *));
	
	float * h_var_15_13 = (float *)malloc(sizeof(float *));
	float * d_var_15_13;
	cudaMalloc((void **)&d_var_15_13, sizeof(float *));
	
	float * h_var_15_14 = (float *)malloc(sizeof(float *));
	float * d_var_15_14;
	cudaMalloc((void **)&d_var_15_14, sizeof(float *));
	
	float * h_var_15_15 = (float *)malloc(sizeof(float *));
	float * d_var_15_15;
	cudaMalloc((void **)&d_var_15_15, sizeof(float *));
	
	float * h_var_15_16 = (float *)malloc(sizeof(float *));
	float * d_var_15_16;
	cudaMalloc((void **)&d_var_15_16, sizeof(float *));
	
	float * h_var_15_17 = (float *)malloc(sizeof(float *));
	float * d_var_15_17;
	cudaMalloc((void **)&d_var_15_17, sizeof(float *));
	
	float * h_var_15_18 = (float *)malloc(sizeof(float *));
	float * d_var_15_18;
	cudaMalloc((void **)&d_var_15_18, sizeof(float *));
	
	float * h_var_15_19 = (float *)malloc(sizeof(float *));
	float * d_var_15_19;
	cudaMalloc((void **)&d_var_15_19, sizeof(float *));
	
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
	
	float * h_var_16_10 = (float *)malloc(sizeof(float *));
	float * d_var_16_10;
	cudaMalloc((void **)&d_var_16_10, sizeof(float *));
	
	float * h_var_16_11 = (float *)malloc(sizeof(float *));
	float * d_var_16_11;
	cudaMalloc((void **)&d_var_16_11, sizeof(float *));
	
	float * h_var_16_12 = (float *)malloc(sizeof(float *));
	float * d_var_16_12;
	cudaMalloc((void **)&d_var_16_12, sizeof(float *));
	
	float * h_var_16_13 = (float *)malloc(sizeof(float *));
	float * d_var_16_13;
	cudaMalloc((void **)&d_var_16_13, sizeof(float *));
	
	float * h_var_16_14 = (float *)malloc(sizeof(float *));
	float * d_var_16_14;
	cudaMalloc((void **)&d_var_16_14, sizeof(float *));
	
	float * h_var_16_15 = (float *)malloc(sizeof(float *));
	float * d_var_16_15;
	cudaMalloc((void **)&d_var_16_15, sizeof(float *));
	
	float * h_var_16_16 = (float *)malloc(sizeof(float *));
	float * d_var_16_16;
	cudaMalloc((void **)&d_var_16_16, sizeof(float *));
	
	float * h_var_16_17 = (float *)malloc(sizeof(float *));
	float * d_var_16_17;
	cudaMalloc((void **)&d_var_16_17, sizeof(float *));
	
	float * h_var_16_18 = (float *)malloc(sizeof(float *));
	float * d_var_16_18;
	cudaMalloc((void **)&d_var_16_18, sizeof(float *));
	
	float * h_var_16_19 = (float *)malloc(sizeof(float *));
	float * d_var_16_19;
	cudaMalloc((void **)&d_var_16_19, sizeof(float *));
	
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
	
	float * h_var_17_10 = (float *)malloc(sizeof(float *));
	float * d_var_17_10;
	cudaMalloc((void **)&d_var_17_10, sizeof(float *));
	
	float * h_var_17_11 = (float *)malloc(sizeof(float *));
	float * d_var_17_11;
	cudaMalloc((void **)&d_var_17_11, sizeof(float *));
	
	float * h_var_17_12 = (float *)malloc(sizeof(float *));
	float * d_var_17_12;
	cudaMalloc((void **)&d_var_17_12, sizeof(float *));
	
	float * h_var_17_13 = (float *)malloc(sizeof(float *));
	float * d_var_17_13;
	cudaMalloc((void **)&d_var_17_13, sizeof(float *));
	
	float * h_var_17_14 = (float *)malloc(sizeof(float *));
	float * d_var_17_14;
	cudaMalloc((void **)&d_var_17_14, sizeof(float *));
	
	float * h_var_17_15 = (float *)malloc(sizeof(float *));
	float * d_var_17_15;
	cudaMalloc((void **)&d_var_17_15, sizeof(float *));
	
	float * h_var_17_16 = (float *)malloc(sizeof(float *));
	float * d_var_17_16;
	cudaMalloc((void **)&d_var_17_16, sizeof(float *));
	
	float * h_var_17_17 = (float *)malloc(sizeof(float *));
	float * d_var_17_17;
	cudaMalloc((void **)&d_var_17_17, sizeof(float *));
	
	float * h_var_17_18 = (float *)malloc(sizeof(float *));
	float * d_var_17_18;
	cudaMalloc((void **)&d_var_17_18, sizeof(float *));
	
	float * h_var_17_19 = (float *)malloc(sizeof(float *));
	float * d_var_17_19;
	cudaMalloc((void **)&d_var_17_19, sizeof(float *));
	
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
	
	float * h_var_18_10 = (float *)malloc(sizeof(float *));
	float * d_var_18_10;
	cudaMalloc((void **)&d_var_18_10, sizeof(float *));
	
	float * h_var_18_11 = (float *)malloc(sizeof(float *));
	float * d_var_18_11;
	cudaMalloc((void **)&d_var_18_11, sizeof(float *));
	
	float * h_var_18_12 = (float *)malloc(sizeof(float *));
	float * d_var_18_12;
	cudaMalloc((void **)&d_var_18_12, sizeof(float *));
	
	float * h_var_18_13 = (float *)malloc(sizeof(float *));
	float * d_var_18_13;
	cudaMalloc((void **)&d_var_18_13, sizeof(float *));
	
	float * h_var_18_14 = (float *)malloc(sizeof(float *));
	float * d_var_18_14;
	cudaMalloc((void **)&d_var_18_14, sizeof(float *));
	
	float * h_var_18_15 = (float *)malloc(sizeof(float *));
	float * d_var_18_15;
	cudaMalloc((void **)&d_var_18_15, sizeof(float *));
	
	float * h_var_18_16 = (float *)malloc(sizeof(float *));
	float * d_var_18_16;
	cudaMalloc((void **)&d_var_18_16, sizeof(float *));
	
	float * h_var_18_17 = (float *)malloc(sizeof(float *));
	float * d_var_18_17;
	cudaMalloc((void **)&d_var_18_17, sizeof(float *));
	
	float * h_var_18_18 = (float *)malloc(sizeof(float *));
	float * d_var_18_18;
	cudaMalloc((void **)&d_var_18_18, sizeof(float *));
	
	float * h_var_18_19 = (float *)malloc(sizeof(float *));
	float * d_var_18_19;
	cudaMalloc((void **)&d_var_18_19, sizeof(float *));
	
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
	
	float * h_var_19_10 = (float *)malloc(sizeof(float *));
	float * d_var_19_10;
	cudaMalloc((void **)&d_var_19_10, sizeof(float *));
	
	float * h_var_19_11 = (float *)malloc(sizeof(float *));
	float * d_var_19_11;
	cudaMalloc((void **)&d_var_19_11, sizeof(float *));
	
	float * h_var_19_12 = (float *)malloc(sizeof(float *));
	float * d_var_19_12;
	cudaMalloc((void **)&d_var_19_12, sizeof(float *));
	
	float * h_var_19_13 = (float *)malloc(sizeof(float *));
	float * d_var_19_13;
	cudaMalloc((void **)&d_var_19_13, sizeof(float *));
	
	float * h_var_19_14 = (float *)malloc(sizeof(float *));
	float * d_var_19_14;
	cudaMalloc((void **)&d_var_19_14, sizeof(float *));
	
	float * h_var_19_15 = (float *)malloc(sizeof(float *));
	float * d_var_19_15;
	cudaMalloc((void **)&d_var_19_15, sizeof(float *));
	
	float * h_var_19_16 = (float *)malloc(sizeof(float *));
	float * d_var_19_16;
	cudaMalloc((void **)&d_var_19_16, sizeof(float *));
	
	float * h_var_19_17 = (float *)malloc(sizeof(float *));
	float * d_var_19_17;
	cudaMalloc((void **)&d_var_19_17, sizeof(float *));
	
	float * h_var_19_18 = (float *)malloc(sizeof(float *));
	float * d_var_19_18;
	cudaMalloc((void **)&d_var_19_18, sizeof(float *));
	
	float * h_var_19_19 = (float *)malloc(sizeof(float *));
	float * d_var_19_19;
	cudaMalloc((void **)&d_var_19_19, sizeof(float *));
	
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
	
	float * h_var_20_10 = (float *)malloc(sizeof(float *));
	float * d_var_20_10;
	cudaMalloc((void **)&d_var_20_10, sizeof(float *));
	
	float * h_var_20_11 = (float *)malloc(sizeof(float *));
	float * d_var_20_11;
	cudaMalloc((void **)&d_var_20_11, sizeof(float *));
	
	float * h_var_20_12 = (float *)malloc(sizeof(float *));
	float * d_var_20_12;
	cudaMalloc((void **)&d_var_20_12, sizeof(float *));
	
	float * h_var_20_13 = (float *)malloc(sizeof(float *));
	float * d_var_20_13;
	cudaMalloc((void **)&d_var_20_13, sizeof(float *));
	
	float * h_var_20_14 = (float *)malloc(sizeof(float *));
	float * d_var_20_14;
	cudaMalloc((void **)&d_var_20_14, sizeof(float *));
	
	float * h_var_20_15 = (float *)malloc(sizeof(float *));
	float * d_var_20_15;
	cudaMalloc((void **)&d_var_20_15, sizeof(float *));
	
	float * h_var_20_16 = (float *)malloc(sizeof(float *));
	float * d_var_20_16;
	cudaMalloc((void **)&d_var_20_16, sizeof(float *));
	
	float * h_var_20_17 = (float *)malloc(sizeof(float *));
	float * d_var_20_17;
	cudaMalloc((void **)&d_var_20_17, sizeof(float *));
	
	float * h_var_20_18 = (float *)malloc(sizeof(float *));
	float * d_var_20_18;
	cudaMalloc((void **)&d_var_20_18, sizeof(float *));
	
	float * h_var_20_19 = (float *)malloc(sizeof(float *));
	float * d_var_20_19;
	cudaMalloc((void **)&d_var_20_19, sizeof(float *));
	
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
	
	float * h_var_21_10 = (float *)malloc(sizeof(float *));
	float * d_var_21_10;
	cudaMalloc((void **)&d_var_21_10, sizeof(float *));
	
	float * h_var_21_11 = (float *)malloc(sizeof(float *));
	float * d_var_21_11;
	cudaMalloc((void **)&d_var_21_11, sizeof(float *));
	
	float * h_var_21_12 = (float *)malloc(sizeof(float *));
	float * d_var_21_12;
	cudaMalloc((void **)&d_var_21_12, sizeof(float *));
	
	float * h_var_21_13 = (float *)malloc(sizeof(float *));
	float * d_var_21_13;
	cudaMalloc((void **)&d_var_21_13, sizeof(float *));
	
	float * h_var_21_14 = (float *)malloc(sizeof(float *));
	float * d_var_21_14;
	cudaMalloc((void **)&d_var_21_14, sizeof(float *));
	
	float * h_var_21_15 = (float *)malloc(sizeof(float *));
	float * d_var_21_15;
	cudaMalloc((void **)&d_var_21_15, sizeof(float *));
	
	float * h_var_21_16 = (float *)malloc(sizeof(float *));
	float * d_var_21_16;
	cudaMalloc((void **)&d_var_21_16, sizeof(float *));
	
	float * h_var_21_17 = (float *)malloc(sizeof(float *));
	float * d_var_21_17;
	cudaMalloc((void **)&d_var_21_17, sizeof(float *));
	
	float * h_var_21_18 = (float *)malloc(sizeof(float *));
	float * d_var_21_18;
	cudaMalloc((void **)&d_var_21_18, sizeof(float *));
	
	float * h_var_21_19 = (float *)malloc(sizeof(float *));
	float * d_var_21_19;
	cudaMalloc((void **)&d_var_21_19, sizeof(float *));
	
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
	
	float * h_var_22_10 = (float *)malloc(sizeof(float *));
	float * d_var_22_10;
	cudaMalloc((void **)&d_var_22_10, sizeof(float *));
	
	float * h_var_22_11 = (float *)malloc(sizeof(float *));
	float * d_var_22_11;
	cudaMalloc((void **)&d_var_22_11, sizeof(float *));
	
	float * h_var_22_12 = (float *)malloc(sizeof(float *));
	float * d_var_22_12;
	cudaMalloc((void **)&d_var_22_12, sizeof(float *));
	
	float * h_var_22_13 = (float *)malloc(sizeof(float *));
	float * d_var_22_13;
	cudaMalloc((void **)&d_var_22_13, sizeof(float *));
	
	float * h_var_22_14 = (float *)malloc(sizeof(float *));
	float * d_var_22_14;
	cudaMalloc((void **)&d_var_22_14, sizeof(float *));
	
	float * h_var_22_15 = (float *)malloc(sizeof(float *));
	float * d_var_22_15;
	cudaMalloc((void **)&d_var_22_15, sizeof(float *));
	
	float * h_var_22_16 = (float *)malloc(sizeof(float *));
	float * d_var_22_16;
	cudaMalloc((void **)&d_var_22_16, sizeof(float *));
	
	float * h_var_22_17 = (float *)malloc(sizeof(float *));
	float * d_var_22_17;
	cudaMalloc((void **)&d_var_22_17, sizeof(float *));
	
	float * h_var_22_18 = (float *)malloc(sizeof(float *));
	float * d_var_22_18;
	cudaMalloc((void **)&d_var_22_18, sizeof(float *));
	
	float * h_var_22_19 = (float *)malloc(sizeof(float *));
	float * d_var_22_19;
	cudaMalloc((void **)&d_var_22_19, sizeof(float *));
	
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
	
	float * h_var_23_10 = (float *)malloc(sizeof(float *));
	float * d_var_23_10;
	cudaMalloc((void **)&d_var_23_10, sizeof(float *));
	
	float * h_var_23_11 = (float *)malloc(sizeof(float *));
	float * d_var_23_11;
	cudaMalloc((void **)&d_var_23_11, sizeof(float *));
	
	float * h_var_23_12 = (float *)malloc(sizeof(float *));
	float * d_var_23_12;
	cudaMalloc((void **)&d_var_23_12, sizeof(float *));
	
	float * h_var_23_13 = (float *)malloc(sizeof(float *));
	float * d_var_23_13;
	cudaMalloc((void **)&d_var_23_13, sizeof(float *));
	
	float * h_var_23_14 = (float *)malloc(sizeof(float *));
	float * d_var_23_14;
	cudaMalloc((void **)&d_var_23_14, sizeof(float *));
	
	float * h_var_23_15 = (float *)malloc(sizeof(float *));
	float * d_var_23_15;
	cudaMalloc((void **)&d_var_23_15, sizeof(float *));
	
	float * h_var_23_16 = (float *)malloc(sizeof(float *));
	float * d_var_23_16;
	cudaMalloc((void **)&d_var_23_16, sizeof(float *));
	
	float * h_var_23_17 = (float *)malloc(sizeof(float *));
	float * d_var_23_17;
	cudaMalloc((void **)&d_var_23_17, sizeof(float *));
	
	float * h_var_23_18 = (float *)malloc(sizeof(float *));
	float * d_var_23_18;
	cudaMalloc((void **)&d_var_23_18, sizeof(float *));
	
	float * h_var_23_19 = (float *)malloc(sizeof(float *));
	float * d_var_23_19;
	cudaMalloc((void **)&d_var_23_19, sizeof(float *));
	
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
	
	float * h_var_24_10 = (float *)malloc(sizeof(float *));
	float * d_var_24_10;
	cudaMalloc((void **)&d_var_24_10, sizeof(float *));
	
	float * h_var_24_11 = (float *)malloc(sizeof(float *));
	float * d_var_24_11;
	cudaMalloc((void **)&d_var_24_11, sizeof(float *));
	
	float * h_var_24_12 = (float *)malloc(sizeof(float *));
	float * d_var_24_12;
	cudaMalloc((void **)&d_var_24_12, sizeof(float *));
	
	float * h_var_24_13 = (float *)malloc(sizeof(float *));
	float * d_var_24_13;
	cudaMalloc((void **)&d_var_24_13, sizeof(float *));
	
	float * h_var_24_14 = (float *)malloc(sizeof(float *));
	float * d_var_24_14;
	cudaMalloc((void **)&d_var_24_14, sizeof(float *));
	
	float * h_var_24_15 = (float *)malloc(sizeof(float *));
	float * d_var_24_15;
	cudaMalloc((void **)&d_var_24_15, sizeof(float *));
	
	float * h_var_24_16 = (float *)malloc(sizeof(float *));
	float * d_var_24_16;
	cudaMalloc((void **)&d_var_24_16, sizeof(float *));
	
	float * h_var_24_17 = (float *)malloc(sizeof(float *));
	float * d_var_24_17;
	cudaMalloc((void **)&d_var_24_17, sizeof(float *));
	
	float * h_var_24_18 = (float *)malloc(sizeof(float *));
	float * d_var_24_18;
	cudaMalloc((void **)&d_var_24_18, sizeof(float *));
	
	float * h_var_24_19 = (float *)malloc(sizeof(float *));
	float * d_var_24_19;
	cudaMalloc((void **)&d_var_24_19, sizeof(float *));
	
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
	
	float * h_var_25_10 = (float *)malloc(sizeof(float *));
	float * d_var_25_10;
	cudaMalloc((void **)&d_var_25_10, sizeof(float *));
	
	float * h_var_25_11 = (float *)malloc(sizeof(float *));
	float * d_var_25_11;
	cudaMalloc((void **)&d_var_25_11, sizeof(float *));
	
	float * h_var_25_12 = (float *)malloc(sizeof(float *));
	float * d_var_25_12;
	cudaMalloc((void **)&d_var_25_12, sizeof(float *));
	
	float * h_var_25_13 = (float *)malloc(sizeof(float *));
	float * d_var_25_13;
	cudaMalloc((void **)&d_var_25_13, sizeof(float *));
	
	float * h_var_25_14 = (float *)malloc(sizeof(float *));
	float * d_var_25_14;
	cudaMalloc((void **)&d_var_25_14, sizeof(float *));
	
	float * h_var_25_15 = (float *)malloc(sizeof(float *));
	float * d_var_25_15;
	cudaMalloc((void **)&d_var_25_15, sizeof(float *));
	
	float * h_var_25_16 = (float *)malloc(sizeof(float *));
	float * d_var_25_16;
	cudaMalloc((void **)&d_var_25_16, sizeof(float *));
	
	float * h_var_25_17 = (float *)malloc(sizeof(float *));
	float * d_var_25_17;
	cudaMalloc((void **)&d_var_25_17, sizeof(float *));
	
	float * h_var_25_18 = (float *)malloc(sizeof(float *));
	float * d_var_25_18;
	cudaMalloc((void **)&d_var_25_18, sizeof(float *));
	
	float * h_var_25_19 = (float *)malloc(sizeof(float *));
	float * d_var_25_19;
	cudaMalloc((void **)&d_var_25_19, sizeof(float *));
	
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
	
	float * h_var_26_10 = (float *)malloc(sizeof(float *));
	float * d_var_26_10;
	cudaMalloc((void **)&d_var_26_10, sizeof(float *));
	
	float * h_var_26_11 = (float *)malloc(sizeof(float *));
	float * d_var_26_11;
	cudaMalloc((void **)&d_var_26_11, sizeof(float *));
	
	float * h_var_26_12 = (float *)malloc(sizeof(float *));
	float * d_var_26_12;
	cudaMalloc((void **)&d_var_26_12, sizeof(float *));
	
	float * h_var_26_13 = (float *)malloc(sizeof(float *));
	float * d_var_26_13;
	cudaMalloc((void **)&d_var_26_13, sizeof(float *));
	
	float * h_var_26_14 = (float *)malloc(sizeof(float *));
	float * d_var_26_14;
	cudaMalloc((void **)&d_var_26_14, sizeof(float *));
	
	float * h_var_26_15 = (float *)malloc(sizeof(float *));
	float * d_var_26_15;
	cudaMalloc((void **)&d_var_26_15, sizeof(float *));
	
	float * h_var_26_16 = (float *)malloc(sizeof(float *));
	float * d_var_26_16;
	cudaMalloc((void **)&d_var_26_16, sizeof(float *));
	
	float * h_var_26_17 = (float *)malloc(sizeof(float *));
	float * d_var_26_17;
	cudaMalloc((void **)&d_var_26_17, sizeof(float *));
	
	float * h_var_26_18 = (float *)malloc(sizeof(float *));
	float * d_var_26_18;
	cudaMalloc((void **)&d_var_26_18, sizeof(float *));
	
	float * h_var_26_19 = (float *)malloc(sizeof(float *));
	float * d_var_26_19;
	cudaMalloc((void **)&d_var_26_19, sizeof(float *));
	
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
	
	float * h_var_27_10 = (float *)malloc(sizeof(float *));
	float * d_var_27_10;
	cudaMalloc((void **)&d_var_27_10, sizeof(float *));
	
	float * h_var_27_11 = (float *)malloc(sizeof(float *));
	float * d_var_27_11;
	cudaMalloc((void **)&d_var_27_11, sizeof(float *));
	
	float * h_var_27_12 = (float *)malloc(sizeof(float *));
	float * d_var_27_12;
	cudaMalloc((void **)&d_var_27_12, sizeof(float *));
	
	float * h_var_27_13 = (float *)malloc(sizeof(float *));
	float * d_var_27_13;
	cudaMalloc((void **)&d_var_27_13, sizeof(float *));
	
	float * h_var_27_14 = (float *)malloc(sizeof(float *));
	float * d_var_27_14;
	cudaMalloc((void **)&d_var_27_14, sizeof(float *));
	
	float * h_var_27_15 = (float *)malloc(sizeof(float *));
	float * d_var_27_15;
	cudaMalloc((void **)&d_var_27_15, sizeof(float *));
	
	float * h_var_27_16 = (float *)malloc(sizeof(float *));
	float * d_var_27_16;
	cudaMalloc((void **)&d_var_27_16, sizeof(float *));
	
	float * h_var_27_17 = (float *)malloc(sizeof(float *));
	float * d_var_27_17;
	cudaMalloc((void **)&d_var_27_17, sizeof(float *));
	
	float * h_var_27_18 = (float *)malloc(sizeof(float *));
	float * d_var_27_18;
	cudaMalloc((void **)&d_var_27_18, sizeof(float *));
	
	float * h_var_27_19 = (float *)malloc(sizeof(float *));
	float * d_var_27_19;
	cudaMalloc((void **)&d_var_27_19, sizeof(float *));
	
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
	
	float * h_var_28_10 = (float *)malloc(sizeof(float *));
	float * d_var_28_10;
	cudaMalloc((void **)&d_var_28_10, sizeof(float *));
	
	float * h_var_28_11 = (float *)malloc(sizeof(float *));
	float * d_var_28_11;
	cudaMalloc((void **)&d_var_28_11, sizeof(float *));
	
	float * h_var_28_12 = (float *)malloc(sizeof(float *));
	float * d_var_28_12;
	cudaMalloc((void **)&d_var_28_12, sizeof(float *));
	
	float * h_var_28_13 = (float *)malloc(sizeof(float *));
	float * d_var_28_13;
	cudaMalloc((void **)&d_var_28_13, sizeof(float *));
	
	float * h_var_28_14 = (float *)malloc(sizeof(float *));
	float * d_var_28_14;
	cudaMalloc((void **)&d_var_28_14, sizeof(float *));
	
	float * h_var_28_15 = (float *)malloc(sizeof(float *));
	float * d_var_28_15;
	cudaMalloc((void **)&d_var_28_15, sizeof(float *));
	
	float * h_var_28_16 = (float *)malloc(sizeof(float *));
	float * d_var_28_16;
	cudaMalloc((void **)&d_var_28_16, sizeof(float *));
	
	float * h_var_28_17 = (float *)malloc(sizeof(float *));
	float * d_var_28_17;
	cudaMalloc((void **)&d_var_28_17, sizeof(float *));
	
	float * h_var_28_18 = (float *)malloc(sizeof(float *));
	float * d_var_28_18;
	cudaMalloc((void **)&d_var_28_18, sizeof(float *));
	
	float * h_var_28_19 = (float *)malloc(sizeof(float *));
	float * d_var_28_19;
	cudaMalloc((void **)&d_var_28_19, sizeof(float *));
	
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
	
	float * h_var_29_10 = (float *)malloc(sizeof(float *));
	float * d_var_29_10;
	cudaMalloc((void **)&d_var_29_10, sizeof(float *));
	
	float * h_var_29_11 = (float *)malloc(sizeof(float *));
	float * d_var_29_11;
	cudaMalloc((void **)&d_var_29_11, sizeof(float *));
	
	float * h_var_29_12 = (float *)malloc(sizeof(float *));
	float * d_var_29_12;
	cudaMalloc((void **)&d_var_29_12, sizeof(float *));
	
	float * h_var_29_13 = (float *)malloc(sizeof(float *));
	float * d_var_29_13;
	cudaMalloc((void **)&d_var_29_13, sizeof(float *));
	
	float * h_var_29_14 = (float *)malloc(sizeof(float *));
	float * d_var_29_14;
	cudaMalloc((void **)&d_var_29_14, sizeof(float *));
	
	float * h_var_29_15 = (float *)malloc(sizeof(float *));
	float * d_var_29_15;
	cudaMalloc((void **)&d_var_29_15, sizeof(float *));
	
	float * h_var_29_16 = (float *)malloc(sizeof(float *));
	float * d_var_29_16;
	cudaMalloc((void **)&d_var_29_16, sizeof(float *));
	
	float * h_var_29_17 = (float *)malloc(sizeof(float *));
	float * d_var_29_17;
	cudaMalloc((void **)&d_var_29_17, sizeof(float *));
	
	float * h_var_29_18 = (float *)malloc(sizeof(float *));
	float * d_var_29_18;
	cudaMalloc((void **)&d_var_29_18, sizeof(float *));
	
	float * h_var_29_19 = (float *)malloc(sizeof(float *));
	float * d_var_29_19;
	cudaMalloc((void **)&d_var_29_19, sizeof(float *));
	
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
	
	float * h_var_30_10 = (float *)malloc(sizeof(float *));
	float * d_var_30_10;
	cudaMalloc((void **)&d_var_30_10, sizeof(float *));
	
	float * h_var_30_11 = (float *)malloc(sizeof(float *));
	float * d_var_30_11;
	cudaMalloc((void **)&d_var_30_11, sizeof(float *));
	
	float * h_var_30_12 = (float *)malloc(sizeof(float *));
	float * d_var_30_12;
	cudaMalloc((void **)&d_var_30_12, sizeof(float *));
	
	float * h_var_30_13 = (float *)malloc(sizeof(float *));
	float * d_var_30_13;
	cudaMalloc((void **)&d_var_30_13, sizeof(float *));
	
	float * h_var_30_14 = (float *)malloc(sizeof(float *));
	float * d_var_30_14;
	cudaMalloc((void **)&d_var_30_14, sizeof(float *));
	
	float * h_var_30_15 = (float *)malloc(sizeof(float *));
	float * d_var_30_15;
	cudaMalloc((void **)&d_var_30_15, sizeof(float *));
	
	float * h_var_30_16 = (float *)malloc(sizeof(float *));
	float * d_var_30_16;
	cudaMalloc((void **)&d_var_30_16, sizeof(float *));
	
	float * h_var_30_17 = (float *)malloc(sizeof(float *));
	float * d_var_30_17;
	cudaMalloc((void **)&d_var_30_17, sizeof(float *));
	
	float * h_var_30_18 = (float *)malloc(sizeof(float *));
	float * d_var_30_18;
	cudaMalloc((void **)&d_var_30_18, sizeof(float *));
	
	float * h_var_30_19 = (float *)malloc(sizeof(float *));
	float * d_var_30_19;
	cudaMalloc((void **)&d_var_30_19, sizeof(float *));
	
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
	
	float * h_var_31_10 = (float *)malloc(sizeof(float *));
	float * d_var_31_10;
	cudaMalloc((void **)&d_var_31_10, sizeof(float *));
	
	float * h_var_31_11 = (float *)malloc(sizeof(float *));
	float * d_var_31_11;
	cudaMalloc((void **)&d_var_31_11, sizeof(float *));
	
	float * h_var_31_12 = (float *)malloc(sizeof(float *));
	float * d_var_31_12;
	cudaMalloc((void **)&d_var_31_12, sizeof(float *));
	
	float * h_var_31_13 = (float *)malloc(sizeof(float *));
	float * d_var_31_13;
	cudaMalloc((void **)&d_var_31_13, sizeof(float *));
	
	float * h_var_31_14 = (float *)malloc(sizeof(float *));
	float * d_var_31_14;
	cudaMalloc((void **)&d_var_31_14, sizeof(float *));
	
	float * h_var_31_15 = (float *)malloc(sizeof(float *));
	float * d_var_31_15;
	cudaMalloc((void **)&d_var_31_15, sizeof(float *));
	
	float * h_var_31_16 = (float *)malloc(sizeof(float *));
	float * d_var_31_16;
	cudaMalloc((void **)&d_var_31_16, sizeof(float *));
	
	float * h_var_31_17 = (float *)malloc(sizeof(float *));
	float * d_var_31_17;
	cudaMalloc((void **)&d_var_31_17, sizeof(float *));
	
	float * h_var_31_18 = (float *)malloc(sizeof(float *));
	float * d_var_31_18;
	cudaMalloc((void **)&d_var_31_18, sizeof(float *));
	
	float * h_var_31_19 = (float *)malloc(sizeof(float *));
	float * d_var_31_19;
	cudaMalloc((void **)&d_var_31_19, sizeof(float *));
	
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
	
	float * h_var_32_10 = (float *)malloc(sizeof(float *));
	float * d_var_32_10;
	cudaMalloc((void **)&d_var_32_10, sizeof(float *));
	
	float * h_var_32_11 = (float *)malloc(sizeof(float *));
	float * d_var_32_11;
	cudaMalloc((void **)&d_var_32_11, sizeof(float *));
	
	float * h_var_32_12 = (float *)malloc(sizeof(float *));
	float * d_var_32_12;
	cudaMalloc((void **)&d_var_32_12, sizeof(float *));
	
	float * h_var_32_13 = (float *)malloc(sizeof(float *));
	float * d_var_32_13;
	cudaMalloc((void **)&d_var_32_13, sizeof(float *));
	
	float * h_var_32_14 = (float *)malloc(sizeof(float *));
	float * d_var_32_14;
	cudaMalloc((void **)&d_var_32_14, sizeof(float *));
	
	float * h_var_32_15 = (float *)malloc(sizeof(float *));
	float * d_var_32_15;
	cudaMalloc((void **)&d_var_32_15, sizeof(float *));
	
	float * h_var_32_16 = (float *)malloc(sizeof(float *));
	float * d_var_32_16;
	cudaMalloc((void **)&d_var_32_16, sizeof(float *));
	
	float * h_var_32_17 = (float *)malloc(sizeof(float *));
	float * d_var_32_17;
	cudaMalloc((void **)&d_var_32_17, sizeof(float *));
	
	float * h_var_32_18 = (float *)malloc(sizeof(float *));
	float * d_var_32_18;
	cudaMalloc((void **)&d_var_32_18, sizeof(float *));
	
	float * h_var_32_19 = (float *)malloc(sizeof(float *));
	float * d_var_32_19;
	cudaMalloc((void **)&d_var_32_19, sizeof(float *));
	
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
	
	float * h_var_33_10 = (float *)malloc(sizeof(float *));
	float * d_var_33_10;
	cudaMalloc((void **)&d_var_33_10, sizeof(float *));
	
	float * h_var_33_11 = (float *)malloc(sizeof(float *));
	float * d_var_33_11;
	cudaMalloc((void **)&d_var_33_11, sizeof(float *));
	
	float * h_var_33_12 = (float *)malloc(sizeof(float *));
	float * d_var_33_12;
	cudaMalloc((void **)&d_var_33_12, sizeof(float *));
	
	float * h_var_33_13 = (float *)malloc(sizeof(float *));
	float * d_var_33_13;
	cudaMalloc((void **)&d_var_33_13, sizeof(float *));
	
	float * h_var_33_14 = (float *)malloc(sizeof(float *));
	float * d_var_33_14;
	cudaMalloc((void **)&d_var_33_14, sizeof(float *));
	
	float * h_var_33_15 = (float *)malloc(sizeof(float *));
	float * d_var_33_15;
	cudaMalloc((void **)&d_var_33_15, sizeof(float *));
	
	float * h_var_33_16 = (float *)malloc(sizeof(float *));
	float * d_var_33_16;
	cudaMalloc((void **)&d_var_33_16, sizeof(float *));
	
	float * h_var_33_17 = (float *)malloc(sizeof(float *));
	float * d_var_33_17;
	cudaMalloc((void **)&d_var_33_17, sizeof(float *));
	
	float * h_var_33_18 = (float *)malloc(sizeof(float *));
	float * d_var_33_18;
	cudaMalloc((void **)&d_var_33_18, sizeof(float *));
	
	float * h_var_33_19 = (float *)malloc(sizeof(float *));
	float * d_var_33_19;
	cudaMalloc((void **)&d_var_33_19, sizeof(float *));
	
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
	
	float * h_var_34_10 = (float *)malloc(sizeof(float *));
	float * d_var_34_10;
	cudaMalloc((void **)&d_var_34_10, sizeof(float *));
	
	float * h_var_34_11 = (float *)malloc(sizeof(float *));
	float * d_var_34_11;
	cudaMalloc((void **)&d_var_34_11, sizeof(float *));
	
	float * h_var_34_12 = (float *)malloc(sizeof(float *));
	float * d_var_34_12;
	cudaMalloc((void **)&d_var_34_12, sizeof(float *));
	
	float * h_var_34_13 = (float *)malloc(sizeof(float *));
	float * d_var_34_13;
	cudaMalloc((void **)&d_var_34_13, sizeof(float *));
	
	float * h_var_34_14 = (float *)malloc(sizeof(float *));
	float * d_var_34_14;
	cudaMalloc((void **)&d_var_34_14, sizeof(float *));
	
	float * h_var_34_15 = (float *)malloc(sizeof(float *));
	float * d_var_34_15;
	cudaMalloc((void **)&d_var_34_15, sizeof(float *));
	
	float * h_var_34_16 = (float *)malloc(sizeof(float *));
	float * d_var_34_16;
	cudaMalloc((void **)&d_var_34_16, sizeof(float *));
	
	float * h_var_34_17 = (float *)malloc(sizeof(float *));
	float * d_var_34_17;
	cudaMalloc((void **)&d_var_34_17, sizeof(float *));
	
	float * h_var_34_18 = (float *)malloc(sizeof(float *));
	float * d_var_34_18;
	cudaMalloc((void **)&d_var_34_18, sizeof(float *));
	
	float * h_var_34_19 = (float *)malloc(sizeof(float *));
	float * d_var_34_19;
	cudaMalloc((void **)&d_var_34_19, sizeof(float *));
	
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
	
	float * h_var_35_10 = (float *)malloc(sizeof(float *));
	float * d_var_35_10;
	cudaMalloc((void **)&d_var_35_10, sizeof(float *));
	
	float * h_var_35_11 = (float *)malloc(sizeof(float *));
	float * d_var_35_11;
	cudaMalloc((void **)&d_var_35_11, sizeof(float *));
	
	float * h_var_35_12 = (float *)malloc(sizeof(float *));
	float * d_var_35_12;
	cudaMalloc((void **)&d_var_35_12, sizeof(float *));
	
	float * h_var_35_13 = (float *)malloc(sizeof(float *));
	float * d_var_35_13;
	cudaMalloc((void **)&d_var_35_13, sizeof(float *));
	
	float * h_var_35_14 = (float *)malloc(sizeof(float *));
	float * d_var_35_14;
	cudaMalloc((void **)&d_var_35_14, sizeof(float *));
	
	float * h_var_35_15 = (float *)malloc(sizeof(float *));
	float * d_var_35_15;
	cudaMalloc((void **)&d_var_35_15, sizeof(float *));
	
	float * h_var_35_16 = (float *)malloc(sizeof(float *));
	float * d_var_35_16;
	cudaMalloc((void **)&d_var_35_16, sizeof(float *));
	
	float * h_var_35_17 = (float *)malloc(sizeof(float *));
	float * d_var_35_17;
	cudaMalloc((void **)&d_var_35_17, sizeof(float *));
	
	float * h_var_35_18 = (float *)malloc(sizeof(float *));
	float * d_var_35_18;
	cudaMalloc((void **)&d_var_35_18, sizeof(float *));
	
	float * h_var_35_19 = (float *)malloc(sizeof(float *));
	float * d_var_35_19;
	cudaMalloc((void **)&d_var_35_19, sizeof(float *));
	
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
	
	float * h_var_36_10 = (float *)malloc(sizeof(float *));
	float * d_var_36_10;
	cudaMalloc((void **)&d_var_36_10, sizeof(float *));
	
	float * h_var_36_11 = (float *)malloc(sizeof(float *));
	float * d_var_36_11;
	cudaMalloc((void **)&d_var_36_11, sizeof(float *));
	
	float * h_var_36_12 = (float *)malloc(sizeof(float *));
	float * d_var_36_12;
	cudaMalloc((void **)&d_var_36_12, sizeof(float *));
	
	float * h_var_36_13 = (float *)malloc(sizeof(float *));
	float * d_var_36_13;
	cudaMalloc((void **)&d_var_36_13, sizeof(float *));
	
	float * h_var_36_14 = (float *)malloc(sizeof(float *));
	float * d_var_36_14;
	cudaMalloc((void **)&d_var_36_14, sizeof(float *));
	
	float * h_var_36_15 = (float *)malloc(sizeof(float *));
	float * d_var_36_15;
	cudaMalloc((void **)&d_var_36_15, sizeof(float *));
	
	float * h_var_36_16 = (float *)malloc(sizeof(float *));
	float * d_var_36_16;
	cudaMalloc((void **)&d_var_36_16, sizeof(float *));
	
	float * h_var_36_17 = (float *)malloc(sizeof(float *));
	float * d_var_36_17;
	cudaMalloc((void **)&d_var_36_17, sizeof(float *));
	
	float * h_var_36_18 = (float *)malloc(sizeof(float *));
	float * d_var_36_18;
	cudaMalloc((void **)&d_var_36_18, sizeof(float *));
	
	float * h_var_36_19 = (float *)malloc(sizeof(float *));
	float * d_var_36_19;
	cudaMalloc((void **)&d_var_36_19, sizeof(float *));
	
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
	
	float * h_var_37_10 = (float *)malloc(sizeof(float *));
	float * d_var_37_10;
	cudaMalloc((void **)&d_var_37_10, sizeof(float *));
	
	float * h_var_37_11 = (float *)malloc(sizeof(float *));
	float * d_var_37_11;
	cudaMalloc((void **)&d_var_37_11, sizeof(float *));
	
	float * h_var_37_12 = (float *)malloc(sizeof(float *));
	float * d_var_37_12;
	cudaMalloc((void **)&d_var_37_12, sizeof(float *));
	
	float * h_var_37_13 = (float *)malloc(sizeof(float *));
	float * d_var_37_13;
	cudaMalloc((void **)&d_var_37_13, sizeof(float *));
	
	float * h_var_37_14 = (float *)malloc(sizeof(float *));
	float * d_var_37_14;
	cudaMalloc((void **)&d_var_37_14, sizeof(float *));
	
	float * h_var_37_15 = (float *)malloc(sizeof(float *));
	float * d_var_37_15;
	cudaMalloc((void **)&d_var_37_15, sizeof(float *));
	
	float * h_var_37_16 = (float *)malloc(sizeof(float *));
	float * d_var_37_16;
	cudaMalloc((void **)&d_var_37_16, sizeof(float *));
	
	float * h_var_37_17 = (float *)malloc(sizeof(float *));
	float * d_var_37_17;
	cudaMalloc((void **)&d_var_37_17, sizeof(float *));
	
	float * h_var_37_18 = (float *)malloc(sizeof(float *));
	float * d_var_37_18;
	cudaMalloc((void **)&d_var_37_18, sizeof(float *));
	
	float * h_var_37_19 = (float *)malloc(sizeof(float *));
	float * d_var_37_19;
	cudaMalloc((void **)&d_var_37_19, sizeof(float *));
	
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
	
	float * h_var_38_10 = (float *)malloc(sizeof(float *));
	float * d_var_38_10;
	cudaMalloc((void **)&d_var_38_10, sizeof(float *));
	
	float * h_var_38_11 = (float *)malloc(sizeof(float *));
	float * d_var_38_11;
	cudaMalloc((void **)&d_var_38_11, sizeof(float *));
	
	float * h_var_38_12 = (float *)malloc(sizeof(float *));
	float * d_var_38_12;
	cudaMalloc((void **)&d_var_38_12, sizeof(float *));
	
	float * h_var_38_13 = (float *)malloc(sizeof(float *));
	float * d_var_38_13;
	cudaMalloc((void **)&d_var_38_13, sizeof(float *));
	
	float * h_var_38_14 = (float *)malloc(sizeof(float *));
	float * d_var_38_14;
	cudaMalloc((void **)&d_var_38_14, sizeof(float *));
	
	float * h_var_38_15 = (float *)malloc(sizeof(float *));
	float * d_var_38_15;
	cudaMalloc((void **)&d_var_38_15, sizeof(float *));
	
	float * h_var_38_16 = (float *)malloc(sizeof(float *));
	float * d_var_38_16;
	cudaMalloc((void **)&d_var_38_16, sizeof(float *));
	
	float * h_var_38_17 = (float *)malloc(sizeof(float *));
	float * d_var_38_17;
	cudaMalloc((void **)&d_var_38_17, sizeof(float *));
	
	float * h_var_38_18 = (float *)malloc(sizeof(float *));
	float * d_var_38_18;
	cudaMalloc((void **)&d_var_38_18, sizeof(float *));
	
	float * h_var_38_19 = (float *)malloc(sizeof(float *));
	float * d_var_38_19;
	cudaMalloc((void **)&d_var_38_19, sizeof(float *));
	
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
	
	float * h_var_39_10 = (float *)malloc(sizeof(float *));
	float * d_var_39_10;
	cudaMalloc((void **)&d_var_39_10, sizeof(float *));
	
	float * h_var_39_11 = (float *)malloc(sizeof(float *));
	float * d_var_39_11;
	cudaMalloc((void **)&d_var_39_11, sizeof(float *));
	
	float * h_var_39_12 = (float *)malloc(sizeof(float *));
	float * d_var_39_12;
	cudaMalloc((void **)&d_var_39_12, sizeof(float *));
	
	float * h_var_39_13 = (float *)malloc(sizeof(float *));
	float * d_var_39_13;
	cudaMalloc((void **)&d_var_39_13, sizeof(float *));
	
	float * h_var_39_14 = (float *)malloc(sizeof(float *));
	float * d_var_39_14;
	cudaMalloc((void **)&d_var_39_14, sizeof(float *));
	
	float * h_var_39_15 = (float *)malloc(sizeof(float *));
	float * d_var_39_15;
	cudaMalloc((void **)&d_var_39_15, sizeof(float *));
	
	float * h_var_39_16 = (float *)malloc(sizeof(float *));
	float * d_var_39_16;
	cudaMalloc((void **)&d_var_39_16, sizeof(float *));
	
	float * h_var_39_17 = (float *)malloc(sizeof(float *));
	float * d_var_39_17;
	cudaMalloc((void **)&d_var_39_17, sizeof(float *));
	
	float * h_var_39_18 = (float *)malloc(sizeof(float *));
	float * d_var_39_18;
	cudaMalloc((void **)&d_var_39_18, sizeof(float *));
	
	float * h_var_39_19 = (float *)malloc(sizeof(float *));
	float * d_var_39_19;
	cudaMalloc((void **)&d_var_39_19, sizeof(float *));
	
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
	
	float * h_var_40_10 = (float *)malloc(sizeof(float *));
	float * d_var_40_10;
	cudaMalloc((void **)&d_var_40_10, sizeof(float *));
	
	float * h_var_40_11 = (float *)malloc(sizeof(float *));
	float * d_var_40_11;
	cudaMalloc((void **)&d_var_40_11, sizeof(float *));
	
	float * h_var_40_12 = (float *)malloc(sizeof(float *));
	float * d_var_40_12;
	cudaMalloc((void **)&d_var_40_12, sizeof(float *));
	
	float * h_var_40_13 = (float *)malloc(sizeof(float *));
	float * d_var_40_13;
	cudaMalloc((void **)&d_var_40_13, sizeof(float *));
	
	float * h_var_40_14 = (float *)malloc(sizeof(float *));
	float * d_var_40_14;
	cudaMalloc((void **)&d_var_40_14, sizeof(float *));
	
	float * h_var_40_15 = (float *)malloc(sizeof(float *));
	float * d_var_40_15;
	cudaMalloc((void **)&d_var_40_15, sizeof(float *));
	
	float * h_var_40_16 = (float *)malloc(sizeof(float *));
	float * d_var_40_16;
	cudaMalloc((void **)&d_var_40_16, sizeof(float *));
	
	float * h_var_40_17 = (float *)malloc(sizeof(float *));
	float * d_var_40_17;
	cudaMalloc((void **)&d_var_40_17, sizeof(float *));
	
	float * h_var_40_18 = (float *)malloc(sizeof(float *));
	float * d_var_40_18;
	cudaMalloc((void **)&d_var_40_18, sizeof(float *));
	
	float * h_var_40_19 = (float *)malloc(sizeof(float *));
	float * d_var_40_19;
	cudaMalloc((void **)&d_var_40_19, sizeof(float *));
	
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
	
	float * h_var_41_10 = (float *)malloc(sizeof(float *));
	float * d_var_41_10;
	cudaMalloc((void **)&d_var_41_10, sizeof(float *));
	
	float * h_var_41_11 = (float *)malloc(sizeof(float *));
	float * d_var_41_11;
	cudaMalloc((void **)&d_var_41_11, sizeof(float *));
	
	float * h_var_41_12 = (float *)malloc(sizeof(float *));
	float * d_var_41_12;
	cudaMalloc((void **)&d_var_41_12, sizeof(float *));
	
	float * h_var_41_13 = (float *)malloc(sizeof(float *));
	float * d_var_41_13;
	cudaMalloc((void **)&d_var_41_13, sizeof(float *));
	
	float * h_var_41_14 = (float *)malloc(sizeof(float *));
	float * d_var_41_14;
	cudaMalloc((void **)&d_var_41_14, sizeof(float *));
	
	float * h_var_41_15 = (float *)malloc(sizeof(float *));
	float * d_var_41_15;
	cudaMalloc((void **)&d_var_41_15, sizeof(float *));
	
	float * h_var_41_16 = (float *)malloc(sizeof(float *));
	float * d_var_41_16;
	cudaMalloc((void **)&d_var_41_16, sizeof(float *));
	
	float * h_var_41_17 = (float *)malloc(sizeof(float *));
	float * d_var_41_17;
	cudaMalloc((void **)&d_var_41_17, sizeof(float *));
	
	float * h_var_41_18 = (float *)malloc(sizeof(float *));
	float * d_var_41_18;
	cudaMalloc((void **)&d_var_41_18, sizeof(float *));
	
	float * h_var_41_19 = (float *)malloc(sizeof(float *));
	float * d_var_41_19;
	cudaMalloc((void **)&d_var_41_19, sizeof(float *));
	
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
	
	float * h_var_42_10 = (float *)malloc(sizeof(float *));
	float * d_var_42_10;
	cudaMalloc((void **)&d_var_42_10, sizeof(float *));
	
	float * h_var_42_11 = (float *)malloc(sizeof(float *));
	float * d_var_42_11;
	cudaMalloc((void **)&d_var_42_11, sizeof(float *));
	
	float * h_var_42_12 = (float *)malloc(sizeof(float *));
	float * d_var_42_12;
	cudaMalloc((void **)&d_var_42_12, sizeof(float *));
	
	float * h_var_42_13 = (float *)malloc(sizeof(float *));
	float * d_var_42_13;
	cudaMalloc((void **)&d_var_42_13, sizeof(float *));
	
	float * h_var_42_14 = (float *)malloc(sizeof(float *));
	float * d_var_42_14;
	cudaMalloc((void **)&d_var_42_14, sizeof(float *));
	
	float * h_var_42_15 = (float *)malloc(sizeof(float *));
	float * d_var_42_15;
	cudaMalloc((void **)&d_var_42_15, sizeof(float *));
	
	float * h_var_42_16 = (float *)malloc(sizeof(float *));
	float * d_var_42_16;
	cudaMalloc((void **)&d_var_42_16, sizeof(float *));
	
	float * h_var_42_17 = (float *)malloc(sizeof(float *));
	float * d_var_42_17;
	cudaMalloc((void **)&d_var_42_17, sizeof(float *));
	
	float * h_var_42_18 = (float *)malloc(sizeof(float *));
	float * d_var_42_18;
	cudaMalloc((void **)&d_var_42_18, sizeof(float *));
	
	float * h_var_42_19 = (float *)malloc(sizeof(float *));
	float * d_var_42_19;
	cudaMalloc((void **)&d_var_42_19, sizeof(float *));
	
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
	
	float * h_var_43_10 = (float *)malloc(sizeof(float *));
	float * d_var_43_10;
	cudaMalloc((void **)&d_var_43_10, sizeof(float *));
	
	float * h_var_43_11 = (float *)malloc(sizeof(float *));
	float * d_var_43_11;
	cudaMalloc((void **)&d_var_43_11, sizeof(float *));
	
	float * h_var_43_12 = (float *)malloc(sizeof(float *));
	float * d_var_43_12;
	cudaMalloc((void **)&d_var_43_12, sizeof(float *));
	
	float * h_var_43_13 = (float *)malloc(sizeof(float *));
	float * d_var_43_13;
	cudaMalloc((void **)&d_var_43_13, sizeof(float *));
	
	float * h_var_43_14 = (float *)malloc(sizeof(float *));
	float * d_var_43_14;
	cudaMalloc((void **)&d_var_43_14, sizeof(float *));
	
	float * h_var_43_15 = (float *)malloc(sizeof(float *));
	float * d_var_43_15;
	cudaMalloc((void **)&d_var_43_15, sizeof(float *));
	
	float * h_var_43_16 = (float *)malloc(sizeof(float *));
	float * d_var_43_16;
	cudaMalloc((void **)&d_var_43_16, sizeof(float *));
	
	float * h_var_43_17 = (float *)malloc(sizeof(float *));
	float * d_var_43_17;
	cudaMalloc((void **)&d_var_43_17, sizeof(float *));
	
	float * h_var_43_18 = (float *)malloc(sizeof(float *));
	float * d_var_43_18;
	cudaMalloc((void **)&d_var_43_18, sizeof(float *));
	
	float * h_var_43_19 = (float *)malloc(sizeof(float *));
	float * d_var_43_19;
	cudaMalloc((void **)&d_var_43_19, sizeof(float *));
	
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
	
	float * h_var_44_10 = (float *)malloc(sizeof(float *));
	float * d_var_44_10;
	cudaMalloc((void **)&d_var_44_10, sizeof(float *));
	
	float * h_var_44_11 = (float *)malloc(sizeof(float *));
	float * d_var_44_11;
	cudaMalloc((void **)&d_var_44_11, sizeof(float *));
	
	float * h_var_44_12 = (float *)malloc(sizeof(float *));
	float * d_var_44_12;
	cudaMalloc((void **)&d_var_44_12, sizeof(float *));
	
	float * h_var_44_13 = (float *)malloc(sizeof(float *));
	float * d_var_44_13;
	cudaMalloc((void **)&d_var_44_13, sizeof(float *));
	
	float * h_var_44_14 = (float *)malloc(sizeof(float *));
	float * d_var_44_14;
	cudaMalloc((void **)&d_var_44_14, sizeof(float *));
	
	float * h_var_44_15 = (float *)malloc(sizeof(float *));
	float * d_var_44_15;
	cudaMalloc((void **)&d_var_44_15, sizeof(float *));
	
	float * h_var_44_16 = (float *)malloc(sizeof(float *));
	float * d_var_44_16;
	cudaMalloc((void **)&d_var_44_16, sizeof(float *));
	
	float * h_var_44_17 = (float *)malloc(sizeof(float *));
	float * d_var_44_17;
	cudaMalloc((void **)&d_var_44_17, sizeof(float *));
	
	float * h_var_44_18 = (float *)malloc(sizeof(float *));
	float * d_var_44_18;
	cudaMalloc((void **)&d_var_44_18, sizeof(float *));
	
	float * h_var_44_19 = (float *)malloc(sizeof(float *));
	float * d_var_44_19;
	cudaMalloc((void **)&d_var_44_19, sizeof(float *));
	
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
	
	float * h_var_45_10 = (float *)malloc(sizeof(float *));
	float * d_var_45_10;
	cudaMalloc((void **)&d_var_45_10, sizeof(float *));
	
	float * h_var_45_11 = (float *)malloc(sizeof(float *));
	float * d_var_45_11;
	cudaMalloc((void **)&d_var_45_11, sizeof(float *));
	
	float * h_var_45_12 = (float *)malloc(sizeof(float *));
	float * d_var_45_12;
	cudaMalloc((void **)&d_var_45_12, sizeof(float *));
	
	float * h_var_45_13 = (float *)malloc(sizeof(float *));
	float * d_var_45_13;
	cudaMalloc((void **)&d_var_45_13, sizeof(float *));
	
	float * h_var_45_14 = (float *)malloc(sizeof(float *));
	float * d_var_45_14;
	cudaMalloc((void **)&d_var_45_14, sizeof(float *));
	
	float * h_var_45_15 = (float *)malloc(sizeof(float *));
	float * d_var_45_15;
	cudaMalloc((void **)&d_var_45_15, sizeof(float *));
	
	float * h_var_45_16 = (float *)malloc(sizeof(float *));
	float * d_var_45_16;
	cudaMalloc((void **)&d_var_45_16, sizeof(float *));
	
	float * h_var_45_17 = (float *)malloc(sizeof(float *));
	float * d_var_45_17;
	cudaMalloc((void **)&d_var_45_17, sizeof(float *));
	
	float * h_var_45_18 = (float *)malloc(sizeof(float *));
	float * d_var_45_18;
	cudaMalloc((void **)&d_var_45_18, sizeof(float *));
	
	float * h_var_45_19 = (float *)malloc(sizeof(float *));
	float * d_var_45_19;
	cudaMalloc((void **)&d_var_45_19, sizeof(float *));
	
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
	
	float * h_var_46_10 = (float *)malloc(sizeof(float *));
	float * d_var_46_10;
	cudaMalloc((void **)&d_var_46_10, sizeof(float *));
	
	float * h_var_46_11 = (float *)malloc(sizeof(float *));
	float * d_var_46_11;
	cudaMalloc((void **)&d_var_46_11, sizeof(float *));
	
	float * h_var_46_12 = (float *)malloc(sizeof(float *));
	float * d_var_46_12;
	cudaMalloc((void **)&d_var_46_12, sizeof(float *));
	
	float * h_var_46_13 = (float *)malloc(sizeof(float *));
	float * d_var_46_13;
	cudaMalloc((void **)&d_var_46_13, sizeof(float *));
	
	float * h_var_46_14 = (float *)malloc(sizeof(float *));
	float * d_var_46_14;
	cudaMalloc((void **)&d_var_46_14, sizeof(float *));
	
	float * h_var_46_15 = (float *)malloc(sizeof(float *));
	float * d_var_46_15;
	cudaMalloc((void **)&d_var_46_15, sizeof(float *));
	
	float * h_var_46_16 = (float *)malloc(sizeof(float *));
	float * d_var_46_16;
	cudaMalloc((void **)&d_var_46_16, sizeof(float *));
	
	float * h_var_46_17 = (float *)malloc(sizeof(float *));
	float * d_var_46_17;
	cudaMalloc((void **)&d_var_46_17, sizeof(float *));
	
	float * h_var_46_18 = (float *)malloc(sizeof(float *));
	float * d_var_46_18;
	cudaMalloc((void **)&d_var_46_18, sizeof(float *));
	
	float * h_var_46_19 = (float *)malloc(sizeof(float *));
	float * d_var_46_19;
	cudaMalloc((void **)&d_var_46_19, sizeof(float *));
	
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
	
	float * h_var_47_10 = (float *)malloc(sizeof(float *));
	float * d_var_47_10;
	cudaMalloc((void **)&d_var_47_10, sizeof(float *));
	
	float * h_var_47_11 = (float *)malloc(sizeof(float *));
	float * d_var_47_11;
	cudaMalloc((void **)&d_var_47_11, sizeof(float *));
	
	float * h_var_47_12 = (float *)malloc(sizeof(float *));
	float * d_var_47_12;
	cudaMalloc((void **)&d_var_47_12, sizeof(float *));
	
	float * h_var_47_13 = (float *)malloc(sizeof(float *));
	float * d_var_47_13;
	cudaMalloc((void **)&d_var_47_13, sizeof(float *));
	
	float * h_var_47_14 = (float *)malloc(sizeof(float *));
	float * d_var_47_14;
	cudaMalloc((void **)&d_var_47_14, sizeof(float *));
	
	float * h_var_47_15 = (float *)malloc(sizeof(float *));
	float * d_var_47_15;
	cudaMalloc((void **)&d_var_47_15, sizeof(float *));
	
	float * h_var_47_16 = (float *)malloc(sizeof(float *));
	float * d_var_47_16;
	cudaMalloc((void **)&d_var_47_16, sizeof(float *));
	
	float * h_var_47_17 = (float *)malloc(sizeof(float *));
	float * d_var_47_17;
	cudaMalloc((void **)&d_var_47_17, sizeof(float *));
	
	float * h_var_47_18 = (float *)malloc(sizeof(float *));
	float * d_var_47_18;
	cudaMalloc((void **)&d_var_47_18, sizeof(float *));
	
	float * h_var_47_19 = (float *)malloc(sizeof(float *));
	float * d_var_47_19;
	cudaMalloc((void **)&d_var_47_19, sizeof(float *));
	
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
	
	float * h_var_48_10 = (float *)malloc(sizeof(float *));
	float * d_var_48_10;
	cudaMalloc((void **)&d_var_48_10, sizeof(float *));
	
	float * h_var_48_11 = (float *)malloc(sizeof(float *));
	float * d_var_48_11;
	cudaMalloc((void **)&d_var_48_11, sizeof(float *));
	
	float * h_var_48_12 = (float *)malloc(sizeof(float *));
	float * d_var_48_12;
	cudaMalloc((void **)&d_var_48_12, sizeof(float *));
	
	float * h_var_48_13 = (float *)malloc(sizeof(float *));
	float * d_var_48_13;
	cudaMalloc((void **)&d_var_48_13, sizeof(float *));
	
	float * h_var_48_14 = (float *)malloc(sizeof(float *));
	float * d_var_48_14;
	cudaMalloc((void **)&d_var_48_14, sizeof(float *));
	
	float * h_var_48_15 = (float *)malloc(sizeof(float *));
	float * d_var_48_15;
	cudaMalloc((void **)&d_var_48_15, sizeof(float *));
	
	float * h_var_48_16 = (float *)malloc(sizeof(float *));
	float * d_var_48_16;
	cudaMalloc((void **)&d_var_48_16, sizeof(float *));
	
	float * h_var_48_17 = (float *)malloc(sizeof(float *));
	float * d_var_48_17;
	cudaMalloc((void **)&d_var_48_17, sizeof(float *));
	
	float * h_var_48_18 = (float *)malloc(sizeof(float *));
	float * d_var_48_18;
	cudaMalloc((void **)&d_var_48_18, sizeof(float *));
	
	float * h_var_48_19 = (float *)malloc(sizeof(float *));
	float * d_var_48_19;
	cudaMalloc((void **)&d_var_48_19, sizeof(float *));
	
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
	
	float * h_var_49_10 = (float *)malloc(sizeof(float *));
	float * d_var_49_10;
	cudaMalloc((void **)&d_var_49_10, sizeof(float *));
	
	float * h_var_49_11 = (float *)malloc(sizeof(float *));
	float * d_var_49_11;
	cudaMalloc((void **)&d_var_49_11, sizeof(float *));
	
	float * h_var_49_12 = (float *)malloc(sizeof(float *));
	float * d_var_49_12;
	cudaMalloc((void **)&d_var_49_12, sizeof(float *));
	
	float * h_var_49_13 = (float *)malloc(sizeof(float *));
	float * d_var_49_13;
	cudaMalloc((void **)&d_var_49_13, sizeof(float *));
	
	float * h_var_49_14 = (float *)malloc(sizeof(float *));
	float * d_var_49_14;
	cudaMalloc((void **)&d_var_49_14, sizeof(float *));
	
	float * h_var_49_15 = (float *)malloc(sizeof(float *));
	float * d_var_49_15;
	cudaMalloc((void **)&d_var_49_15, sizeof(float *));
	
	float * h_var_49_16 = (float *)malloc(sizeof(float *));
	float * d_var_49_16;
	cudaMalloc((void **)&d_var_49_16, sizeof(float *));
	
	float * h_var_49_17 = (float *)malloc(sizeof(float *));
	float * d_var_49_17;
	cudaMalloc((void **)&d_var_49_17, sizeof(float *));
	
	float * h_var_49_18 = (float *)malloc(sizeof(float *));
	float * d_var_49_18;
	cudaMalloc((void **)&d_var_49_18, sizeof(float *));
	
	float * h_var_49_19 = (float *)malloc(sizeof(float *));
	float * d_var_49_19;
	cudaMalloc((void **)&d_var_49_19, sizeof(float *));
	
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
	
	float * h_var_50_10 = (float *)malloc(sizeof(float *));
	float * d_var_50_10;
	cudaMalloc((void **)&d_var_50_10, sizeof(float *));
	
	float * h_var_50_11 = (float *)malloc(sizeof(float *));
	float * d_var_50_11;
	cudaMalloc((void **)&d_var_50_11, sizeof(float *));
	
	float * h_var_50_12 = (float *)malloc(sizeof(float *));
	float * d_var_50_12;
	cudaMalloc((void **)&d_var_50_12, sizeof(float *));
	
	float * h_var_50_13 = (float *)malloc(sizeof(float *));
	float * d_var_50_13;
	cudaMalloc((void **)&d_var_50_13, sizeof(float *));
	
	float * h_var_50_14 = (float *)malloc(sizeof(float *));
	float * d_var_50_14;
	cudaMalloc((void **)&d_var_50_14, sizeof(float *));
	
	float * h_var_50_15 = (float *)malloc(sizeof(float *));
	float * d_var_50_15;
	cudaMalloc((void **)&d_var_50_15, sizeof(float *));
	
	float * h_var_50_16 = (float *)malloc(sizeof(float *));
	float * d_var_50_16;
	cudaMalloc((void **)&d_var_50_16, sizeof(float *));
	
	float * h_var_50_17 = (float *)malloc(sizeof(float *));
	float * d_var_50_17;
	cudaMalloc((void **)&d_var_50_17, sizeof(float *));
	
	float * h_var_50_18 = (float *)malloc(sizeof(float *));
	float * d_var_50_18;
	cudaMalloc((void **)&d_var_50_18, sizeof(float *));
	
	float * h_var_50_19 = (float *)malloc(sizeof(float *));
	float * d_var_50_19;
	cudaMalloc((void **)&d_var_50_19, sizeof(float *));
	
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
	
	float * h_var_51_10 = (float *)malloc(sizeof(float *));
	float * d_var_51_10;
	cudaMalloc((void **)&d_var_51_10, sizeof(float *));
	
	float * h_var_51_11 = (float *)malloc(sizeof(float *));
	float * d_var_51_11;
	cudaMalloc((void **)&d_var_51_11, sizeof(float *));
	
	float * h_var_51_12 = (float *)malloc(sizeof(float *));
	float * d_var_51_12;
	cudaMalloc((void **)&d_var_51_12, sizeof(float *));
	
	float * h_var_51_13 = (float *)malloc(sizeof(float *));
	float * d_var_51_13;
	cudaMalloc((void **)&d_var_51_13, sizeof(float *));
	
	float * h_var_51_14 = (float *)malloc(sizeof(float *));
	float * d_var_51_14;
	cudaMalloc((void **)&d_var_51_14, sizeof(float *));
	
	float * h_var_51_15 = (float *)malloc(sizeof(float *));
	float * d_var_51_15;
	cudaMalloc((void **)&d_var_51_15, sizeof(float *));
	
	float * h_var_51_16 = (float *)malloc(sizeof(float *));
	float * d_var_51_16;
	cudaMalloc((void **)&d_var_51_16, sizeof(float *));
	
	float * h_var_51_17 = (float *)malloc(sizeof(float *));
	float * d_var_51_17;
	cudaMalloc((void **)&d_var_51_17, sizeof(float *));
	
	float * h_var_51_18 = (float *)malloc(sizeof(float *));
	float * d_var_51_18;
	cudaMalloc((void **)&d_var_51_18, sizeof(float *));
	
	float * h_var_51_19 = (float *)malloc(sizeof(float *));
	float * d_var_51_19;
	cudaMalloc((void **)&d_var_51_19, sizeof(float *));
	
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
	
	float * h_var_52_10 = (float *)malloc(sizeof(float *));
	float * d_var_52_10;
	cudaMalloc((void **)&d_var_52_10, sizeof(float *));
	
	float * h_var_52_11 = (float *)malloc(sizeof(float *));
	float * d_var_52_11;
	cudaMalloc((void **)&d_var_52_11, sizeof(float *));
	
	float * h_var_52_12 = (float *)malloc(sizeof(float *));
	float * d_var_52_12;
	cudaMalloc((void **)&d_var_52_12, sizeof(float *));
	
	float * h_var_52_13 = (float *)malloc(sizeof(float *));
	float * d_var_52_13;
	cudaMalloc((void **)&d_var_52_13, sizeof(float *));
	
	float * h_var_52_14 = (float *)malloc(sizeof(float *));
	float * d_var_52_14;
	cudaMalloc((void **)&d_var_52_14, sizeof(float *));
	
	float * h_var_52_15 = (float *)malloc(sizeof(float *));
	float * d_var_52_15;
	cudaMalloc((void **)&d_var_52_15, sizeof(float *));
	
	float * h_var_52_16 = (float *)malloc(sizeof(float *));
	float * d_var_52_16;
	cudaMalloc((void **)&d_var_52_16, sizeof(float *));
	
	float * h_var_52_17 = (float *)malloc(sizeof(float *));
	float * d_var_52_17;
	cudaMalloc((void **)&d_var_52_17, sizeof(float *));
	
	float * h_var_52_18 = (float *)malloc(sizeof(float *));
	float * d_var_52_18;
	cudaMalloc((void **)&d_var_52_18, sizeof(float *));
	
	float * h_var_52_19 = (float *)malloc(sizeof(float *));
	float * d_var_52_19;
	cudaMalloc((void **)&d_var_52_19, sizeof(float *));
	
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
	
	float * h_var_53_10 = (float *)malloc(sizeof(float *));
	float * d_var_53_10;
	cudaMalloc((void **)&d_var_53_10, sizeof(float *));
	
	float * h_var_53_11 = (float *)malloc(sizeof(float *));
	float * d_var_53_11;
	cudaMalloc((void **)&d_var_53_11, sizeof(float *));
	
	float * h_var_53_12 = (float *)malloc(sizeof(float *));
	float * d_var_53_12;
	cudaMalloc((void **)&d_var_53_12, sizeof(float *));
	
	float * h_var_53_13 = (float *)malloc(sizeof(float *));
	float * d_var_53_13;
	cudaMalloc((void **)&d_var_53_13, sizeof(float *));
	
	float * h_var_53_14 = (float *)malloc(sizeof(float *));
	float * d_var_53_14;
	cudaMalloc((void **)&d_var_53_14, sizeof(float *));
	
	float * h_var_53_15 = (float *)malloc(sizeof(float *));
	float * d_var_53_15;
	cudaMalloc((void **)&d_var_53_15, sizeof(float *));
	
	float * h_var_53_16 = (float *)malloc(sizeof(float *));
	float * d_var_53_16;
	cudaMalloc((void **)&d_var_53_16, sizeof(float *));
	
	float * h_var_53_17 = (float *)malloc(sizeof(float *));
	float * d_var_53_17;
	cudaMalloc((void **)&d_var_53_17, sizeof(float *));
	
	float * h_var_53_18 = (float *)malloc(sizeof(float *));
	float * d_var_53_18;
	cudaMalloc((void **)&d_var_53_18, sizeof(float *));
	
	float * h_var_53_19 = (float *)malloc(sizeof(float *));
	float * d_var_53_19;
	cudaMalloc((void **)&d_var_53_19, sizeof(float *));
	
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
	
	float * h_var_54_10 = (float *)malloc(sizeof(float *));
	float * d_var_54_10;
	cudaMalloc((void **)&d_var_54_10, sizeof(float *));
	
	float * h_var_54_11 = (float *)malloc(sizeof(float *));
	float * d_var_54_11;
	cudaMalloc((void **)&d_var_54_11, sizeof(float *));
	
	float * h_var_54_12 = (float *)malloc(sizeof(float *));
	float * d_var_54_12;
	cudaMalloc((void **)&d_var_54_12, sizeof(float *));
	
	float * h_var_54_13 = (float *)malloc(sizeof(float *));
	float * d_var_54_13;
	cudaMalloc((void **)&d_var_54_13, sizeof(float *));
	
	float * h_var_54_14 = (float *)malloc(sizeof(float *));
	float * d_var_54_14;
	cudaMalloc((void **)&d_var_54_14, sizeof(float *));
	
	float * h_var_54_15 = (float *)malloc(sizeof(float *));
	float * d_var_54_15;
	cudaMalloc((void **)&d_var_54_15, sizeof(float *));
	
	float * h_var_54_16 = (float *)malloc(sizeof(float *));
	float * d_var_54_16;
	cudaMalloc((void **)&d_var_54_16, sizeof(float *));
	
	float * h_var_54_17 = (float *)malloc(sizeof(float *));
	float * d_var_54_17;
	cudaMalloc((void **)&d_var_54_17, sizeof(float *));
	
	float * h_var_54_18 = (float *)malloc(sizeof(float *));
	float * d_var_54_18;
	cudaMalloc((void **)&d_var_54_18, sizeof(float *));
	
	float * h_var_54_19 = (float *)malloc(sizeof(float *));
	float * d_var_54_19;
	cudaMalloc((void **)&d_var_54_19, sizeof(float *));
	
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
	
	float * h_var_55_10 = (float *)malloc(sizeof(float *));
	float * d_var_55_10;
	cudaMalloc((void **)&d_var_55_10, sizeof(float *));
	
	float * h_var_55_11 = (float *)malloc(sizeof(float *));
	float * d_var_55_11;
	cudaMalloc((void **)&d_var_55_11, sizeof(float *));
	
	float * h_var_55_12 = (float *)malloc(sizeof(float *));
	float * d_var_55_12;
	cudaMalloc((void **)&d_var_55_12, sizeof(float *));
	
	float * h_var_55_13 = (float *)malloc(sizeof(float *));
	float * d_var_55_13;
	cudaMalloc((void **)&d_var_55_13, sizeof(float *));
	
	float * h_var_55_14 = (float *)malloc(sizeof(float *));
	float * d_var_55_14;
	cudaMalloc((void **)&d_var_55_14, sizeof(float *));
	
	float * h_var_55_15 = (float *)malloc(sizeof(float *));
	float * d_var_55_15;
	cudaMalloc((void **)&d_var_55_15, sizeof(float *));
	
	float * h_var_55_16 = (float *)malloc(sizeof(float *));
	float * d_var_55_16;
	cudaMalloc((void **)&d_var_55_16, sizeof(float *));
	
	float * h_var_55_17 = (float *)malloc(sizeof(float *));
	float * d_var_55_17;
	cudaMalloc((void **)&d_var_55_17, sizeof(float *));
	
	float * h_var_55_18 = (float *)malloc(sizeof(float *));
	float * d_var_55_18;
	cudaMalloc((void **)&d_var_55_18, sizeof(float *));
	
	float * h_var_55_19 = (float *)malloc(sizeof(float *));
	float * d_var_55_19;
	cudaMalloc((void **)&d_var_55_19, sizeof(float *));
	
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
	
	float * h_var_56_10 = (float *)malloc(sizeof(float *));
	float * d_var_56_10;
	cudaMalloc((void **)&d_var_56_10, sizeof(float *));
	
	float * h_var_56_11 = (float *)malloc(sizeof(float *));
	float * d_var_56_11;
	cudaMalloc((void **)&d_var_56_11, sizeof(float *));
	
	float * h_var_56_12 = (float *)malloc(sizeof(float *));
	float * d_var_56_12;
	cudaMalloc((void **)&d_var_56_12, sizeof(float *));
	
	float * h_var_56_13 = (float *)malloc(sizeof(float *));
	float * d_var_56_13;
	cudaMalloc((void **)&d_var_56_13, sizeof(float *));
	
	float * h_var_56_14 = (float *)malloc(sizeof(float *));
	float * d_var_56_14;
	cudaMalloc((void **)&d_var_56_14, sizeof(float *));
	
	float * h_var_56_15 = (float *)malloc(sizeof(float *));
	float * d_var_56_15;
	cudaMalloc((void **)&d_var_56_15, sizeof(float *));
	
	float * h_var_56_16 = (float *)malloc(sizeof(float *));
	float * d_var_56_16;
	cudaMalloc((void **)&d_var_56_16, sizeof(float *));
	
	float * h_var_56_17 = (float *)malloc(sizeof(float *));
	float * d_var_56_17;
	cudaMalloc((void **)&d_var_56_17, sizeof(float *));
	
	float * h_var_56_18 = (float *)malloc(sizeof(float *));
	float * d_var_56_18;
	cudaMalloc((void **)&d_var_56_18, sizeof(float *));
	
	float * h_var_56_19 = (float *)malloc(sizeof(float *));
	float * d_var_56_19;
	cudaMalloc((void **)&d_var_56_19, sizeof(float *));
	
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
	
	float * h_var_57_10 = (float *)malloc(sizeof(float *));
	float * d_var_57_10;
	cudaMalloc((void **)&d_var_57_10, sizeof(float *));
	
	float * h_var_57_11 = (float *)malloc(sizeof(float *));
	float * d_var_57_11;
	cudaMalloc((void **)&d_var_57_11, sizeof(float *));
	
	float * h_var_57_12 = (float *)malloc(sizeof(float *));
	float * d_var_57_12;
	cudaMalloc((void **)&d_var_57_12, sizeof(float *));
	
	float * h_var_57_13 = (float *)malloc(sizeof(float *));
	float * d_var_57_13;
	cudaMalloc((void **)&d_var_57_13, sizeof(float *));
	
	float * h_var_57_14 = (float *)malloc(sizeof(float *));
	float * d_var_57_14;
	cudaMalloc((void **)&d_var_57_14, sizeof(float *));
	
	float * h_var_57_15 = (float *)malloc(sizeof(float *));
	float * d_var_57_15;
	cudaMalloc((void **)&d_var_57_15, sizeof(float *));
	
	float * h_var_57_16 = (float *)malloc(sizeof(float *));
	float * d_var_57_16;
	cudaMalloc((void **)&d_var_57_16, sizeof(float *));
	
	float * h_var_57_17 = (float *)malloc(sizeof(float *));
	float * d_var_57_17;
	cudaMalloc((void **)&d_var_57_17, sizeof(float *));
	
	float * h_var_57_18 = (float *)malloc(sizeof(float *));
	float * d_var_57_18;
	cudaMalloc((void **)&d_var_57_18, sizeof(float *));
	
	float * h_var_57_19 = (float *)malloc(sizeof(float *));
	float * d_var_57_19;
	cudaMalloc((void **)&d_var_57_19, sizeof(float *));
	
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
	
	float * h_var_58_10 = (float *)malloc(sizeof(float *));
	float * d_var_58_10;
	cudaMalloc((void **)&d_var_58_10, sizeof(float *));
	
	float * h_var_58_11 = (float *)malloc(sizeof(float *));
	float * d_var_58_11;
	cudaMalloc((void **)&d_var_58_11, sizeof(float *));
	
	float * h_var_58_12 = (float *)malloc(sizeof(float *));
	float * d_var_58_12;
	cudaMalloc((void **)&d_var_58_12, sizeof(float *));
	
	float * h_var_58_13 = (float *)malloc(sizeof(float *));
	float * d_var_58_13;
	cudaMalloc((void **)&d_var_58_13, sizeof(float *));
	
	float * h_var_58_14 = (float *)malloc(sizeof(float *));
	float * d_var_58_14;
	cudaMalloc((void **)&d_var_58_14, sizeof(float *));
	
	float * h_var_58_15 = (float *)malloc(sizeof(float *));
	float * d_var_58_15;
	cudaMalloc((void **)&d_var_58_15, sizeof(float *));
	
	float * h_var_58_16 = (float *)malloc(sizeof(float *));
	float * d_var_58_16;
	cudaMalloc((void **)&d_var_58_16, sizeof(float *));
	
	float * h_var_58_17 = (float *)malloc(sizeof(float *));
	float * d_var_58_17;
	cudaMalloc((void **)&d_var_58_17, sizeof(float *));
	
	float * h_var_58_18 = (float *)malloc(sizeof(float *));
	float * d_var_58_18;
	cudaMalloc((void **)&d_var_58_18, sizeof(float *));
	
	float * h_var_58_19 = (float *)malloc(sizeof(float *));
	float * d_var_58_19;
	cudaMalloc((void **)&d_var_58_19, sizeof(float *));
	
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
	
	float * h_var_59_10 = (float *)malloc(sizeof(float *));
	float * d_var_59_10;
	cudaMalloc((void **)&d_var_59_10, sizeof(float *));
	
	float * h_var_59_11 = (float *)malloc(sizeof(float *));
	float * d_var_59_11;
	cudaMalloc((void **)&d_var_59_11, sizeof(float *));
	
	float * h_var_59_12 = (float *)malloc(sizeof(float *));
	float * d_var_59_12;
	cudaMalloc((void **)&d_var_59_12, sizeof(float *));
	
	float * h_var_59_13 = (float *)malloc(sizeof(float *));
	float * d_var_59_13;
	cudaMalloc((void **)&d_var_59_13, sizeof(float *));
	
	float * h_var_59_14 = (float *)malloc(sizeof(float *));
	float * d_var_59_14;
	cudaMalloc((void **)&d_var_59_14, sizeof(float *));
	
	float * h_var_59_15 = (float *)malloc(sizeof(float *));
	float * d_var_59_15;
	cudaMalloc((void **)&d_var_59_15, sizeof(float *));
	
	float * h_var_59_16 = (float *)malloc(sizeof(float *));
	float * d_var_59_16;
	cudaMalloc((void **)&d_var_59_16, sizeof(float *));
	
	float * h_var_59_17 = (float *)malloc(sizeof(float *));
	float * d_var_59_17;
	cudaMalloc((void **)&d_var_59_17, sizeof(float *));
	
	float * h_var_59_18 = (float *)malloc(sizeof(float *));
	float * d_var_59_18;
	cudaMalloc((void **)&d_var_59_18, sizeof(float *));
	
	float * h_var_59_19 = (float *)malloc(sizeof(float *));
	float * d_var_59_19;
	cudaMalloc((void **)&d_var_59_19, sizeof(float *));
	
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
	
	float * h_var_60_10 = (float *)malloc(sizeof(float *));
	float * d_var_60_10;
	cudaMalloc((void **)&d_var_60_10, sizeof(float *));
	
	float * h_var_60_11 = (float *)malloc(sizeof(float *));
	float * d_var_60_11;
	cudaMalloc((void **)&d_var_60_11, sizeof(float *));
	
	float * h_var_60_12 = (float *)malloc(sizeof(float *));
	float * d_var_60_12;
	cudaMalloc((void **)&d_var_60_12, sizeof(float *));
	
	float * h_var_60_13 = (float *)malloc(sizeof(float *));
	float * d_var_60_13;
	cudaMalloc((void **)&d_var_60_13, sizeof(float *));
	
	float * h_var_60_14 = (float *)malloc(sizeof(float *));
	float * d_var_60_14;
	cudaMalloc((void **)&d_var_60_14, sizeof(float *));
	
	float * h_var_60_15 = (float *)malloc(sizeof(float *));
	float * d_var_60_15;
	cudaMalloc((void **)&d_var_60_15, sizeof(float *));
	
	float * h_var_60_16 = (float *)malloc(sizeof(float *));
	float * d_var_60_16;
	cudaMalloc((void **)&d_var_60_16, sizeof(float *));
	
	float * h_var_60_17 = (float *)malloc(sizeof(float *));
	float * d_var_60_17;
	cudaMalloc((void **)&d_var_60_17, sizeof(float *));
	
	float * h_var_60_18 = (float *)malloc(sizeof(float *));
	float * d_var_60_18;
	cudaMalloc((void **)&d_var_60_18, sizeof(float *));
	
	float * h_var_60_19 = (float *)malloc(sizeof(float *));
	float * d_var_60_19;
	cudaMalloc((void **)&d_var_60_19, sizeof(float *));
	
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
	
	float * h_var_61_10 = (float *)malloc(sizeof(float *));
	float * d_var_61_10;
	cudaMalloc((void **)&d_var_61_10, sizeof(float *));
	
	float * h_var_61_11 = (float *)malloc(sizeof(float *));
	float * d_var_61_11;
	cudaMalloc((void **)&d_var_61_11, sizeof(float *));
	
	float * h_var_61_12 = (float *)malloc(sizeof(float *));
	float * d_var_61_12;
	cudaMalloc((void **)&d_var_61_12, sizeof(float *));
	
	float * h_var_61_13 = (float *)malloc(sizeof(float *));
	float * d_var_61_13;
	cudaMalloc((void **)&d_var_61_13, sizeof(float *));
	
	float * h_var_61_14 = (float *)malloc(sizeof(float *));
	float * d_var_61_14;
	cudaMalloc((void **)&d_var_61_14, sizeof(float *));
	
	float * h_var_61_15 = (float *)malloc(sizeof(float *));
	float * d_var_61_15;
	cudaMalloc((void **)&d_var_61_15, sizeof(float *));
	
	float * h_var_61_16 = (float *)malloc(sizeof(float *));
	float * d_var_61_16;
	cudaMalloc((void **)&d_var_61_16, sizeof(float *));
	
	float * h_var_61_17 = (float *)malloc(sizeof(float *));
	float * d_var_61_17;
	cudaMalloc((void **)&d_var_61_17, sizeof(float *));
	
	float * h_var_61_18 = (float *)malloc(sizeof(float *));
	float * d_var_61_18;
	cudaMalloc((void **)&d_var_61_18, sizeof(float *));
	
	float * h_var_61_19 = (float *)malloc(sizeof(float *));
	float * d_var_61_19;
	cudaMalloc((void **)&d_var_61_19, sizeof(float *));
	
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
	
	float * h_var_62_10 = (float *)malloc(sizeof(float *));
	float * d_var_62_10;
	cudaMalloc((void **)&d_var_62_10, sizeof(float *));
	
	float * h_var_62_11 = (float *)malloc(sizeof(float *));
	float * d_var_62_11;
	cudaMalloc((void **)&d_var_62_11, sizeof(float *));
	
	float * h_var_62_12 = (float *)malloc(sizeof(float *));
	float * d_var_62_12;
	cudaMalloc((void **)&d_var_62_12, sizeof(float *));
	
	float * h_var_62_13 = (float *)malloc(sizeof(float *));
	float * d_var_62_13;
	cudaMalloc((void **)&d_var_62_13, sizeof(float *));
	
	float * h_var_62_14 = (float *)malloc(sizeof(float *));
	float * d_var_62_14;
	cudaMalloc((void **)&d_var_62_14, sizeof(float *));
	
	float * h_var_62_15 = (float *)malloc(sizeof(float *));
	float * d_var_62_15;
	cudaMalloc((void **)&d_var_62_15, sizeof(float *));
	
	float * h_var_62_16 = (float *)malloc(sizeof(float *));
	float * d_var_62_16;
	cudaMalloc((void **)&d_var_62_16, sizeof(float *));
	
	float * h_var_62_17 = (float *)malloc(sizeof(float *));
	float * d_var_62_17;
	cudaMalloc((void **)&d_var_62_17, sizeof(float *));
	
	float * h_var_62_18 = (float *)malloc(sizeof(float *));
	float * d_var_62_18;
	cudaMalloc((void **)&d_var_62_18, sizeof(float *));
	
	float * h_var_62_19 = (float *)malloc(sizeof(float *));
	float * d_var_62_19;
	cudaMalloc((void **)&d_var_62_19, sizeof(float *));
	
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
	
	float * h_var_63_10 = (float *)malloc(sizeof(float *));
	float * d_var_63_10;
	cudaMalloc((void **)&d_var_63_10, sizeof(float *));
	
	float * h_var_63_11 = (float *)malloc(sizeof(float *));
	float * d_var_63_11;
	cudaMalloc((void **)&d_var_63_11, sizeof(float *));
	
	float * h_var_63_12 = (float *)malloc(sizeof(float *));
	float * d_var_63_12;
	cudaMalloc((void **)&d_var_63_12, sizeof(float *));
	
	float * h_var_63_13 = (float *)malloc(sizeof(float *));
	float * d_var_63_13;
	cudaMalloc((void **)&d_var_63_13, sizeof(float *));
	
	float * h_var_63_14 = (float *)malloc(sizeof(float *));
	float * d_var_63_14;
	cudaMalloc((void **)&d_var_63_14, sizeof(float *));
	
	float * h_var_63_15 = (float *)malloc(sizeof(float *));
	float * d_var_63_15;
	cudaMalloc((void **)&d_var_63_15, sizeof(float *));
	
	float * h_var_63_16 = (float *)malloc(sizeof(float *));
	float * d_var_63_16;
	cudaMalloc((void **)&d_var_63_16, sizeof(float *));
	
	float * h_var_63_17 = (float *)malloc(sizeof(float *));
	float * d_var_63_17;
	cudaMalloc((void **)&d_var_63_17, sizeof(float *));
	
	float * h_var_63_18 = (float *)malloc(sizeof(float *));
	float * d_var_63_18;
	cudaMalloc((void **)&d_var_63_18, sizeof(float *));
	
	float * h_var_63_19 = (float *)malloc(sizeof(float *));
	float * d_var_63_19;
	cudaMalloc((void **)&d_var_63_19, sizeof(float *));
	
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
	
	float * h_var_64_10 = (float *)malloc(sizeof(float *));
	float * d_var_64_10;
	cudaMalloc((void **)&d_var_64_10, sizeof(float *));
	
	float * h_var_64_11 = (float *)malloc(sizeof(float *));
	float * d_var_64_11;
	cudaMalloc((void **)&d_var_64_11, sizeof(float *));
	
	float * h_var_64_12 = (float *)malloc(sizeof(float *));
	float * d_var_64_12;
	cudaMalloc((void **)&d_var_64_12, sizeof(float *));
	
	float * h_var_64_13 = (float *)malloc(sizeof(float *));
	float * d_var_64_13;
	cudaMalloc((void **)&d_var_64_13, sizeof(float *));
	
	float * h_var_64_14 = (float *)malloc(sizeof(float *));
	float * d_var_64_14;
	cudaMalloc((void **)&d_var_64_14, sizeof(float *));
	
	float * h_var_64_15 = (float *)malloc(sizeof(float *));
	float * d_var_64_15;
	cudaMalloc((void **)&d_var_64_15, sizeof(float *));
	
	float * h_var_64_16 = (float *)malloc(sizeof(float *));
	float * d_var_64_16;
	cudaMalloc((void **)&d_var_64_16, sizeof(float *));
	
	float * h_var_64_17 = (float *)malloc(sizeof(float *));
	float * d_var_64_17;
	cudaMalloc((void **)&d_var_64_17, sizeof(float *));
	
	float * h_var_64_18 = (float *)malloc(sizeof(float *));
	float * d_var_64_18;
	cudaMalloc((void **)&d_var_64_18, sizeof(float *));
	
	float * h_var_64_19 = (float *)malloc(sizeof(float *));
	float * d_var_64_19;
	cudaMalloc((void **)&d_var_64_19, sizeof(float *));
	
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
	
	float * h_var_65_10 = (float *)malloc(sizeof(float *));
	float * d_var_65_10;
	cudaMalloc((void **)&d_var_65_10, sizeof(float *));
	
	float * h_var_65_11 = (float *)malloc(sizeof(float *));
	float * d_var_65_11;
	cudaMalloc((void **)&d_var_65_11, sizeof(float *));
	
	float * h_var_65_12 = (float *)malloc(sizeof(float *));
	float * d_var_65_12;
	cudaMalloc((void **)&d_var_65_12, sizeof(float *));
	
	float * h_var_65_13 = (float *)malloc(sizeof(float *));
	float * d_var_65_13;
	cudaMalloc((void **)&d_var_65_13, sizeof(float *));
	
	float * h_var_65_14 = (float *)malloc(sizeof(float *));
	float * d_var_65_14;
	cudaMalloc((void **)&d_var_65_14, sizeof(float *));
	
	float * h_var_65_15 = (float *)malloc(sizeof(float *));
	float * d_var_65_15;
	cudaMalloc((void **)&d_var_65_15, sizeof(float *));
	
	float * h_var_65_16 = (float *)malloc(sizeof(float *));
	float * d_var_65_16;
	cudaMalloc((void **)&d_var_65_16, sizeof(float *));
	
	float * h_var_65_17 = (float *)malloc(sizeof(float *));
	float * d_var_65_17;
	cudaMalloc((void **)&d_var_65_17, sizeof(float *));
	
	float * h_var_65_18 = (float *)malloc(sizeof(float *));
	float * d_var_65_18;
	cudaMalloc((void **)&d_var_65_18, sizeof(float *));
	
	float * h_var_65_19 = (float *)malloc(sizeof(float *));
	float * d_var_65_19;
	cudaMalloc((void **)&d_var_65_19, sizeof(float *));
	
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
	
	float * h_var_66_10 = (float *)malloc(sizeof(float *));
	float * d_var_66_10;
	cudaMalloc((void **)&d_var_66_10, sizeof(float *));
	
	float * h_var_66_11 = (float *)malloc(sizeof(float *));
	float * d_var_66_11;
	cudaMalloc((void **)&d_var_66_11, sizeof(float *));
	
	float * h_var_66_12 = (float *)malloc(sizeof(float *));
	float * d_var_66_12;
	cudaMalloc((void **)&d_var_66_12, sizeof(float *));
	
	float * h_var_66_13 = (float *)malloc(sizeof(float *));
	float * d_var_66_13;
	cudaMalloc((void **)&d_var_66_13, sizeof(float *));
	
	float * h_var_66_14 = (float *)malloc(sizeof(float *));
	float * d_var_66_14;
	cudaMalloc((void **)&d_var_66_14, sizeof(float *));
	
	float * h_var_66_15 = (float *)malloc(sizeof(float *));
	float * d_var_66_15;
	cudaMalloc((void **)&d_var_66_15, sizeof(float *));
	
	float * h_var_66_16 = (float *)malloc(sizeof(float *));
	float * d_var_66_16;
	cudaMalloc((void **)&d_var_66_16, sizeof(float *));
	
	float * h_var_66_17 = (float *)malloc(sizeof(float *));
	float * d_var_66_17;
	cudaMalloc((void **)&d_var_66_17, sizeof(float *));
	
	float * h_var_66_18 = (float *)malloc(sizeof(float *));
	float * d_var_66_18;
	cudaMalloc((void **)&d_var_66_18, sizeof(float *));
	
	float * h_var_66_19 = (float *)malloc(sizeof(float *));
	float * d_var_66_19;
	cudaMalloc((void **)&d_var_66_19, sizeof(float *));
	
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
	
	float * h_var_67_10 = (float *)malloc(sizeof(float *));
	float * d_var_67_10;
	cudaMalloc((void **)&d_var_67_10, sizeof(float *));
	
	float * h_var_67_11 = (float *)malloc(sizeof(float *));
	float * d_var_67_11;
	cudaMalloc((void **)&d_var_67_11, sizeof(float *));
	
	float * h_var_67_12 = (float *)malloc(sizeof(float *));
	float * d_var_67_12;
	cudaMalloc((void **)&d_var_67_12, sizeof(float *));
	
	float * h_var_67_13 = (float *)malloc(sizeof(float *));
	float * d_var_67_13;
	cudaMalloc((void **)&d_var_67_13, sizeof(float *));
	
	float * h_var_67_14 = (float *)malloc(sizeof(float *));
	float * d_var_67_14;
	cudaMalloc((void **)&d_var_67_14, sizeof(float *));
	
	float * h_var_67_15 = (float *)malloc(sizeof(float *));
	float * d_var_67_15;
	cudaMalloc((void **)&d_var_67_15, sizeof(float *));
	
	float * h_var_67_16 = (float *)malloc(sizeof(float *));
	float * d_var_67_16;
	cudaMalloc((void **)&d_var_67_16, sizeof(float *));
	
	float * h_var_67_17 = (float *)malloc(sizeof(float *));
	float * d_var_67_17;
	cudaMalloc((void **)&d_var_67_17, sizeof(float *));
	
	float * h_var_67_18 = (float *)malloc(sizeof(float *));
	float * d_var_67_18;
	cudaMalloc((void **)&d_var_67_18, sizeof(float *));
	
	float * h_var_67_19 = (float *)malloc(sizeof(float *));
	float * d_var_67_19;
	cudaMalloc((void **)&d_var_67_19, sizeof(float *));
	
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
	
	float * h_var_68_10 = (float *)malloc(sizeof(float *));
	float * d_var_68_10;
	cudaMalloc((void **)&d_var_68_10, sizeof(float *));
	
	float * h_var_68_11 = (float *)malloc(sizeof(float *));
	float * d_var_68_11;
	cudaMalloc((void **)&d_var_68_11, sizeof(float *));
	
	float * h_var_68_12 = (float *)malloc(sizeof(float *));
	float * d_var_68_12;
	cudaMalloc((void **)&d_var_68_12, sizeof(float *));
	
	float * h_var_68_13 = (float *)malloc(sizeof(float *));
	float * d_var_68_13;
	cudaMalloc((void **)&d_var_68_13, sizeof(float *));
	
	float * h_var_68_14 = (float *)malloc(sizeof(float *));
	float * d_var_68_14;
	cudaMalloc((void **)&d_var_68_14, sizeof(float *));
	
	float * h_var_68_15 = (float *)malloc(sizeof(float *));
	float * d_var_68_15;
	cudaMalloc((void **)&d_var_68_15, sizeof(float *));
	
	float * h_var_68_16 = (float *)malloc(sizeof(float *));
	float * d_var_68_16;
	cudaMalloc((void **)&d_var_68_16, sizeof(float *));
	
	float * h_var_68_17 = (float *)malloc(sizeof(float *));
	float * d_var_68_17;
	cudaMalloc((void **)&d_var_68_17, sizeof(float *));
	
	float * h_var_68_18 = (float *)malloc(sizeof(float *));
	float * d_var_68_18;
	cudaMalloc((void **)&d_var_68_18, sizeof(float *));
	
	float * h_var_68_19 = (float *)malloc(sizeof(float *));
	float * d_var_68_19;
	cudaMalloc((void **)&d_var_68_19, sizeof(float *));
	
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
	
	float * h_var_69_10 = (float *)malloc(sizeof(float *));
	float * d_var_69_10;
	cudaMalloc((void **)&d_var_69_10, sizeof(float *));
	
	float * h_var_69_11 = (float *)malloc(sizeof(float *));
	float * d_var_69_11;
	cudaMalloc((void **)&d_var_69_11, sizeof(float *));
	
	float * h_var_69_12 = (float *)malloc(sizeof(float *));
	float * d_var_69_12;
	cudaMalloc((void **)&d_var_69_12, sizeof(float *));
	
	float * h_var_69_13 = (float *)malloc(sizeof(float *));
	float * d_var_69_13;
	cudaMalloc((void **)&d_var_69_13, sizeof(float *));
	
	float * h_var_69_14 = (float *)malloc(sizeof(float *));
	float * d_var_69_14;
	cudaMalloc((void **)&d_var_69_14, sizeof(float *));
	
	float * h_var_69_15 = (float *)malloc(sizeof(float *));
	float * d_var_69_15;
	cudaMalloc((void **)&d_var_69_15, sizeof(float *));
	
	float * h_var_69_16 = (float *)malloc(sizeof(float *));
	float * d_var_69_16;
	cudaMalloc((void **)&d_var_69_16, sizeof(float *));
	
	float * h_var_69_17 = (float *)malloc(sizeof(float *));
	float * d_var_69_17;
	cudaMalloc((void **)&d_var_69_17, sizeof(float *));
	
	float * h_var_69_18 = (float *)malloc(sizeof(float *));
	float * d_var_69_18;
	cudaMalloc((void **)&d_var_69_18, sizeof(float *));
	
	float * h_var_69_19 = (float *)malloc(sizeof(float *));
	float * d_var_69_19;
	cudaMalloc((void **)&d_var_69_19, sizeof(float *));
	
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
	
	float * h_var_70_10 = (float *)malloc(sizeof(float *));
	float * d_var_70_10;
	cudaMalloc((void **)&d_var_70_10, sizeof(float *));
	
	float * h_var_70_11 = (float *)malloc(sizeof(float *));
	float * d_var_70_11;
	cudaMalloc((void **)&d_var_70_11, sizeof(float *));
	
	float * h_var_70_12 = (float *)malloc(sizeof(float *));
	float * d_var_70_12;
	cudaMalloc((void **)&d_var_70_12, sizeof(float *));
	
	float * h_var_70_13 = (float *)malloc(sizeof(float *));
	float * d_var_70_13;
	cudaMalloc((void **)&d_var_70_13, sizeof(float *));
	
	float * h_var_70_14 = (float *)malloc(sizeof(float *));
	float * d_var_70_14;
	cudaMalloc((void **)&d_var_70_14, sizeof(float *));
	
	float * h_var_70_15 = (float *)malloc(sizeof(float *));
	float * d_var_70_15;
	cudaMalloc((void **)&d_var_70_15, sizeof(float *));
	
	float * h_var_70_16 = (float *)malloc(sizeof(float *));
	float * d_var_70_16;
	cudaMalloc((void **)&d_var_70_16, sizeof(float *));
	
	float * h_var_70_17 = (float *)malloc(sizeof(float *));
	float * d_var_70_17;
	cudaMalloc((void **)&d_var_70_17, sizeof(float *));
	
	float * h_var_70_18 = (float *)malloc(sizeof(float *));
	float * d_var_70_18;
	cudaMalloc((void **)&d_var_70_18, sizeof(float *));
	
	float * h_var_70_19 = (float *)malloc(sizeof(float *));
	float * d_var_70_19;
	cudaMalloc((void **)&d_var_70_19, sizeof(float *));
	
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
	
	float * h_var_71_10 = (float *)malloc(sizeof(float *));
	float * d_var_71_10;
	cudaMalloc((void **)&d_var_71_10, sizeof(float *));
	
	float * h_var_71_11 = (float *)malloc(sizeof(float *));
	float * d_var_71_11;
	cudaMalloc((void **)&d_var_71_11, sizeof(float *));
	
	float * h_var_71_12 = (float *)malloc(sizeof(float *));
	float * d_var_71_12;
	cudaMalloc((void **)&d_var_71_12, sizeof(float *));
	
	float * h_var_71_13 = (float *)malloc(sizeof(float *));
	float * d_var_71_13;
	cudaMalloc((void **)&d_var_71_13, sizeof(float *));
	
	float * h_var_71_14 = (float *)malloc(sizeof(float *));
	float * d_var_71_14;
	cudaMalloc((void **)&d_var_71_14, sizeof(float *));
	
	float * h_var_71_15 = (float *)malloc(sizeof(float *));
	float * d_var_71_15;
	cudaMalloc((void **)&d_var_71_15, sizeof(float *));
	
	float * h_var_71_16 = (float *)malloc(sizeof(float *));
	float * d_var_71_16;
	cudaMalloc((void **)&d_var_71_16, sizeof(float *));
	
	float * h_var_71_17 = (float *)malloc(sizeof(float *));
	float * d_var_71_17;
	cudaMalloc((void **)&d_var_71_17, sizeof(float *));
	
	float * h_var_71_18 = (float *)malloc(sizeof(float *));
	float * d_var_71_18;
	cudaMalloc((void **)&d_var_71_18, sizeof(float *));
	
	float * h_var_71_19 = (float *)malloc(sizeof(float *));
	float * d_var_71_19;
	cudaMalloc((void **)&d_var_71_19, sizeof(float *));
	
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
	
	float * h_var_72_10 = (float *)malloc(sizeof(float *));
	float * d_var_72_10;
	cudaMalloc((void **)&d_var_72_10, sizeof(float *));
	
	float * h_var_72_11 = (float *)malloc(sizeof(float *));
	float * d_var_72_11;
	cudaMalloc((void **)&d_var_72_11, sizeof(float *));
	
	float * h_var_72_12 = (float *)malloc(sizeof(float *));
	float * d_var_72_12;
	cudaMalloc((void **)&d_var_72_12, sizeof(float *));
	
	float * h_var_72_13 = (float *)malloc(sizeof(float *));
	float * d_var_72_13;
	cudaMalloc((void **)&d_var_72_13, sizeof(float *));
	
	float * h_var_72_14 = (float *)malloc(sizeof(float *));
	float * d_var_72_14;
	cudaMalloc((void **)&d_var_72_14, sizeof(float *));
	
	float * h_var_72_15 = (float *)malloc(sizeof(float *));
	float * d_var_72_15;
	cudaMalloc((void **)&d_var_72_15, sizeof(float *));
	
	float * h_var_72_16 = (float *)malloc(sizeof(float *));
	float * d_var_72_16;
	cudaMalloc((void **)&d_var_72_16, sizeof(float *));
	
	float * h_var_72_17 = (float *)malloc(sizeof(float *));
	float * d_var_72_17;
	cudaMalloc((void **)&d_var_72_17, sizeof(float *));
	
	float * h_var_72_18 = (float *)malloc(sizeof(float *));
	float * d_var_72_18;
	cudaMalloc((void **)&d_var_72_18, sizeof(float *));
	
	float * h_var_72_19 = (float *)malloc(sizeof(float *));
	float * d_var_72_19;
	cudaMalloc((void **)&d_var_72_19, sizeof(float *));
	
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
	
	float * h_var_73_10 = (float *)malloc(sizeof(float *));
	float * d_var_73_10;
	cudaMalloc((void **)&d_var_73_10, sizeof(float *));
	
	float * h_var_73_11 = (float *)malloc(sizeof(float *));
	float * d_var_73_11;
	cudaMalloc((void **)&d_var_73_11, sizeof(float *));
	
	float * h_var_73_12 = (float *)malloc(sizeof(float *));
	float * d_var_73_12;
	cudaMalloc((void **)&d_var_73_12, sizeof(float *));
	
	float * h_var_73_13 = (float *)malloc(sizeof(float *));
	float * d_var_73_13;
	cudaMalloc((void **)&d_var_73_13, sizeof(float *));
	
	float * h_var_73_14 = (float *)malloc(sizeof(float *));
	float * d_var_73_14;
	cudaMalloc((void **)&d_var_73_14, sizeof(float *));
	
	float * h_var_73_15 = (float *)malloc(sizeof(float *));
	float * d_var_73_15;
	cudaMalloc((void **)&d_var_73_15, sizeof(float *));
	
	float * h_var_73_16 = (float *)malloc(sizeof(float *));
	float * d_var_73_16;
	cudaMalloc((void **)&d_var_73_16, sizeof(float *));
	
	float * h_var_73_17 = (float *)malloc(sizeof(float *));
	float * d_var_73_17;
	cudaMalloc((void **)&d_var_73_17, sizeof(float *));
	
	float * h_var_73_18 = (float *)malloc(sizeof(float *));
	float * d_var_73_18;
	cudaMalloc((void **)&d_var_73_18, sizeof(float *));
	
	float * h_var_73_19 = (float *)malloc(sizeof(float *));
	float * d_var_73_19;
	cudaMalloc((void **)&d_var_73_19, sizeof(float *));
	
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
	
	float * h_var_74_10 = (float *)malloc(sizeof(float *));
	float * d_var_74_10;
	cudaMalloc((void **)&d_var_74_10, sizeof(float *));
	
	float * h_var_74_11 = (float *)malloc(sizeof(float *));
	float * d_var_74_11;
	cudaMalloc((void **)&d_var_74_11, sizeof(float *));
	
	float * h_var_74_12 = (float *)malloc(sizeof(float *));
	float * d_var_74_12;
	cudaMalloc((void **)&d_var_74_12, sizeof(float *));
	
	float * h_var_74_13 = (float *)malloc(sizeof(float *));
	float * d_var_74_13;
	cudaMalloc((void **)&d_var_74_13, sizeof(float *));
	
	float * h_var_74_14 = (float *)malloc(sizeof(float *));
	float * d_var_74_14;
	cudaMalloc((void **)&d_var_74_14, sizeof(float *));
	
	float * h_var_74_15 = (float *)malloc(sizeof(float *));
	float * d_var_74_15;
	cudaMalloc((void **)&d_var_74_15, sizeof(float *));
	
	float * h_var_74_16 = (float *)malloc(sizeof(float *));
	float * d_var_74_16;
	cudaMalloc((void **)&d_var_74_16, sizeof(float *));
	
	float * h_var_74_17 = (float *)malloc(sizeof(float *));
	float * d_var_74_17;
	cudaMalloc((void **)&d_var_74_17, sizeof(float *));
	
	float * h_var_74_18 = (float *)malloc(sizeof(float *));
	float * d_var_74_18;
	cudaMalloc((void **)&d_var_74_18, sizeof(float *));
	
	float * h_var_74_19 = (float *)malloc(sizeof(float *));
	float * d_var_74_19;
	cudaMalloc((void **)&d_var_74_19, sizeof(float *));
	
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
	
	float * h_var_75_10 = (float *)malloc(sizeof(float *));
	float * d_var_75_10;
	cudaMalloc((void **)&d_var_75_10, sizeof(float *));
	
	float * h_var_75_11 = (float *)malloc(sizeof(float *));
	float * d_var_75_11;
	cudaMalloc((void **)&d_var_75_11, sizeof(float *));
	
	float * h_var_75_12 = (float *)malloc(sizeof(float *));
	float * d_var_75_12;
	cudaMalloc((void **)&d_var_75_12, sizeof(float *));
	
	float * h_var_75_13 = (float *)malloc(sizeof(float *));
	float * d_var_75_13;
	cudaMalloc((void **)&d_var_75_13, sizeof(float *));
	
	float * h_var_75_14 = (float *)malloc(sizeof(float *));
	float * d_var_75_14;
	cudaMalloc((void **)&d_var_75_14, sizeof(float *));
	
	float * h_var_75_15 = (float *)malloc(sizeof(float *));
	float * d_var_75_15;
	cudaMalloc((void **)&d_var_75_15, sizeof(float *));
	
	float * h_var_75_16 = (float *)malloc(sizeof(float *));
	float * d_var_75_16;
	cudaMalloc((void **)&d_var_75_16, sizeof(float *));
	
	float * h_var_75_17 = (float *)malloc(sizeof(float *));
	float * d_var_75_17;
	cudaMalloc((void **)&d_var_75_17, sizeof(float *));
	
	float * h_var_75_18 = (float *)malloc(sizeof(float *));
	float * d_var_75_18;
	cudaMalloc((void **)&d_var_75_18, sizeof(float *));
	
	float * h_var_75_19 = (float *)malloc(sizeof(float *));
	float * d_var_75_19;
	cudaMalloc((void **)&d_var_75_19, sizeof(float *));
	
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
	
	float * h_var_76_10 = (float *)malloc(sizeof(float *));
	float * d_var_76_10;
	cudaMalloc((void **)&d_var_76_10, sizeof(float *));
	
	float * h_var_76_11 = (float *)malloc(sizeof(float *));
	float * d_var_76_11;
	cudaMalloc((void **)&d_var_76_11, sizeof(float *));
	
	float * h_var_76_12 = (float *)malloc(sizeof(float *));
	float * d_var_76_12;
	cudaMalloc((void **)&d_var_76_12, sizeof(float *));
	
	float * h_var_76_13 = (float *)malloc(sizeof(float *));
	float * d_var_76_13;
	cudaMalloc((void **)&d_var_76_13, sizeof(float *));
	
	float * h_var_76_14 = (float *)malloc(sizeof(float *));
	float * d_var_76_14;
	cudaMalloc((void **)&d_var_76_14, sizeof(float *));
	
	float * h_var_76_15 = (float *)malloc(sizeof(float *));
	float * d_var_76_15;
	cudaMalloc((void **)&d_var_76_15, sizeof(float *));
	
	float * h_var_76_16 = (float *)malloc(sizeof(float *));
	float * d_var_76_16;
	cudaMalloc((void **)&d_var_76_16, sizeof(float *));
	
	float * h_var_76_17 = (float *)malloc(sizeof(float *));
	float * d_var_76_17;
	cudaMalloc((void **)&d_var_76_17, sizeof(float *));
	
	float * h_var_76_18 = (float *)malloc(sizeof(float *));
	float * d_var_76_18;
	cudaMalloc((void **)&d_var_76_18, sizeof(float *));
	
	float * h_var_76_19 = (float *)malloc(sizeof(float *));
	float * d_var_76_19;
	cudaMalloc((void **)&d_var_76_19, sizeof(float *));
	
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
	
	float * h_var_77_10 = (float *)malloc(sizeof(float *));
	float * d_var_77_10;
	cudaMalloc((void **)&d_var_77_10, sizeof(float *));
	
	float * h_var_77_11 = (float *)malloc(sizeof(float *));
	float * d_var_77_11;
	cudaMalloc((void **)&d_var_77_11, sizeof(float *));
	
	float * h_var_77_12 = (float *)malloc(sizeof(float *));
	float * d_var_77_12;
	cudaMalloc((void **)&d_var_77_12, sizeof(float *));
	
	float * h_var_77_13 = (float *)malloc(sizeof(float *));
	float * d_var_77_13;
	cudaMalloc((void **)&d_var_77_13, sizeof(float *));
	
	float * h_var_77_14 = (float *)malloc(sizeof(float *));
	float * d_var_77_14;
	cudaMalloc((void **)&d_var_77_14, sizeof(float *));
	
	float * h_var_77_15 = (float *)malloc(sizeof(float *));
	float * d_var_77_15;
	cudaMalloc((void **)&d_var_77_15, sizeof(float *));
	
	float * h_var_77_16 = (float *)malloc(sizeof(float *));
	float * d_var_77_16;
	cudaMalloc((void **)&d_var_77_16, sizeof(float *));
	
	float * h_var_77_17 = (float *)malloc(sizeof(float *));
	float * d_var_77_17;
	cudaMalloc((void **)&d_var_77_17, sizeof(float *));
	
	float * h_var_77_18 = (float *)malloc(sizeof(float *));
	float * d_var_77_18;
	cudaMalloc((void **)&d_var_77_18, sizeof(float *));
	
	float * h_var_77_19 = (float *)malloc(sizeof(float *));
	float * d_var_77_19;
	cudaMalloc((void **)&d_var_77_19, sizeof(float *));
	
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
	
	float * h_var_78_10 = (float *)malloc(sizeof(float *));
	float * d_var_78_10;
	cudaMalloc((void **)&d_var_78_10, sizeof(float *));
	
	float * h_var_78_11 = (float *)malloc(sizeof(float *));
	float * d_var_78_11;
	cudaMalloc((void **)&d_var_78_11, sizeof(float *));
	
	float * h_var_78_12 = (float *)malloc(sizeof(float *));
	float * d_var_78_12;
	cudaMalloc((void **)&d_var_78_12, sizeof(float *));
	
	float * h_var_78_13 = (float *)malloc(sizeof(float *));
	float * d_var_78_13;
	cudaMalloc((void **)&d_var_78_13, sizeof(float *));
	
	float * h_var_78_14 = (float *)malloc(sizeof(float *));
	float * d_var_78_14;
	cudaMalloc((void **)&d_var_78_14, sizeof(float *));
	
	float * h_var_78_15 = (float *)malloc(sizeof(float *));
	float * d_var_78_15;
	cudaMalloc((void **)&d_var_78_15, sizeof(float *));
	
	float * h_var_78_16 = (float *)malloc(sizeof(float *));
	float * d_var_78_16;
	cudaMalloc((void **)&d_var_78_16, sizeof(float *));
	
	float * h_var_78_17 = (float *)malloc(sizeof(float *));
	float * d_var_78_17;
	cudaMalloc((void **)&d_var_78_17, sizeof(float *));
	
	float * h_var_78_18 = (float *)malloc(sizeof(float *));
	float * d_var_78_18;
	cudaMalloc((void **)&d_var_78_18, sizeof(float *));
	
	float * h_var_78_19 = (float *)malloc(sizeof(float *));
	float * d_var_78_19;
	cudaMalloc((void **)&d_var_78_19, sizeof(float *));
	
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
	
	float * h_var_79_10 = (float *)malloc(sizeof(float *));
	float * d_var_79_10;
	cudaMalloc((void **)&d_var_79_10, sizeof(float *));
	
	float * h_var_79_11 = (float *)malloc(sizeof(float *));
	float * d_var_79_11;
	cudaMalloc((void **)&d_var_79_11, sizeof(float *));
	
	float * h_var_79_12 = (float *)malloc(sizeof(float *));
	float * d_var_79_12;
	cudaMalloc((void **)&d_var_79_12, sizeof(float *));
	
	float * h_var_79_13 = (float *)malloc(sizeof(float *));
	float * d_var_79_13;
	cudaMalloc((void **)&d_var_79_13, sizeof(float *));
	
	float * h_var_79_14 = (float *)malloc(sizeof(float *));
	float * d_var_79_14;
	cudaMalloc((void **)&d_var_79_14, sizeof(float *));
	
	float * h_var_79_15 = (float *)malloc(sizeof(float *));
	float * d_var_79_15;
	cudaMalloc((void **)&d_var_79_15, sizeof(float *));
	
	float * h_var_79_16 = (float *)malloc(sizeof(float *));
	float * d_var_79_16;
	cudaMalloc((void **)&d_var_79_16, sizeof(float *));
	
	float * h_var_79_17 = (float *)malloc(sizeof(float *));
	float * d_var_79_17;
	cudaMalloc((void **)&d_var_79_17, sizeof(float *));
	
	float * h_var_79_18 = (float *)malloc(sizeof(float *));
	float * d_var_79_18;
	cudaMalloc((void **)&d_var_79_18, sizeof(float *));
	
	float * h_var_79_19 = (float *)malloc(sizeof(float *));
	float * d_var_79_19;
	cudaMalloc((void **)&d_var_79_19, sizeof(float *));
	
	float * h_var_80_0 = (float *)malloc(sizeof(float *));
	float * d_var_80_0;
	cudaMalloc((void **)&d_var_80_0, sizeof(float *));
	
	float * h_var_80_1 = (float *)malloc(sizeof(float *));
	float * d_var_80_1;
	cudaMalloc((void **)&d_var_80_1, sizeof(float *));
	
	float * h_var_80_2 = (float *)malloc(sizeof(float *));
	float * d_var_80_2;
	cudaMalloc((void **)&d_var_80_2, sizeof(float *));
	
	float * h_var_80_3 = (float *)malloc(sizeof(float *));
	float * d_var_80_3;
	cudaMalloc((void **)&d_var_80_3, sizeof(float *));
	
	float * h_var_80_4 = (float *)malloc(sizeof(float *));
	float * d_var_80_4;
	cudaMalloc((void **)&d_var_80_4, sizeof(float *));
	
	float * h_var_80_5 = (float *)malloc(sizeof(float *));
	float * d_var_80_5;
	cudaMalloc((void **)&d_var_80_5, sizeof(float *));
	
	float * h_var_80_6 = (float *)malloc(sizeof(float *));
	float * d_var_80_6;
	cudaMalloc((void **)&d_var_80_6, sizeof(float *));
	
	float * h_var_80_7 = (float *)malloc(sizeof(float *));
	float * d_var_80_7;
	cudaMalloc((void **)&d_var_80_7, sizeof(float *));
	
	float * h_var_80_8 = (float *)malloc(sizeof(float *));
	float * d_var_80_8;
	cudaMalloc((void **)&d_var_80_8, sizeof(float *));
	
	float * h_var_80_9 = (float *)malloc(sizeof(float *));
	float * d_var_80_9;
	cudaMalloc((void **)&d_var_80_9, sizeof(float *));
	
	float * h_var_80_10 = (float *)malloc(sizeof(float *));
	float * d_var_80_10;
	cudaMalloc((void **)&d_var_80_10, sizeof(float *));
	
	float * h_var_80_11 = (float *)malloc(sizeof(float *));
	float * d_var_80_11;
	cudaMalloc((void **)&d_var_80_11, sizeof(float *));
	
	float * h_var_80_12 = (float *)malloc(sizeof(float *));
	float * d_var_80_12;
	cudaMalloc((void **)&d_var_80_12, sizeof(float *));
	
	float * h_var_80_13 = (float *)malloc(sizeof(float *));
	float * d_var_80_13;
	cudaMalloc((void **)&d_var_80_13, sizeof(float *));
	
	float * h_var_80_14 = (float *)malloc(sizeof(float *));
	float * d_var_80_14;
	cudaMalloc((void **)&d_var_80_14, sizeof(float *));
	
	float * h_var_80_15 = (float *)malloc(sizeof(float *));
	float * d_var_80_15;
	cudaMalloc((void **)&d_var_80_15, sizeof(float *));
	
	float * h_var_80_16 = (float *)malloc(sizeof(float *));
	float * d_var_80_16;
	cudaMalloc((void **)&d_var_80_16, sizeof(float *));
	
	float * h_var_80_17 = (float *)malloc(sizeof(float *));
	float * d_var_80_17;
	cudaMalloc((void **)&d_var_80_17, sizeof(float *));
	
	float * h_var_80_18 = (float *)malloc(sizeof(float *));
	float * d_var_80_18;
	cudaMalloc((void **)&d_var_80_18, sizeof(float *));
	
	float * h_var_80_19 = (float *)malloc(sizeof(float *));
	float * d_var_80_19;
	cudaMalloc((void **)&d_var_80_19, sizeof(float *));
	
	float * h_var_81_0 = (float *)malloc(sizeof(float *));
	float * d_var_81_0;
	cudaMalloc((void **)&d_var_81_0, sizeof(float *));
	
	float * h_var_81_1 = (float *)malloc(sizeof(float *));
	float * d_var_81_1;
	cudaMalloc((void **)&d_var_81_1, sizeof(float *));
	
	float * h_var_81_2 = (float *)malloc(sizeof(float *));
	float * d_var_81_2;
	cudaMalloc((void **)&d_var_81_2, sizeof(float *));
	
	float * h_var_81_3 = (float *)malloc(sizeof(float *));
	float * d_var_81_3;
	cudaMalloc((void **)&d_var_81_3, sizeof(float *));
	
	float * h_var_81_4 = (float *)malloc(sizeof(float *));
	float * d_var_81_4;
	cudaMalloc((void **)&d_var_81_4, sizeof(float *));
	
	float * h_var_81_5 = (float *)malloc(sizeof(float *));
	float * d_var_81_5;
	cudaMalloc((void **)&d_var_81_5, sizeof(float *));
	
	float * h_var_81_6 = (float *)malloc(sizeof(float *));
	float * d_var_81_6;
	cudaMalloc((void **)&d_var_81_6, sizeof(float *));
	
	float * h_var_81_7 = (float *)malloc(sizeof(float *));
	float * d_var_81_7;
	cudaMalloc((void **)&d_var_81_7, sizeof(float *));
	
	float * h_var_81_8 = (float *)malloc(sizeof(float *));
	float * d_var_81_8;
	cudaMalloc((void **)&d_var_81_8, sizeof(float *));
	
	float * h_var_81_9 = (float *)malloc(sizeof(float *));
	float * d_var_81_9;
	cudaMalloc((void **)&d_var_81_9, sizeof(float *));
	
	float * h_var_81_10 = (float *)malloc(sizeof(float *));
	float * d_var_81_10;
	cudaMalloc((void **)&d_var_81_10, sizeof(float *));
	
	float * h_var_81_11 = (float *)malloc(sizeof(float *));
	float * d_var_81_11;
	cudaMalloc((void **)&d_var_81_11, sizeof(float *));
	
	float * h_var_81_12 = (float *)malloc(sizeof(float *));
	float * d_var_81_12;
	cudaMalloc((void **)&d_var_81_12, sizeof(float *));
	
	float * h_var_81_13 = (float *)malloc(sizeof(float *));
	float * d_var_81_13;
	cudaMalloc((void **)&d_var_81_13, sizeof(float *));
	
	float * h_var_81_14 = (float *)malloc(sizeof(float *));
	float * d_var_81_14;
	cudaMalloc((void **)&d_var_81_14, sizeof(float *));
	
	float * h_var_81_15 = (float *)malloc(sizeof(float *));
	float * d_var_81_15;
	cudaMalloc((void **)&d_var_81_15, sizeof(float *));
	
	float * h_var_81_16 = (float *)malloc(sizeof(float *));
	float * d_var_81_16;
	cudaMalloc((void **)&d_var_81_16, sizeof(float *));
	
	float * h_var_81_17 = (float *)malloc(sizeof(float *));
	float * d_var_81_17;
	cudaMalloc((void **)&d_var_81_17, sizeof(float *));
	
	float * h_var_81_18 = (float *)malloc(sizeof(float *));
	float * d_var_81_18;
	cudaMalloc((void **)&d_var_81_18, sizeof(float *));
	
	float * h_var_81_19 = (float *)malloc(sizeof(float *));
	float * d_var_81_19;
	cudaMalloc((void **)&d_var_81_19, sizeof(float *));
	
	float * h_var_82_0 = (float *)malloc(sizeof(float *));
	float * d_var_82_0;
	cudaMalloc((void **)&d_var_82_0, sizeof(float *));
	
	float * h_var_82_1 = (float *)malloc(sizeof(float *));
	float * d_var_82_1;
	cudaMalloc((void **)&d_var_82_1, sizeof(float *));
	
	float * h_var_82_2 = (float *)malloc(sizeof(float *));
	float * d_var_82_2;
	cudaMalloc((void **)&d_var_82_2, sizeof(float *));
	
	float * h_var_82_3 = (float *)malloc(sizeof(float *));
	float * d_var_82_3;
	cudaMalloc((void **)&d_var_82_3, sizeof(float *));
	
	float * h_var_82_4 = (float *)malloc(sizeof(float *));
	float * d_var_82_4;
	cudaMalloc((void **)&d_var_82_4, sizeof(float *));
	
	float * h_var_82_5 = (float *)malloc(sizeof(float *));
	float * d_var_82_5;
	cudaMalloc((void **)&d_var_82_5, sizeof(float *));
	
	float * h_var_82_6 = (float *)malloc(sizeof(float *));
	float * d_var_82_6;
	cudaMalloc((void **)&d_var_82_6, sizeof(float *));
	
	float * h_var_82_7 = (float *)malloc(sizeof(float *));
	float * d_var_82_7;
	cudaMalloc((void **)&d_var_82_7, sizeof(float *));
	
	float * h_var_82_8 = (float *)malloc(sizeof(float *));
	float * d_var_82_8;
	cudaMalloc((void **)&d_var_82_8, sizeof(float *));
	
	float * h_var_82_9 = (float *)malloc(sizeof(float *));
	float * d_var_82_9;
	cudaMalloc((void **)&d_var_82_9, sizeof(float *));
	
	float * h_var_82_10 = (float *)malloc(sizeof(float *));
	float * d_var_82_10;
	cudaMalloc((void **)&d_var_82_10, sizeof(float *));
	
	float * h_var_82_11 = (float *)malloc(sizeof(float *));
	float * d_var_82_11;
	cudaMalloc((void **)&d_var_82_11, sizeof(float *));
	
	float * h_var_82_12 = (float *)malloc(sizeof(float *));
	float * d_var_82_12;
	cudaMalloc((void **)&d_var_82_12, sizeof(float *));
	
	float * h_var_82_13 = (float *)malloc(sizeof(float *));
	float * d_var_82_13;
	cudaMalloc((void **)&d_var_82_13, sizeof(float *));
	
	float * h_var_82_14 = (float *)malloc(sizeof(float *));
	float * d_var_82_14;
	cudaMalloc((void **)&d_var_82_14, sizeof(float *));
	
	float * h_var_82_15 = (float *)malloc(sizeof(float *));
	float * d_var_82_15;
	cudaMalloc((void **)&d_var_82_15, sizeof(float *));
	
	float * h_var_82_16 = (float *)malloc(sizeof(float *));
	float * d_var_82_16;
	cudaMalloc((void **)&d_var_82_16, sizeof(float *));
	
	float * h_var_82_17 = (float *)malloc(sizeof(float *));
	float * d_var_82_17;
	cudaMalloc((void **)&d_var_82_17, sizeof(float *));
	
	float * h_var_82_18 = (float *)malloc(sizeof(float *));
	float * d_var_82_18;
	cudaMalloc((void **)&d_var_82_18, sizeof(float *));
	
	float * h_var_82_19 = (float *)malloc(sizeof(float *));
	float * d_var_82_19;
	cudaMalloc((void **)&d_var_82_19, sizeof(float *));
	
	float * h_var_83_0 = (float *)malloc(sizeof(float *));
	float * d_var_83_0;
	cudaMalloc((void **)&d_var_83_0, sizeof(float *));
	
	float * h_var_83_1 = (float *)malloc(sizeof(float *));
	float * d_var_83_1;
	cudaMalloc((void **)&d_var_83_1, sizeof(float *));
	
	float * h_var_83_2 = (float *)malloc(sizeof(float *));
	float * d_var_83_2;
	cudaMalloc((void **)&d_var_83_2, sizeof(float *));
	
	float * h_var_83_3 = (float *)malloc(sizeof(float *));
	float * d_var_83_3;
	cudaMalloc((void **)&d_var_83_3, sizeof(float *));
	
	float * h_var_83_4 = (float *)malloc(sizeof(float *));
	float * d_var_83_4;
	cudaMalloc((void **)&d_var_83_4, sizeof(float *));
	
	float * h_var_83_5 = (float *)malloc(sizeof(float *));
	float * d_var_83_5;
	cudaMalloc((void **)&d_var_83_5, sizeof(float *));
	
	float * h_var_83_6 = (float *)malloc(sizeof(float *));
	float * d_var_83_6;
	cudaMalloc((void **)&d_var_83_6, sizeof(float *));
	
	float * h_var_83_7 = (float *)malloc(sizeof(float *));
	float * d_var_83_7;
	cudaMalloc((void **)&d_var_83_7, sizeof(float *));
	
	float * h_var_83_8 = (float *)malloc(sizeof(float *));
	float * d_var_83_8;
	cudaMalloc((void **)&d_var_83_8, sizeof(float *));
	
	float * h_var_83_9 = (float *)malloc(sizeof(float *));
	float * d_var_83_9;
	cudaMalloc((void **)&d_var_83_9, sizeof(float *));
	
	float * h_var_83_10 = (float *)malloc(sizeof(float *));
	float * d_var_83_10;
	cudaMalloc((void **)&d_var_83_10, sizeof(float *));
	
	float * h_var_83_11 = (float *)malloc(sizeof(float *));
	float * d_var_83_11;
	cudaMalloc((void **)&d_var_83_11, sizeof(float *));
	
	float * h_var_83_12 = (float *)malloc(sizeof(float *));
	float * d_var_83_12;
	cudaMalloc((void **)&d_var_83_12, sizeof(float *));
	
	float * h_var_83_13 = (float *)malloc(sizeof(float *));
	float * d_var_83_13;
	cudaMalloc((void **)&d_var_83_13, sizeof(float *));
	
	float * h_var_83_14 = (float *)malloc(sizeof(float *));
	float * d_var_83_14;
	cudaMalloc((void **)&d_var_83_14, sizeof(float *));
	
	float * h_var_83_15 = (float *)malloc(sizeof(float *));
	float * d_var_83_15;
	cudaMalloc((void **)&d_var_83_15, sizeof(float *));
	
	float * h_var_83_16 = (float *)malloc(sizeof(float *));
	float * d_var_83_16;
	cudaMalloc((void **)&d_var_83_16, sizeof(float *));
	
	float * h_var_83_17 = (float *)malloc(sizeof(float *));
	float * d_var_83_17;
	cudaMalloc((void **)&d_var_83_17, sizeof(float *));
	
	float * h_var_83_18 = (float *)malloc(sizeof(float *));
	float * d_var_83_18;
	cudaMalloc((void **)&d_var_83_18, sizeof(float *));
	
	float * h_var_83_19 = (float *)malloc(sizeof(float *));
	float * d_var_83_19;
	cudaMalloc((void **)&d_var_83_19, sizeof(float *));
	
	float * h_var_84_0 = (float *)malloc(sizeof(float *));
	float * d_var_84_0;
	cudaMalloc((void **)&d_var_84_0, sizeof(float *));
	
	float * h_var_84_1 = (float *)malloc(sizeof(float *));
	float * d_var_84_1;
	cudaMalloc((void **)&d_var_84_1, sizeof(float *));
	
	float * h_var_84_2 = (float *)malloc(sizeof(float *));
	float * d_var_84_2;
	cudaMalloc((void **)&d_var_84_2, sizeof(float *));
	
	float * h_var_84_3 = (float *)malloc(sizeof(float *));
	float * d_var_84_3;
	cudaMalloc((void **)&d_var_84_3, sizeof(float *));
	
	float * h_var_84_4 = (float *)malloc(sizeof(float *));
	float * d_var_84_4;
	cudaMalloc((void **)&d_var_84_4, sizeof(float *));
	
	float * h_var_84_5 = (float *)malloc(sizeof(float *));
	float * d_var_84_5;
	cudaMalloc((void **)&d_var_84_5, sizeof(float *));
	
	float * h_var_84_6 = (float *)malloc(sizeof(float *));
	float * d_var_84_6;
	cudaMalloc((void **)&d_var_84_6, sizeof(float *));
	
	float * h_var_84_7 = (float *)malloc(sizeof(float *));
	float * d_var_84_7;
	cudaMalloc((void **)&d_var_84_7, sizeof(float *));
	
	float * h_var_84_8 = (float *)malloc(sizeof(float *));
	float * d_var_84_8;
	cudaMalloc((void **)&d_var_84_8, sizeof(float *));
	
	float * h_var_84_9 = (float *)malloc(sizeof(float *));
	float * d_var_84_9;
	cudaMalloc((void **)&d_var_84_9, sizeof(float *));
	
	float * h_var_84_10 = (float *)malloc(sizeof(float *));
	float * d_var_84_10;
	cudaMalloc((void **)&d_var_84_10, sizeof(float *));
	
	float * h_var_84_11 = (float *)malloc(sizeof(float *));
	float * d_var_84_11;
	cudaMalloc((void **)&d_var_84_11, sizeof(float *));
	
	float * h_var_84_12 = (float *)malloc(sizeof(float *));
	float * d_var_84_12;
	cudaMalloc((void **)&d_var_84_12, sizeof(float *));
	
	float * h_var_84_13 = (float *)malloc(sizeof(float *));
	float * d_var_84_13;
	cudaMalloc((void **)&d_var_84_13, sizeof(float *));
	
	float * h_var_84_14 = (float *)malloc(sizeof(float *));
	float * d_var_84_14;
	cudaMalloc((void **)&d_var_84_14, sizeof(float *));
	
	float * h_var_84_15 = (float *)malloc(sizeof(float *));
	float * d_var_84_15;
	cudaMalloc((void **)&d_var_84_15, sizeof(float *));
	
	float * h_var_84_16 = (float *)malloc(sizeof(float *));
	float * d_var_84_16;
	cudaMalloc((void **)&d_var_84_16, sizeof(float *));
	
	float * h_var_84_17 = (float *)malloc(sizeof(float *));
	float * d_var_84_17;
	cudaMalloc((void **)&d_var_84_17, sizeof(float *));
	
	float * h_var_84_18 = (float *)malloc(sizeof(float *));
	float * d_var_84_18;
	cudaMalloc((void **)&d_var_84_18, sizeof(float *));
	
	float * h_var_84_19 = (float *)malloc(sizeof(float *));
	float * d_var_84_19;
	cudaMalloc((void **)&d_var_84_19, sizeof(float *));
	
	float * h_var_85_0 = (float *)malloc(sizeof(float *));
	float * d_var_85_0;
	cudaMalloc((void **)&d_var_85_0, sizeof(float *));
	
	float * h_var_85_1 = (float *)malloc(sizeof(float *));
	float * d_var_85_1;
	cudaMalloc((void **)&d_var_85_1, sizeof(float *));
	
	float * h_var_85_2 = (float *)malloc(sizeof(float *));
	float * d_var_85_2;
	cudaMalloc((void **)&d_var_85_2, sizeof(float *));
	
	float * h_var_85_3 = (float *)malloc(sizeof(float *));
	float * d_var_85_3;
	cudaMalloc((void **)&d_var_85_3, sizeof(float *));
	
	float * h_var_85_4 = (float *)malloc(sizeof(float *));
	float * d_var_85_4;
	cudaMalloc((void **)&d_var_85_4, sizeof(float *));
	
	float * h_var_85_5 = (float *)malloc(sizeof(float *));
	float * d_var_85_5;
	cudaMalloc((void **)&d_var_85_5, sizeof(float *));
	
	float * h_var_85_6 = (float *)malloc(sizeof(float *));
	float * d_var_85_6;
	cudaMalloc((void **)&d_var_85_6, sizeof(float *));
	
	float * h_var_85_7 = (float *)malloc(sizeof(float *));
	float * d_var_85_7;
	cudaMalloc((void **)&d_var_85_7, sizeof(float *));
	
	float * h_var_85_8 = (float *)malloc(sizeof(float *));
	float * d_var_85_8;
	cudaMalloc((void **)&d_var_85_8, sizeof(float *));
	
	float * h_var_85_9 = (float *)malloc(sizeof(float *));
	float * d_var_85_9;
	cudaMalloc((void **)&d_var_85_9, sizeof(float *));
	
	float * h_var_85_10 = (float *)malloc(sizeof(float *));
	float * d_var_85_10;
	cudaMalloc((void **)&d_var_85_10, sizeof(float *));
	
	float * h_var_85_11 = (float *)malloc(sizeof(float *));
	float * d_var_85_11;
	cudaMalloc((void **)&d_var_85_11, sizeof(float *));
	
	float * h_var_85_12 = (float *)malloc(sizeof(float *));
	float * d_var_85_12;
	cudaMalloc((void **)&d_var_85_12, sizeof(float *));
	
	float * h_var_85_13 = (float *)malloc(sizeof(float *));
	float * d_var_85_13;
	cudaMalloc((void **)&d_var_85_13, sizeof(float *));
	
	float * h_var_85_14 = (float *)malloc(sizeof(float *));
	float * d_var_85_14;
	cudaMalloc((void **)&d_var_85_14, sizeof(float *));
	
	float * h_var_85_15 = (float *)malloc(sizeof(float *));
	float * d_var_85_15;
	cudaMalloc((void **)&d_var_85_15, sizeof(float *));
	
	float * h_var_85_16 = (float *)malloc(sizeof(float *));
	float * d_var_85_16;
	cudaMalloc((void **)&d_var_85_16, sizeof(float *));
	
	float * h_var_85_17 = (float *)malloc(sizeof(float *));
	float * d_var_85_17;
	cudaMalloc((void **)&d_var_85_17, sizeof(float *));
	
	float * h_var_85_18 = (float *)malloc(sizeof(float *));
	float * d_var_85_18;
	cudaMalloc((void **)&d_var_85_18, sizeof(float *));
	
	float * h_var_85_19 = (float *)malloc(sizeof(float *));
	float * d_var_85_19;
	cudaMalloc((void **)&d_var_85_19, sizeof(float *));
	
	float * h_var_86_0 = (float *)malloc(sizeof(float *));
	float * d_var_86_0;
	cudaMalloc((void **)&d_var_86_0, sizeof(float *));
	
	float * h_var_86_1 = (float *)malloc(sizeof(float *));
	float * d_var_86_1;
	cudaMalloc((void **)&d_var_86_1, sizeof(float *));
	
	float * h_var_86_2 = (float *)malloc(sizeof(float *));
	float * d_var_86_2;
	cudaMalloc((void **)&d_var_86_2, sizeof(float *));
	
	float * h_var_86_3 = (float *)malloc(sizeof(float *));
	float * d_var_86_3;
	cudaMalloc((void **)&d_var_86_3, sizeof(float *));
	
	float * h_var_86_4 = (float *)malloc(sizeof(float *));
	float * d_var_86_4;
	cudaMalloc((void **)&d_var_86_4, sizeof(float *));
	
	float * h_var_86_5 = (float *)malloc(sizeof(float *));
	float * d_var_86_5;
	cudaMalloc((void **)&d_var_86_5, sizeof(float *));
	
	float * h_var_86_6 = (float *)malloc(sizeof(float *));
	float * d_var_86_6;
	cudaMalloc((void **)&d_var_86_6, sizeof(float *));
	
	float * h_var_86_7 = (float *)malloc(sizeof(float *));
	float * d_var_86_7;
	cudaMalloc((void **)&d_var_86_7, sizeof(float *));
	
	float * h_var_86_8 = (float *)malloc(sizeof(float *));
	float * d_var_86_8;
	cudaMalloc((void **)&d_var_86_8, sizeof(float *));
	
	float * h_var_86_9 = (float *)malloc(sizeof(float *));
	float * d_var_86_9;
	cudaMalloc((void **)&d_var_86_9, sizeof(float *));
	
	float * h_var_86_10 = (float *)malloc(sizeof(float *));
	float * d_var_86_10;
	cudaMalloc((void **)&d_var_86_10, sizeof(float *));
	
	float * h_var_86_11 = (float *)malloc(sizeof(float *));
	float * d_var_86_11;
	cudaMalloc((void **)&d_var_86_11, sizeof(float *));
	
	float * h_var_86_12 = (float *)malloc(sizeof(float *));
	float * d_var_86_12;
	cudaMalloc((void **)&d_var_86_12, sizeof(float *));
	
	float * h_var_86_13 = (float *)malloc(sizeof(float *));
	float * d_var_86_13;
	cudaMalloc((void **)&d_var_86_13, sizeof(float *));
	
	float * h_var_86_14 = (float *)malloc(sizeof(float *));
	float * d_var_86_14;
	cudaMalloc((void **)&d_var_86_14, sizeof(float *));
	
	float * h_var_86_15 = (float *)malloc(sizeof(float *));
	float * d_var_86_15;
	cudaMalloc((void **)&d_var_86_15, sizeof(float *));
	
	float * h_var_86_16 = (float *)malloc(sizeof(float *));
	float * d_var_86_16;
	cudaMalloc((void **)&d_var_86_16, sizeof(float *));
	
	float * h_var_86_17 = (float *)malloc(sizeof(float *));
	float * d_var_86_17;
	cudaMalloc((void **)&d_var_86_17, sizeof(float *));
	
	float * h_var_86_18 = (float *)malloc(sizeof(float *));
	float * d_var_86_18;
	cudaMalloc((void **)&d_var_86_18, sizeof(float *));
	
	float * h_var_86_19 = (float *)malloc(sizeof(float *));
	float * d_var_86_19;
	cudaMalloc((void **)&d_var_86_19, sizeof(float *));
	
	float * h_var_87_0 = (float *)malloc(sizeof(float *));
	float * d_var_87_0;
	cudaMalloc((void **)&d_var_87_0, sizeof(float *));
	
	float * h_var_87_1 = (float *)malloc(sizeof(float *));
	float * d_var_87_1;
	cudaMalloc((void **)&d_var_87_1, sizeof(float *));
	
	float * h_var_87_2 = (float *)malloc(sizeof(float *));
	float * d_var_87_2;
	cudaMalloc((void **)&d_var_87_2, sizeof(float *));
	
	float * h_var_87_3 = (float *)malloc(sizeof(float *));
	float * d_var_87_3;
	cudaMalloc((void **)&d_var_87_3, sizeof(float *));
	
	float * h_var_87_4 = (float *)malloc(sizeof(float *));
	float * d_var_87_4;
	cudaMalloc((void **)&d_var_87_4, sizeof(float *));
	
	float * h_var_87_5 = (float *)malloc(sizeof(float *));
	float * d_var_87_5;
	cudaMalloc((void **)&d_var_87_5, sizeof(float *));
	
	float * h_var_87_6 = (float *)malloc(sizeof(float *));
	float * d_var_87_6;
	cudaMalloc((void **)&d_var_87_6, sizeof(float *));
	
	float * h_var_87_7 = (float *)malloc(sizeof(float *));
	float * d_var_87_7;
	cudaMalloc((void **)&d_var_87_7, sizeof(float *));
	
	float * h_var_87_8 = (float *)malloc(sizeof(float *));
	float * d_var_87_8;
	cudaMalloc((void **)&d_var_87_8, sizeof(float *));
	
	float * h_var_87_9 = (float *)malloc(sizeof(float *));
	float * d_var_87_9;
	cudaMalloc((void **)&d_var_87_9, sizeof(float *));
	
	float * h_var_87_10 = (float *)malloc(sizeof(float *));
	float * d_var_87_10;
	cudaMalloc((void **)&d_var_87_10, sizeof(float *));
	
	float * h_var_87_11 = (float *)malloc(sizeof(float *));
	float * d_var_87_11;
	cudaMalloc((void **)&d_var_87_11, sizeof(float *));
	
	float * h_var_87_12 = (float *)malloc(sizeof(float *));
	float * d_var_87_12;
	cudaMalloc((void **)&d_var_87_12, sizeof(float *));
	
	float * h_var_87_13 = (float *)malloc(sizeof(float *));
	float * d_var_87_13;
	cudaMalloc((void **)&d_var_87_13, sizeof(float *));
	
	float * h_var_87_14 = (float *)malloc(sizeof(float *));
	float * d_var_87_14;
	cudaMalloc((void **)&d_var_87_14, sizeof(float *));
	
	float * h_var_87_15 = (float *)malloc(sizeof(float *));
	float * d_var_87_15;
	cudaMalloc((void **)&d_var_87_15, sizeof(float *));
	
	float * h_var_87_16 = (float *)malloc(sizeof(float *));
	float * d_var_87_16;
	cudaMalloc((void **)&d_var_87_16, sizeof(float *));
	
	float * h_var_87_17 = (float *)malloc(sizeof(float *));
	float * d_var_87_17;
	cudaMalloc((void **)&d_var_87_17, sizeof(float *));
	
	float * h_var_87_18 = (float *)malloc(sizeof(float *));
	float * d_var_87_18;
	cudaMalloc((void **)&d_var_87_18, sizeof(float *));
	
	float * h_var_87_19 = (float *)malloc(sizeof(float *));
	float * d_var_87_19;
	cudaMalloc((void **)&d_var_87_19, sizeof(float *));
	
	float * h_var_88_0 = (float *)malloc(sizeof(float *));
	float * d_var_88_0;
	cudaMalloc((void **)&d_var_88_0, sizeof(float *));
	
	float * h_var_88_1 = (float *)malloc(sizeof(float *));
	float * d_var_88_1;
	cudaMalloc((void **)&d_var_88_1, sizeof(float *));
	
	float * h_var_88_2 = (float *)malloc(sizeof(float *));
	float * d_var_88_2;
	cudaMalloc((void **)&d_var_88_2, sizeof(float *));
	
	float * h_var_88_3 = (float *)malloc(sizeof(float *));
	float * d_var_88_3;
	cudaMalloc((void **)&d_var_88_3, sizeof(float *));
	
	float * h_var_88_4 = (float *)malloc(sizeof(float *));
	float * d_var_88_4;
	cudaMalloc((void **)&d_var_88_4, sizeof(float *));
	
	float * h_var_88_5 = (float *)malloc(sizeof(float *));
	float * d_var_88_5;
	cudaMalloc((void **)&d_var_88_5, sizeof(float *));
	
	float * h_var_88_6 = (float *)malloc(sizeof(float *));
	float * d_var_88_6;
	cudaMalloc((void **)&d_var_88_6, sizeof(float *));
	
	float * h_var_88_7 = (float *)malloc(sizeof(float *));
	float * d_var_88_7;
	cudaMalloc((void **)&d_var_88_7, sizeof(float *));
	
	float * h_var_88_8 = (float *)malloc(sizeof(float *));
	float * d_var_88_8;
	cudaMalloc((void **)&d_var_88_8, sizeof(float *));
	
	float * h_var_88_9 = (float *)malloc(sizeof(float *));
	float * d_var_88_9;
	cudaMalloc((void **)&d_var_88_9, sizeof(float *));
	
	float * h_var_88_10 = (float *)malloc(sizeof(float *));
	float * d_var_88_10;
	cudaMalloc((void **)&d_var_88_10, sizeof(float *));
	
	float * h_var_88_11 = (float *)malloc(sizeof(float *));
	float * d_var_88_11;
	cudaMalloc((void **)&d_var_88_11, sizeof(float *));
	
	float * h_var_88_12 = (float *)malloc(sizeof(float *));
	float * d_var_88_12;
	cudaMalloc((void **)&d_var_88_12, sizeof(float *));
	
	float * h_var_88_13 = (float *)malloc(sizeof(float *));
	float * d_var_88_13;
	cudaMalloc((void **)&d_var_88_13, sizeof(float *));
	
	float * h_var_88_14 = (float *)malloc(sizeof(float *));
	float * d_var_88_14;
	cudaMalloc((void **)&d_var_88_14, sizeof(float *));
	
	float * h_var_88_15 = (float *)malloc(sizeof(float *));
	float * d_var_88_15;
	cudaMalloc((void **)&d_var_88_15, sizeof(float *));
	
	float * h_var_88_16 = (float *)malloc(sizeof(float *));
	float * d_var_88_16;
	cudaMalloc((void **)&d_var_88_16, sizeof(float *));
	
	float * h_var_88_17 = (float *)malloc(sizeof(float *));
	float * d_var_88_17;
	cudaMalloc((void **)&d_var_88_17, sizeof(float *));
	
	float * h_var_88_18 = (float *)malloc(sizeof(float *));
	float * d_var_88_18;
	cudaMalloc((void **)&d_var_88_18, sizeof(float *));
	
	float * h_var_88_19 = (float *)malloc(sizeof(float *));
	float * d_var_88_19;
	cudaMalloc((void **)&d_var_88_19, sizeof(float *));
	
	float * h_var_89_0 = (float *)malloc(sizeof(float *));
	float * d_var_89_0;
	cudaMalloc((void **)&d_var_89_0, sizeof(float *));
	
	float * h_var_89_1 = (float *)malloc(sizeof(float *));
	float * d_var_89_1;
	cudaMalloc((void **)&d_var_89_1, sizeof(float *));
	
	float * h_var_89_2 = (float *)malloc(sizeof(float *));
	float * d_var_89_2;
	cudaMalloc((void **)&d_var_89_2, sizeof(float *));
	
	float * h_var_89_3 = (float *)malloc(sizeof(float *));
	float * d_var_89_3;
	cudaMalloc((void **)&d_var_89_3, sizeof(float *));
	
	float * h_var_89_4 = (float *)malloc(sizeof(float *));
	float * d_var_89_4;
	cudaMalloc((void **)&d_var_89_4, sizeof(float *));
	
	float * h_var_89_5 = (float *)malloc(sizeof(float *));
	float * d_var_89_5;
	cudaMalloc((void **)&d_var_89_5, sizeof(float *));
	
	float * h_var_89_6 = (float *)malloc(sizeof(float *));
	float * d_var_89_6;
	cudaMalloc((void **)&d_var_89_6, sizeof(float *));
	
	float * h_var_89_7 = (float *)malloc(sizeof(float *));
	float * d_var_89_7;
	cudaMalloc((void **)&d_var_89_7, sizeof(float *));
	
	float * h_var_89_8 = (float *)malloc(sizeof(float *));
	float * d_var_89_8;
	cudaMalloc((void **)&d_var_89_8, sizeof(float *));
	
	float * h_var_89_9 = (float *)malloc(sizeof(float *));
	float * d_var_89_9;
	cudaMalloc((void **)&d_var_89_9, sizeof(float *));
	
	float * h_var_89_10 = (float *)malloc(sizeof(float *));
	float * d_var_89_10;
	cudaMalloc((void **)&d_var_89_10, sizeof(float *));
	
	float * h_var_89_11 = (float *)malloc(sizeof(float *));
	float * d_var_89_11;
	cudaMalloc((void **)&d_var_89_11, sizeof(float *));
	
	float * h_var_89_12 = (float *)malloc(sizeof(float *));
	float * d_var_89_12;
	cudaMalloc((void **)&d_var_89_12, sizeof(float *));
	
	float * h_var_89_13 = (float *)malloc(sizeof(float *));
	float * d_var_89_13;
	cudaMalloc((void **)&d_var_89_13, sizeof(float *));
	
	float * h_var_89_14 = (float *)malloc(sizeof(float *));
	float * d_var_89_14;
	cudaMalloc((void **)&d_var_89_14, sizeof(float *));
	
	float * h_var_89_15 = (float *)malloc(sizeof(float *));
	float * d_var_89_15;
	cudaMalloc((void **)&d_var_89_15, sizeof(float *));
	
	float * h_var_89_16 = (float *)malloc(sizeof(float *));
	float * d_var_89_16;
	cudaMalloc((void **)&d_var_89_16, sizeof(float *));
	
	float * h_var_89_17 = (float *)malloc(sizeof(float *));
	float * d_var_89_17;
	cudaMalloc((void **)&d_var_89_17, sizeof(float *));
	
	float * h_var_89_18 = (float *)malloc(sizeof(float *));
	float * d_var_89_18;
	cudaMalloc((void **)&d_var_89_18, sizeof(float *));
	
	float * h_var_89_19 = (float *)malloc(sizeof(float *));
	float * d_var_89_19;
	cudaMalloc((void **)&d_var_89_19, sizeof(float *));
	
	float * h_var_90_0 = (float *)malloc(sizeof(float *));
	float * d_var_90_0;
	cudaMalloc((void **)&d_var_90_0, sizeof(float *));
	
	float * h_var_90_1 = (float *)malloc(sizeof(float *));
	float * d_var_90_1;
	cudaMalloc((void **)&d_var_90_1, sizeof(float *));
	
	float * h_var_90_2 = (float *)malloc(sizeof(float *));
	float * d_var_90_2;
	cudaMalloc((void **)&d_var_90_2, sizeof(float *));
	
	float * h_var_90_3 = (float *)malloc(sizeof(float *));
	float * d_var_90_3;
	cudaMalloc((void **)&d_var_90_3, sizeof(float *));
	
	float * h_var_90_4 = (float *)malloc(sizeof(float *));
	float * d_var_90_4;
	cudaMalloc((void **)&d_var_90_4, sizeof(float *));
	
	float * h_var_90_5 = (float *)malloc(sizeof(float *));
	float * d_var_90_5;
	cudaMalloc((void **)&d_var_90_5, sizeof(float *));
	
	float * h_var_90_6 = (float *)malloc(sizeof(float *));
	float * d_var_90_6;
	cudaMalloc((void **)&d_var_90_6, sizeof(float *));
	
	float * h_var_90_7 = (float *)malloc(sizeof(float *));
	float * d_var_90_7;
	cudaMalloc((void **)&d_var_90_7, sizeof(float *));
	
	float * h_var_90_8 = (float *)malloc(sizeof(float *));
	float * d_var_90_8;
	cudaMalloc((void **)&d_var_90_8, sizeof(float *));
	
	float * h_var_90_9 = (float *)malloc(sizeof(float *));
	float * d_var_90_9;
	cudaMalloc((void **)&d_var_90_9, sizeof(float *));
	
	float * h_var_90_10 = (float *)malloc(sizeof(float *));
	float * d_var_90_10;
	cudaMalloc((void **)&d_var_90_10, sizeof(float *));
	
	float * h_var_90_11 = (float *)malloc(sizeof(float *));
	float * d_var_90_11;
	cudaMalloc((void **)&d_var_90_11, sizeof(float *));
	
	float * h_var_90_12 = (float *)malloc(sizeof(float *));
	float * d_var_90_12;
	cudaMalloc((void **)&d_var_90_12, sizeof(float *));
	
	float * h_var_90_13 = (float *)malloc(sizeof(float *));
	float * d_var_90_13;
	cudaMalloc((void **)&d_var_90_13, sizeof(float *));
	
	float * h_var_90_14 = (float *)malloc(sizeof(float *));
	float * d_var_90_14;
	cudaMalloc((void **)&d_var_90_14, sizeof(float *));
	
	float * h_var_90_15 = (float *)malloc(sizeof(float *));
	float * d_var_90_15;
	cudaMalloc((void **)&d_var_90_15, sizeof(float *));
	
	float * h_var_90_16 = (float *)malloc(sizeof(float *));
	float * d_var_90_16;
	cudaMalloc((void **)&d_var_90_16, sizeof(float *));
	
	float * h_var_90_17 = (float *)malloc(sizeof(float *));
	float * d_var_90_17;
	cudaMalloc((void **)&d_var_90_17, sizeof(float *));
	
	float * h_var_90_18 = (float *)malloc(sizeof(float *));
	float * d_var_90_18;
	cudaMalloc((void **)&d_var_90_18, sizeof(float *));
	
	float * h_var_90_19 = (float *)malloc(sizeof(float *));
	float * d_var_90_19;
	cudaMalloc((void **)&d_var_90_19, sizeof(float *));
	
	float * h_var_91_0 = (float *)malloc(sizeof(float *));
	float * d_var_91_0;
	cudaMalloc((void **)&d_var_91_0, sizeof(float *));
	
	float * h_var_91_1 = (float *)malloc(sizeof(float *));
	float * d_var_91_1;
	cudaMalloc((void **)&d_var_91_1, sizeof(float *));
	
	float * h_var_91_2 = (float *)malloc(sizeof(float *));
	float * d_var_91_2;
	cudaMalloc((void **)&d_var_91_2, sizeof(float *));
	
	float * h_var_91_3 = (float *)malloc(sizeof(float *));
	float * d_var_91_3;
	cudaMalloc((void **)&d_var_91_3, sizeof(float *));
	
	float * h_var_91_4 = (float *)malloc(sizeof(float *));
	float * d_var_91_4;
	cudaMalloc((void **)&d_var_91_4, sizeof(float *));
	
	float * h_var_91_5 = (float *)malloc(sizeof(float *));
	float * d_var_91_5;
	cudaMalloc((void **)&d_var_91_5, sizeof(float *));
	
	float * h_var_91_6 = (float *)malloc(sizeof(float *));
	float * d_var_91_6;
	cudaMalloc((void **)&d_var_91_6, sizeof(float *));
	
	float * h_var_91_7 = (float *)malloc(sizeof(float *));
	float * d_var_91_7;
	cudaMalloc((void **)&d_var_91_7, sizeof(float *));
	
	float * h_var_91_8 = (float *)malloc(sizeof(float *));
	float * d_var_91_8;
	cudaMalloc((void **)&d_var_91_8, sizeof(float *));
	
	float * h_var_91_9 = (float *)malloc(sizeof(float *));
	float * d_var_91_9;
	cudaMalloc((void **)&d_var_91_9, sizeof(float *));
	
	float * h_var_91_10 = (float *)malloc(sizeof(float *));
	float * d_var_91_10;
	cudaMalloc((void **)&d_var_91_10, sizeof(float *));
	
	float * h_var_91_11 = (float *)malloc(sizeof(float *));
	float * d_var_91_11;
	cudaMalloc((void **)&d_var_91_11, sizeof(float *));
	
	float * h_var_91_12 = (float *)malloc(sizeof(float *));
	float * d_var_91_12;
	cudaMalloc((void **)&d_var_91_12, sizeof(float *));
	
	float * h_var_91_13 = (float *)malloc(sizeof(float *));
	float * d_var_91_13;
	cudaMalloc((void **)&d_var_91_13, sizeof(float *));
	
	float * h_var_91_14 = (float *)malloc(sizeof(float *));
	float * d_var_91_14;
	cudaMalloc((void **)&d_var_91_14, sizeof(float *));
	
	float * h_var_91_15 = (float *)malloc(sizeof(float *));
	float * d_var_91_15;
	cudaMalloc((void **)&d_var_91_15, sizeof(float *));
	
	float * h_var_91_16 = (float *)malloc(sizeof(float *));
	float * d_var_91_16;
	cudaMalloc((void **)&d_var_91_16, sizeof(float *));
	
	float * h_var_91_17 = (float *)malloc(sizeof(float *));
	float * d_var_91_17;
	cudaMalloc((void **)&d_var_91_17, sizeof(float *));
	
	float * h_var_91_18 = (float *)malloc(sizeof(float *));
	float * d_var_91_18;
	cudaMalloc((void **)&d_var_91_18, sizeof(float *));
	
	float * h_var_91_19 = (float *)malloc(sizeof(float *));
	float * d_var_91_19;
	cudaMalloc((void **)&d_var_91_19, sizeof(float *));
	
	float * h_var_92_0 = (float *)malloc(sizeof(float *));
	float * d_var_92_0;
	cudaMalloc((void **)&d_var_92_0, sizeof(float *));
	
	float * h_var_92_1 = (float *)malloc(sizeof(float *));
	float * d_var_92_1;
	cudaMalloc((void **)&d_var_92_1, sizeof(float *));
	
	float * h_var_92_2 = (float *)malloc(sizeof(float *));
	float * d_var_92_2;
	cudaMalloc((void **)&d_var_92_2, sizeof(float *));
	
	float * h_var_92_3 = (float *)malloc(sizeof(float *));
	float * d_var_92_3;
	cudaMalloc((void **)&d_var_92_3, sizeof(float *));
	
	float * h_var_92_4 = (float *)malloc(sizeof(float *));
	float * d_var_92_4;
	cudaMalloc((void **)&d_var_92_4, sizeof(float *));
	
	float * h_var_92_5 = (float *)malloc(sizeof(float *));
	float * d_var_92_5;
	cudaMalloc((void **)&d_var_92_5, sizeof(float *));
	
	float * h_var_92_6 = (float *)malloc(sizeof(float *));
	float * d_var_92_6;
	cudaMalloc((void **)&d_var_92_6, sizeof(float *));
	
	float * h_var_92_7 = (float *)malloc(sizeof(float *));
	float * d_var_92_7;
	cudaMalloc((void **)&d_var_92_7, sizeof(float *));
	
	float * h_var_92_8 = (float *)malloc(sizeof(float *));
	float * d_var_92_8;
	cudaMalloc((void **)&d_var_92_8, sizeof(float *));
	
	float * h_var_92_9 = (float *)malloc(sizeof(float *));
	float * d_var_92_9;
	cudaMalloc((void **)&d_var_92_9, sizeof(float *));
	
	float * h_var_92_10 = (float *)malloc(sizeof(float *));
	float * d_var_92_10;
	cudaMalloc((void **)&d_var_92_10, sizeof(float *));
	
	float * h_var_92_11 = (float *)malloc(sizeof(float *));
	float * d_var_92_11;
	cudaMalloc((void **)&d_var_92_11, sizeof(float *));
	
	float * h_var_92_12 = (float *)malloc(sizeof(float *));
	float * d_var_92_12;
	cudaMalloc((void **)&d_var_92_12, sizeof(float *));
	
	float * h_var_92_13 = (float *)malloc(sizeof(float *));
	float * d_var_92_13;
	cudaMalloc((void **)&d_var_92_13, sizeof(float *));
	
	float * h_var_92_14 = (float *)malloc(sizeof(float *));
	float * d_var_92_14;
	cudaMalloc((void **)&d_var_92_14, sizeof(float *));
	
	float * h_var_92_15 = (float *)malloc(sizeof(float *));
	float * d_var_92_15;
	cudaMalloc((void **)&d_var_92_15, sizeof(float *));
	
	float * h_var_92_16 = (float *)malloc(sizeof(float *));
	float * d_var_92_16;
	cudaMalloc((void **)&d_var_92_16, sizeof(float *));
	
	float * h_var_92_17 = (float *)malloc(sizeof(float *));
	float * d_var_92_17;
	cudaMalloc((void **)&d_var_92_17, sizeof(float *));
	
	float * h_var_92_18 = (float *)malloc(sizeof(float *));
	float * d_var_92_18;
	cudaMalloc((void **)&d_var_92_18, sizeof(float *));
	
	float * h_var_92_19 = (float *)malloc(sizeof(float *));
	float * d_var_92_19;
	cudaMalloc((void **)&d_var_92_19, sizeof(float *));
	
	float * h_var_93_0 = (float *)malloc(sizeof(float *));
	float * d_var_93_0;
	cudaMalloc((void **)&d_var_93_0, sizeof(float *));
	
	float * h_var_93_1 = (float *)malloc(sizeof(float *));
	float * d_var_93_1;
	cudaMalloc((void **)&d_var_93_1, sizeof(float *));
	
	float * h_var_93_2 = (float *)malloc(sizeof(float *));
	float * d_var_93_2;
	cudaMalloc((void **)&d_var_93_2, sizeof(float *));
	
	float * h_var_93_3 = (float *)malloc(sizeof(float *));
	float * d_var_93_3;
	cudaMalloc((void **)&d_var_93_3, sizeof(float *));
	
	float * h_var_93_4 = (float *)malloc(sizeof(float *));
	float * d_var_93_4;
	cudaMalloc((void **)&d_var_93_4, sizeof(float *));
	
	float * h_var_93_5 = (float *)malloc(sizeof(float *));
	float * d_var_93_5;
	cudaMalloc((void **)&d_var_93_5, sizeof(float *));
	
	float * h_var_93_6 = (float *)malloc(sizeof(float *));
	float * d_var_93_6;
	cudaMalloc((void **)&d_var_93_6, sizeof(float *));
	
	float * h_var_93_7 = (float *)malloc(sizeof(float *));
	float * d_var_93_7;
	cudaMalloc((void **)&d_var_93_7, sizeof(float *));
	
	float * h_var_93_8 = (float *)malloc(sizeof(float *));
	float * d_var_93_8;
	cudaMalloc((void **)&d_var_93_8, sizeof(float *));
	
	float * h_var_93_9 = (float *)malloc(sizeof(float *));
	float * d_var_93_9;
	cudaMalloc((void **)&d_var_93_9, sizeof(float *));
	
	float * h_var_93_10 = (float *)malloc(sizeof(float *));
	float * d_var_93_10;
	cudaMalloc((void **)&d_var_93_10, sizeof(float *));
	
	float * h_var_93_11 = (float *)malloc(sizeof(float *));
	float * d_var_93_11;
	cudaMalloc((void **)&d_var_93_11, sizeof(float *));
	
	float * h_var_93_12 = (float *)malloc(sizeof(float *));
	float * d_var_93_12;
	cudaMalloc((void **)&d_var_93_12, sizeof(float *));
	
	float * h_var_93_13 = (float *)malloc(sizeof(float *));
	float * d_var_93_13;
	cudaMalloc((void **)&d_var_93_13, sizeof(float *));
	
	float * h_var_93_14 = (float *)malloc(sizeof(float *));
	float * d_var_93_14;
	cudaMalloc((void **)&d_var_93_14, sizeof(float *));
	
	float * h_var_93_15 = (float *)malloc(sizeof(float *));
	float * d_var_93_15;
	cudaMalloc((void **)&d_var_93_15, sizeof(float *));
	
	float * h_var_93_16 = (float *)malloc(sizeof(float *));
	float * d_var_93_16;
	cudaMalloc((void **)&d_var_93_16, sizeof(float *));
	
	float * h_var_93_17 = (float *)malloc(sizeof(float *));
	float * d_var_93_17;
	cudaMalloc((void **)&d_var_93_17, sizeof(float *));
	
	float * h_var_93_18 = (float *)malloc(sizeof(float *));
	float * d_var_93_18;
	cudaMalloc((void **)&d_var_93_18, sizeof(float *));
	
	float * h_var_93_19 = (float *)malloc(sizeof(float *));
	float * d_var_93_19;
	cudaMalloc((void **)&d_var_93_19, sizeof(float *));
	
	float * h_var_94_0 = (float *)malloc(sizeof(float *));
	float * d_var_94_0;
	cudaMalloc((void **)&d_var_94_0, sizeof(float *));
	
	float * h_var_94_1 = (float *)malloc(sizeof(float *));
	float * d_var_94_1;
	cudaMalloc((void **)&d_var_94_1, sizeof(float *));
	
	float * h_var_94_2 = (float *)malloc(sizeof(float *));
	float * d_var_94_2;
	cudaMalloc((void **)&d_var_94_2, sizeof(float *));
	
	float * h_var_94_3 = (float *)malloc(sizeof(float *));
	float * d_var_94_3;
	cudaMalloc((void **)&d_var_94_3, sizeof(float *));
	
	float * h_var_94_4 = (float *)malloc(sizeof(float *));
	float * d_var_94_4;
	cudaMalloc((void **)&d_var_94_4, sizeof(float *));
	
	float * h_var_94_5 = (float *)malloc(sizeof(float *));
	float * d_var_94_5;
	cudaMalloc((void **)&d_var_94_5, sizeof(float *));
	
	float * h_var_94_6 = (float *)malloc(sizeof(float *));
	float * d_var_94_6;
	cudaMalloc((void **)&d_var_94_6, sizeof(float *));
	
	float * h_var_94_7 = (float *)malloc(sizeof(float *));
	float * d_var_94_7;
	cudaMalloc((void **)&d_var_94_7, sizeof(float *));
	
	float * h_var_94_8 = (float *)malloc(sizeof(float *));
	float * d_var_94_8;
	cudaMalloc((void **)&d_var_94_8, sizeof(float *));
	
	float * h_var_94_9 = (float *)malloc(sizeof(float *));
	float * d_var_94_9;
	cudaMalloc((void **)&d_var_94_9, sizeof(float *));
	
	float * h_var_94_10 = (float *)malloc(sizeof(float *));
	float * d_var_94_10;
	cudaMalloc((void **)&d_var_94_10, sizeof(float *));
	
	float * h_var_94_11 = (float *)malloc(sizeof(float *));
	float * d_var_94_11;
	cudaMalloc((void **)&d_var_94_11, sizeof(float *));
	
	float * h_var_94_12 = (float *)malloc(sizeof(float *));
	float * d_var_94_12;
	cudaMalloc((void **)&d_var_94_12, sizeof(float *));
	
	float * h_var_94_13 = (float *)malloc(sizeof(float *));
	float * d_var_94_13;
	cudaMalloc((void **)&d_var_94_13, sizeof(float *));
	
	float * h_var_94_14 = (float *)malloc(sizeof(float *));
	float * d_var_94_14;
	cudaMalloc((void **)&d_var_94_14, sizeof(float *));
	
	float * h_var_94_15 = (float *)malloc(sizeof(float *));
	float * d_var_94_15;
	cudaMalloc((void **)&d_var_94_15, sizeof(float *));
	
	float * h_var_94_16 = (float *)malloc(sizeof(float *));
	float * d_var_94_16;
	cudaMalloc((void **)&d_var_94_16, sizeof(float *));
	
	float * h_var_94_17 = (float *)malloc(sizeof(float *));
	float * d_var_94_17;
	cudaMalloc((void **)&d_var_94_17, sizeof(float *));
	
	float * h_var_94_18 = (float *)malloc(sizeof(float *));
	float * d_var_94_18;
	cudaMalloc((void **)&d_var_94_18, sizeof(float *));
	
	float * h_var_94_19 = (float *)malloc(sizeof(float *));
	float * d_var_94_19;
	cudaMalloc((void **)&d_var_94_19, sizeof(float *));
	
	float * h_var_95_0 = (float *)malloc(sizeof(float *));
	float * d_var_95_0;
	cudaMalloc((void **)&d_var_95_0, sizeof(float *));
	
	float * h_var_95_1 = (float *)malloc(sizeof(float *));
	float * d_var_95_1;
	cudaMalloc((void **)&d_var_95_1, sizeof(float *));
	
	float * h_var_95_2 = (float *)malloc(sizeof(float *));
	float * d_var_95_2;
	cudaMalloc((void **)&d_var_95_2, sizeof(float *));
	
	float * h_var_95_3 = (float *)malloc(sizeof(float *));
	float * d_var_95_3;
	cudaMalloc((void **)&d_var_95_3, sizeof(float *));
	
	float * h_var_95_4 = (float *)malloc(sizeof(float *));
	float * d_var_95_4;
	cudaMalloc((void **)&d_var_95_4, sizeof(float *));
	
	float * h_var_95_5 = (float *)malloc(sizeof(float *));
	float * d_var_95_5;
	cudaMalloc((void **)&d_var_95_5, sizeof(float *));
	
	float * h_var_95_6 = (float *)malloc(sizeof(float *));
	float * d_var_95_6;
	cudaMalloc((void **)&d_var_95_6, sizeof(float *));
	
	float * h_var_95_7 = (float *)malloc(sizeof(float *));
	float * d_var_95_7;
	cudaMalloc((void **)&d_var_95_7, sizeof(float *));
	
	float * h_var_95_8 = (float *)malloc(sizeof(float *));
	float * d_var_95_8;
	cudaMalloc((void **)&d_var_95_8, sizeof(float *));
	
	float * h_var_95_9 = (float *)malloc(sizeof(float *));
	float * d_var_95_9;
	cudaMalloc((void **)&d_var_95_9, sizeof(float *));
	
	float * h_var_95_10 = (float *)malloc(sizeof(float *));
	float * d_var_95_10;
	cudaMalloc((void **)&d_var_95_10, sizeof(float *));
	
	float * h_var_95_11 = (float *)malloc(sizeof(float *));
	float * d_var_95_11;
	cudaMalloc((void **)&d_var_95_11, sizeof(float *));
	
	float * h_var_95_12 = (float *)malloc(sizeof(float *));
	float * d_var_95_12;
	cudaMalloc((void **)&d_var_95_12, sizeof(float *));
	
	float * h_var_95_13 = (float *)malloc(sizeof(float *));
	float * d_var_95_13;
	cudaMalloc((void **)&d_var_95_13, sizeof(float *));
	
	float * h_var_95_14 = (float *)malloc(sizeof(float *));
	float * d_var_95_14;
	cudaMalloc((void **)&d_var_95_14, sizeof(float *));
	
	float * h_var_95_15 = (float *)malloc(sizeof(float *));
	float * d_var_95_15;
	cudaMalloc((void **)&d_var_95_15, sizeof(float *));
	
	float * h_var_95_16 = (float *)malloc(sizeof(float *));
	float * d_var_95_16;
	cudaMalloc((void **)&d_var_95_16, sizeof(float *));
	
	float * h_var_95_17 = (float *)malloc(sizeof(float *));
	float * d_var_95_17;
	cudaMalloc((void **)&d_var_95_17, sizeof(float *));
	
	float * h_var_95_18 = (float *)malloc(sizeof(float *));
	float * d_var_95_18;
	cudaMalloc((void **)&d_var_95_18, sizeof(float *));
	
	float * h_var_95_19 = (float *)malloc(sizeof(float *));
	float * d_var_95_19;
	cudaMalloc((void **)&d_var_95_19, sizeof(float *));
	
	float * h_var_96_0 = (float *)malloc(sizeof(float *));
	float * d_var_96_0;
	cudaMalloc((void **)&d_var_96_0, sizeof(float *));
	
	float * h_var_96_1 = (float *)malloc(sizeof(float *));
	float * d_var_96_1;
	cudaMalloc((void **)&d_var_96_1, sizeof(float *));
	
	float * h_var_96_2 = (float *)malloc(sizeof(float *));
	float * d_var_96_2;
	cudaMalloc((void **)&d_var_96_2, sizeof(float *));
	
	float * h_var_96_3 = (float *)malloc(sizeof(float *));
	float * d_var_96_3;
	cudaMalloc((void **)&d_var_96_3, sizeof(float *));
	
	float * h_var_96_4 = (float *)malloc(sizeof(float *));
	float * d_var_96_4;
	cudaMalloc((void **)&d_var_96_4, sizeof(float *));
	
	float * h_var_96_5 = (float *)malloc(sizeof(float *));
	float * d_var_96_5;
	cudaMalloc((void **)&d_var_96_5, sizeof(float *));
	
	float * h_var_96_6 = (float *)malloc(sizeof(float *));
	float * d_var_96_6;
	cudaMalloc((void **)&d_var_96_6, sizeof(float *));
	
	float * h_var_96_7 = (float *)malloc(sizeof(float *));
	float * d_var_96_7;
	cudaMalloc((void **)&d_var_96_7, sizeof(float *));
	
	float * h_var_96_8 = (float *)malloc(sizeof(float *));
	float * d_var_96_8;
	cudaMalloc((void **)&d_var_96_8, sizeof(float *));
	
	float * h_var_96_9 = (float *)malloc(sizeof(float *));
	float * d_var_96_9;
	cudaMalloc((void **)&d_var_96_9, sizeof(float *));
	
	float * h_var_96_10 = (float *)malloc(sizeof(float *));
	float * d_var_96_10;
	cudaMalloc((void **)&d_var_96_10, sizeof(float *));
	
	float * h_var_96_11 = (float *)malloc(sizeof(float *));
	float * d_var_96_11;
	cudaMalloc((void **)&d_var_96_11, sizeof(float *));
	
	float * h_var_96_12 = (float *)malloc(sizeof(float *));
	float * d_var_96_12;
	cudaMalloc((void **)&d_var_96_12, sizeof(float *));
	
	float * h_var_96_13 = (float *)malloc(sizeof(float *));
	float * d_var_96_13;
	cudaMalloc((void **)&d_var_96_13, sizeof(float *));
	
	float * h_var_96_14 = (float *)malloc(sizeof(float *));
	float * d_var_96_14;
	cudaMalloc((void **)&d_var_96_14, sizeof(float *));
	
	float * h_var_96_15 = (float *)malloc(sizeof(float *));
	float * d_var_96_15;
	cudaMalloc((void **)&d_var_96_15, sizeof(float *));
	
	float * h_var_96_16 = (float *)malloc(sizeof(float *));
	float * d_var_96_16;
	cudaMalloc((void **)&d_var_96_16, sizeof(float *));
	
	float * h_var_96_17 = (float *)malloc(sizeof(float *));
	float * d_var_96_17;
	cudaMalloc((void **)&d_var_96_17, sizeof(float *));
	
	float * h_var_96_18 = (float *)malloc(sizeof(float *));
	float * d_var_96_18;
	cudaMalloc((void **)&d_var_96_18, sizeof(float *));
	
	float * h_var_96_19 = (float *)malloc(sizeof(float *));
	float * d_var_96_19;
	cudaMalloc((void **)&d_var_96_19, sizeof(float *));
	
	float * h_var_97_0 = (float *)malloc(sizeof(float *));
	float * d_var_97_0;
	cudaMalloc((void **)&d_var_97_0, sizeof(float *));
	
	float * h_var_97_1 = (float *)malloc(sizeof(float *));
	float * d_var_97_1;
	cudaMalloc((void **)&d_var_97_1, sizeof(float *));
	
	float * h_var_97_2 = (float *)malloc(sizeof(float *));
	float * d_var_97_2;
	cudaMalloc((void **)&d_var_97_2, sizeof(float *));
	
	float * h_var_97_3 = (float *)malloc(sizeof(float *));
	float * d_var_97_3;
	cudaMalloc((void **)&d_var_97_3, sizeof(float *));
	
	float * h_var_97_4 = (float *)malloc(sizeof(float *));
	float * d_var_97_4;
	cudaMalloc((void **)&d_var_97_4, sizeof(float *));
	
	float * h_var_97_5 = (float *)malloc(sizeof(float *));
	float * d_var_97_5;
	cudaMalloc((void **)&d_var_97_5, sizeof(float *));
	
	float * h_var_97_6 = (float *)malloc(sizeof(float *));
	float * d_var_97_6;
	cudaMalloc((void **)&d_var_97_6, sizeof(float *));
	
	float * h_var_97_7 = (float *)malloc(sizeof(float *));
	float * d_var_97_7;
	cudaMalloc((void **)&d_var_97_7, sizeof(float *));
	
	float * h_var_97_8 = (float *)malloc(sizeof(float *));
	float * d_var_97_8;
	cudaMalloc((void **)&d_var_97_8, sizeof(float *));
	
	float * h_var_97_9 = (float *)malloc(sizeof(float *));
	float * d_var_97_9;
	cudaMalloc((void **)&d_var_97_9, sizeof(float *));
	
	float * h_var_97_10 = (float *)malloc(sizeof(float *));
	float * d_var_97_10;
	cudaMalloc((void **)&d_var_97_10, sizeof(float *));
	
	float * h_var_97_11 = (float *)malloc(sizeof(float *));
	float * d_var_97_11;
	cudaMalloc((void **)&d_var_97_11, sizeof(float *));
	
	float * h_var_97_12 = (float *)malloc(sizeof(float *));
	float * d_var_97_12;
	cudaMalloc((void **)&d_var_97_12, sizeof(float *));
	
	float * h_var_97_13 = (float *)malloc(sizeof(float *));
	float * d_var_97_13;
	cudaMalloc((void **)&d_var_97_13, sizeof(float *));
	
	float * h_var_97_14 = (float *)malloc(sizeof(float *));
	float * d_var_97_14;
	cudaMalloc((void **)&d_var_97_14, sizeof(float *));
	
	float * h_var_97_15 = (float *)malloc(sizeof(float *));
	float * d_var_97_15;
	cudaMalloc((void **)&d_var_97_15, sizeof(float *));
	
	float * h_var_97_16 = (float *)malloc(sizeof(float *));
	float * d_var_97_16;
	cudaMalloc((void **)&d_var_97_16, sizeof(float *));
	
	float * h_var_97_17 = (float *)malloc(sizeof(float *));
	float * d_var_97_17;
	cudaMalloc((void **)&d_var_97_17, sizeof(float *));
	
	float * h_var_97_18 = (float *)malloc(sizeof(float *));
	float * d_var_97_18;
	cudaMalloc((void **)&d_var_97_18, sizeof(float *));
	
	float * h_var_97_19 = (float *)malloc(sizeof(float *));
	float * d_var_97_19;
	cudaMalloc((void **)&d_var_97_19, sizeof(float *));
	
	float * h_var_98_0 = (float *)malloc(sizeof(float *));
	float * d_var_98_0;
	cudaMalloc((void **)&d_var_98_0, sizeof(float *));
	
	float * h_var_98_1 = (float *)malloc(sizeof(float *));
	float * d_var_98_1;
	cudaMalloc((void **)&d_var_98_1, sizeof(float *));
	
	float * h_var_98_2 = (float *)malloc(sizeof(float *));
	float * d_var_98_2;
	cudaMalloc((void **)&d_var_98_2, sizeof(float *));
	
	float * h_var_98_3 = (float *)malloc(sizeof(float *));
	float * d_var_98_3;
	cudaMalloc((void **)&d_var_98_3, sizeof(float *));
	
	float * h_var_98_4 = (float *)malloc(sizeof(float *));
	float * d_var_98_4;
	cudaMalloc((void **)&d_var_98_4, sizeof(float *));
	
	float * h_var_98_5 = (float *)malloc(sizeof(float *));
	float * d_var_98_5;
	cudaMalloc((void **)&d_var_98_5, sizeof(float *));
	
	float * h_var_98_6 = (float *)malloc(sizeof(float *));
	float * d_var_98_6;
	cudaMalloc((void **)&d_var_98_6, sizeof(float *));
	
	float * h_var_98_7 = (float *)malloc(sizeof(float *));
	float * d_var_98_7;
	cudaMalloc((void **)&d_var_98_7, sizeof(float *));
	
	float * h_var_98_8 = (float *)malloc(sizeof(float *));
	float * d_var_98_8;
	cudaMalloc((void **)&d_var_98_8, sizeof(float *));
	
	float * h_var_98_9 = (float *)malloc(sizeof(float *));
	float * d_var_98_9;
	cudaMalloc((void **)&d_var_98_9, sizeof(float *));
	
	float * h_var_98_10 = (float *)malloc(sizeof(float *));
	float * d_var_98_10;
	cudaMalloc((void **)&d_var_98_10, sizeof(float *));
	
	float * h_var_98_11 = (float *)malloc(sizeof(float *));
	float * d_var_98_11;
	cudaMalloc((void **)&d_var_98_11, sizeof(float *));
	
	float * h_var_98_12 = (float *)malloc(sizeof(float *));
	float * d_var_98_12;
	cudaMalloc((void **)&d_var_98_12, sizeof(float *));
	
	float * h_var_98_13 = (float *)malloc(sizeof(float *));
	float * d_var_98_13;
	cudaMalloc((void **)&d_var_98_13, sizeof(float *));
	
	float * h_var_98_14 = (float *)malloc(sizeof(float *));
	float * d_var_98_14;
	cudaMalloc((void **)&d_var_98_14, sizeof(float *));
	
	float * h_var_98_15 = (float *)malloc(sizeof(float *));
	float * d_var_98_15;
	cudaMalloc((void **)&d_var_98_15, sizeof(float *));
	
	float * h_var_98_16 = (float *)malloc(sizeof(float *));
	float * d_var_98_16;
	cudaMalloc((void **)&d_var_98_16, sizeof(float *));
	
	float * h_var_98_17 = (float *)malloc(sizeof(float *));
	float * d_var_98_17;
	cudaMalloc((void **)&d_var_98_17, sizeof(float *));
	
	float * h_var_98_18 = (float *)malloc(sizeof(float *));
	float * d_var_98_18;
	cudaMalloc((void **)&d_var_98_18, sizeof(float *));
	
	float * h_var_98_19 = (float *)malloc(sizeof(float *));
	float * d_var_98_19;
	cudaMalloc((void **)&d_var_98_19, sizeof(float *));
	
	float * h_var_99_0 = (float *)malloc(sizeof(float *));
	float * d_var_99_0;
	cudaMalloc((void **)&d_var_99_0, sizeof(float *));
	
	float * h_var_99_1 = (float *)malloc(sizeof(float *));
	float * d_var_99_1;
	cudaMalloc((void **)&d_var_99_1, sizeof(float *));
	
	float * h_var_99_2 = (float *)malloc(sizeof(float *));
	float * d_var_99_2;
	cudaMalloc((void **)&d_var_99_2, sizeof(float *));
	
	float * h_var_99_3 = (float *)malloc(sizeof(float *));
	float * d_var_99_3;
	cudaMalloc((void **)&d_var_99_3, sizeof(float *));
	
	float * h_var_99_4 = (float *)malloc(sizeof(float *));
	float * d_var_99_4;
	cudaMalloc((void **)&d_var_99_4, sizeof(float *));
	
	float * h_var_99_5 = (float *)malloc(sizeof(float *));
	float * d_var_99_5;
	cudaMalloc((void **)&d_var_99_5, sizeof(float *));
	
	float * h_var_99_6 = (float *)malloc(sizeof(float *));
	float * d_var_99_6;
	cudaMalloc((void **)&d_var_99_6, sizeof(float *));
	
	float * h_var_99_7 = (float *)malloc(sizeof(float *));
	float * d_var_99_7;
	cudaMalloc((void **)&d_var_99_7, sizeof(float *));
	
	float * h_var_99_8 = (float *)malloc(sizeof(float *));
	float * d_var_99_8;
	cudaMalloc((void **)&d_var_99_8, sizeof(float *));
	
	float * h_var_99_9 = (float *)malloc(sizeof(float *));
	float * d_var_99_9;
	cudaMalloc((void **)&d_var_99_9, sizeof(float *));
	
	float * h_var_99_10 = (float *)malloc(sizeof(float *));
	float * d_var_99_10;
	cudaMalloc((void **)&d_var_99_10, sizeof(float *));
	
	float * h_var_99_11 = (float *)malloc(sizeof(float *));
	float * d_var_99_11;
	cudaMalloc((void **)&d_var_99_11, sizeof(float *));
	
	float * h_var_99_12 = (float *)malloc(sizeof(float *));
	float * d_var_99_12;
	cudaMalloc((void **)&d_var_99_12, sizeof(float *));
	
	float * h_var_99_13 = (float *)malloc(sizeof(float *));
	float * d_var_99_13;
	cudaMalloc((void **)&d_var_99_13, sizeof(float *));
	
	float * h_var_99_14 = (float *)malloc(sizeof(float *));
	float * d_var_99_14;
	cudaMalloc((void **)&d_var_99_14, sizeof(float *));
	
	float * h_var_99_15 = (float *)malloc(sizeof(float *));
	float * d_var_99_15;
	cudaMalloc((void **)&d_var_99_15, sizeof(float *));
	
	float * h_var_99_16 = (float *)malloc(sizeof(float *));
	float * d_var_99_16;
	cudaMalloc((void **)&d_var_99_16, sizeof(float *));
	
	float * h_var_99_17 = (float *)malloc(sizeof(float *));
	float * d_var_99_17;
	cudaMalloc((void **)&d_var_99_17, sizeof(float *));
	
	float * h_var_99_18 = (float *)malloc(sizeof(float *));
	float * d_var_99_18;
	cudaMalloc((void **)&d_var_99_18, sizeof(float *));
	
	float * h_var_99_19 = (float *)malloc(sizeof(float *));
	float * d_var_99_19;
	cudaMalloc((void **)&d_var_99_19, sizeof(float *));
	

    // clang-format off
	
	kernel_0<<<10, 10>>>(d_var_0_0, d_var_0_1, d_var_0_2, d_var_0_3, d_var_0_4, d_var_0_5, d_var_0_6, d_var_0_7, d_var_0_8, d_var_0_9, d_var_0_10, d_var_0_11, d_var_0_12, d_var_0_13, d_var_0_14, d_var_0_15, d_var_0_16, d_var_0_17, d_var_0_18, d_var_0_19);
	
	kernel_1<<<10, 10>>>(d_var_1_0, d_var_1_1, d_var_1_2, d_var_1_3, d_var_1_4, d_var_1_5, d_var_1_6, d_var_1_7, d_var_1_8, d_var_1_9, d_var_1_10, d_var_1_11, d_var_1_12, d_var_1_13, d_var_1_14, d_var_1_15, d_var_1_16, d_var_1_17, d_var_1_18, d_var_1_19);
	
	kernel_2<<<10, 10>>>(d_var_2_0, d_var_2_1, d_var_2_2, d_var_2_3, d_var_2_4, d_var_2_5, d_var_2_6, d_var_2_7, d_var_2_8, d_var_2_9, d_var_2_10, d_var_2_11, d_var_2_12, d_var_2_13, d_var_2_14, d_var_2_15, d_var_2_16, d_var_2_17, d_var_2_18, d_var_2_19);
	
	kernel_3<<<10, 10>>>(d_var_3_0, d_var_3_1, d_var_3_2, d_var_3_3, d_var_3_4, d_var_3_5, d_var_3_6, d_var_3_7, d_var_3_8, d_var_3_9, d_var_3_10, d_var_3_11, d_var_3_12, d_var_3_13, d_var_3_14, d_var_3_15, d_var_3_16, d_var_3_17, d_var_3_18, d_var_3_19);
	
	kernel_4<<<10, 10>>>(d_var_4_0, d_var_4_1, d_var_4_2, d_var_4_3, d_var_4_4, d_var_4_5, d_var_4_6, d_var_4_7, d_var_4_8, d_var_4_9, d_var_4_10, d_var_4_11, d_var_4_12, d_var_4_13, d_var_4_14, d_var_4_15, d_var_4_16, d_var_4_17, d_var_4_18, d_var_4_19);
	
	kernel_5<<<10, 10>>>(d_var_5_0, d_var_5_1, d_var_5_2, d_var_5_3, d_var_5_4, d_var_5_5, d_var_5_6, d_var_5_7, d_var_5_8, d_var_5_9, d_var_5_10, d_var_5_11, d_var_5_12, d_var_5_13, d_var_5_14, d_var_5_15, d_var_5_16, d_var_5_17, d_var_5_18, d_var_5_19);
	
	kernel_6<<<10, 10>>>(d_var_6_0, d_var_6_1, d_var_6_2, d_var_6_3, d_var_6_4, d_var_6_5, d_var_6_6, d_var_6_7, d_var_6_8, d_var_6_9, d_var_6_10, d_var_6_11, d_var_6_12, d_var_6_13, d_var_6_14, d_var_6_15, d_var_6_16, d_var_6_17, d_var_6_18, d_var_6_19);
	
	kernel_7<<<10, 10>>>(d_var_7_0, d_var_7_1, d_var_7_2, d_var_7_3, d_var_7_4, d_var_7_5, d_var_7_6, d_var_7_7, d_var_7_8, d_var_7_9, d_var_7_10, d_var_7_11, d_var_7_12, d_var_7_13, d_var_7_14, d_var_7_15, d_var_7_16, d_var_7_17, d_var_7_18, d_var_7_19);
	
	kernel_8<<<10, 10>>>(d_var_8_0, d_var_8_1, d_var_8_2, d_var_8_3, d_var_8_4, d_var_8_5, d_var_8_6, d_var_8_7, d_var_8_8, d_var_8_9, d_var_8_10, d_var_8_11, d_var_8_12, d_var_8_13, d_var_8_14, d_var_8_15, d_var_8_16, d_var_8_17, d_var_8_18, d_var_8_19);
	
	kernel_9<<<10, 10>>>(d_var_9_0, d_var_9_1, d_var_9_2, d_var_9_3, d_var_9_4, d_var_9_5, d_var_9_6, d_var_9_7, d_var_9_8, d_var_9_9, d_var_9_10, d_var_9_11, d_var_9_12, d_var_9_13, d_var_9_14, d_var_9_15, d_var_9_16, d_var_9_17, d_var_9_18, d_var_9_19);
	
	kernel_10<<<10, 10>>>(d_var_10_0, d_var_10_1, d_var_10_2, d_var_10_3, d_var_10_4, d_var_10_5, d_var_10_6, d_var_10_7, d_var_10_8, d_var_10_9, d_var_10_10, d_var_10_11, d_var_10_12, d_var_10_13, d_var_10_14, d_var_10_15, d_var_10_16, d_var_10_17, d_var_10_18, d_var_10_19);
	
	kernel_11<<<10, 10>>>(d_var_11_0, d_var_11_1, d_var_11_2, d_var_11_3, d_var_11_4, d_var_11_5, d_var_11_6, d_var_11_7, d_var_11_8, d_var_11_9, d_var_11_10, d_var_11_11, d_var_11_12, d_var_11_13, d_var_11_14, d_var_11_15, d_var_11_16, d_var_11_17, d_var_11_18, d_var_11_19);
	
	kernel_12<<<10, 10>>>(d_var_12_0, d_var_12_1, d_var_12_2, d_var_12_3, d_var_12_4, d_var_12_5, d_var_12_6, d_var_12_7, d_var_12_8, d_var_12_9, d_var_12_10, d_var_12_11, d_var_12_12, d_var_12_13, d_var_12_14, d_var_12_15, d_var_12_16, d_var_12_17, d_var_12_18, d_var_12_19);
	
	kernel_13<<<10, 10>>>(d_var_13_0, d_var_13_1, d_var_13_2, d_var_13_3, d_var_13_4, d_var_13_5, d_var_13_6, d_var_13_7, d_var_13_8, d_var_13_9, d_var_13_10, d_var_13_11, d_var_13_12, d_var_13_13, d_var_13_14, d_var_13_15, d_var_13_16, d_var_13_17, d_var_13_18, d_var_13_19);
	
	kernel_14<<<10, 10>>>(d_var_14_0, d_var_14_1, d_var_14_2, d_var_14_3, d_var_14_4, d_var_14_5, d_var_14_6, d_var_14_7, d_var_14_8, d_var_14_9, d_var_14_10, d_var_14_11, d_var_14_12, d_var_14_13, d_var_14_14, d_var_14_15, d_var_14_16, d_var_14_17, d_var_14_18, d_var_14_19);
	
	kernel_15<<<10, 10>>>(d_var_15_0, d_var_15_1, d_var_15_2, d_var_15_3, d_var_15_4, d_var_15_5, d_var_15_6, d_var_15_7, d_var_15_8, d_var_15_9, d_var_15_10, d_var_15_11, d_var_15_12, d_var_15_13, d_var_15_14, d_var_15_15, d_var_15_16, d_var_15_17, d_var_15_18, d_var_15_19);
	
	kernel_16<<<10, 10>>>(d_var_16_0, d_var_16_1, d_var_16_2, d_var_16_3, d_var_16_4, d_var_16_5, d_var_16_6, d_var_16_7, d_var_16_8, d_var_16_9, d_var_16_10, d_var_16_11, d_var_16_12, d_var_16_13, d_var_16_14, d_var_16_15, d_var_16_16, d_var_16_17, d_var_16_18, d_var_16_19);
	
	kernel_17<<<10, 10>>>(d_var_17_0, d_var_17_1, d_var_17_2, d_var_17_3, d_var_17_4, d_var_17_5, d_var_17_6, d_var_17_7, d_var_17_8, d_var_17_9, d_var_17_10, d_var_17_11, d_var_17_12, d_var_17_13, d_var_17_14, d_var_17_15, d_var_17_16, d_var_17_17, d_var_17_18, d_var_17_19);
	
	kernel_18<<<10, 10>>>(d_var_18_0, d_var_18_1, d_var_18_2, d_var_18_3, d_var_18_4, d_var_18_5, d_var_18_6, d_var_18_7, d_var_18_8, d_var_18_9, d_var_18_10, d_var_18_11, d_var_18_12, d_var_18_13, d_var_18_14, d_var_18_15, d_var_18_16, d_var_18_17, d_var_18_18, d_var_18_19);
	
	kernel_19<<<10, 10>>>(d_var_19_0, d_var_19_1, d_var_19_2, d_var_19_3, d_var_19_4, d_var_19_5, d_var_19_6, d_var_19_7, d_var_19_8, d_var_19_9, d_var_19_10, d_var_19_11, d_var_19_12, d_var_19_13, d_var_19_14, d_var_19_15, d_var_19_16, d_var_19_17, d_var_19_18, d_var_19_19);
	
	kernel_20<<<10, 10>>>(d_var_20_0, d_var_20_1, d_var_20_2, d_var_20_3, d_var_20_4, d_var_20_5, d_var_20_6, d_var_20_7, d_var_20_8, d_var_20_9, d_var_20_10, d_var_20_11, d_var_20_12, d_var_20_13, d_var_20_14, d_var_20_15, d_var_20_16, d_var_20_17, d_var_20_18, d_var_20_19);
	
	kernel_21<<<10, 10>>>(d_var_21_0, d_var_21_1, d_var_21_2, d_var_21_3, d_var_21_4, d_var_21_5, d_var_21_6, d_var_21_7, d_var_21_8, d_var_21_9, d_var_21_10, d_var_21_11, d_var_21_12, d_var_21_13, d_var_21_14, d_var_21_15, d_var_21_16, d_var_21_17, d_var_21_18, d_var_21_19);
	
	kernel_22<<<10, 10>>>(d_var_22_0, d_var_22_1, d_var_22_2, d_var_22_3, d_var_22_4, d_var_22_5, d_var_22_6, d_var_22_7, d_var_22_8, d_var_22_9, d_var_22_10, d_var_22_11, d_var_22_12, d_var_22_13, d_var_22_14, d_var_22_15, d_var_22_16, d_var_22_17, d_var_22_18, d_var_22_19);
	
	kernel_23<<<10, 10>>>(d_var_23_0, d_var_23_1, d_var_23_2, d_var_23_3, d_var_23_4, d_var_23_5, d_var_23_6, d_var_23_7, d_var_23_8, d_var_23_9, d_var_23_10, d_var_23_11, d_var_23_12, d_var_23_13, d_var_23_14, d_var_23_15, d_var_23_16, d_var_23_17, d_var_23_18, d_var_23_19);
	
	kernel_24<<<10, 10>>>(d_var_24_0, d_var_24_1, d_var_24_2, d_var_24_3, d_var_24_4, d_var_24_5, d_var_24_6, d_var_24_7, d_var_24_8, d_var_24_9, d_var_24_10, d_var_24_11, d_var_24_12, d_var_24_13, d_var_24_14, d_var_24_15, d_var_24_16, d_var_24_17, d_var_24_18, d_var_24_19);
	
	kernel_25<<<10, 10>>>(d_var_25_0, d_var_25_1, d_var_25_2, d_var_25_3, d_var_25_4, d_var_25_5, d_var_25_6, d_var_25_7, d_var_25_8, d_var_25_9, d_var_25_10, d_var_25_11, d_var_25_12, d_var_25_13, d_var_25_14, d_var_25_15, d_var_25_16, d_var_25_17, d_var_25_18, d_var_25_19);
	
	kernel_26<<<10, 10>>>(d_var_26_0, d_var_26_1, d_var_26_2, d_var_26_3, d_var_26_4, d_var_26_5, d_var_26_6, d_var_26_7, d_var_26_8, d_var_26_9, d_var_26_10, d_var_26_11, d_var_26_12, d_var_26_13, d_var_26_14, d_var_26_15, d_var_26_16, d_var_26_17, d_var_26_18, d_var_26_19);
	
	kernel_27<<<10, 10>>>(d_var_27_0, d_var_27_1, d_var_27_2, d_var_27_3, d_var_27_4, d_var_27_5, d_var_27_6, d_var_27_7, d_var_27_8, d_var_27_9, d_var_27_10, d_var_27_11, d_var_27_12, d_var_27_13, d_var_27_14, d_var_27_15, d_var_27_16, d_var_27_17, d_var_27_18, d_var_27_19);
	
	kernel_28<<<10, 10>>>(d_var_28_0, d_var_28_1, d_var_28_2, d_var_28_3, d_var_28_4, d_var_28_5, d_var_28_6, d_var_28_7, d_var_28_8, d_var_28_9, d_var_28_10, d_var_28_11, d_var_28_12, d_var_28_13, d_var_28_14, d_var_28_15, d_var_28_16, d_var_28_17, d_var_28_18, d_var_28_19);
	
	kernel_29<<<10, 10>>>(d_var_29_0, d_var_29_1, d_var_29_2, d_var_29_3, d_var_29_4, d_var_29_5, d_var_29_6, d_var_29_7, d_var_29_8, d_var_29_9, d_var_29_10, d_var_29_11, d_var_29_12, d_var_29_13, d_var_29_14, d_var_29_15, d_var_29_16, d_var_29_17, d_var_29_18, d_var_29_19);
	
	kernel_30<<<10, 10>>>(d_var_30_0, d_var_30_1, d_var_30_2, d_var_30_3, d_var_30_4, d_var_30_5, d_var_30_6, d_var_30_7, d_var_30_8, d_var_30_9, d_var_30_10, d_var_30_11, d_var_30_12, d_var_30_13, d_var_30_14, d_var_30_15, d_var_30_16, d_var_30_17, d_var_30_18, d_var_30_19);
	
	kernel_31<<<10, 10>>>(d_var_31_0, d_var_31_1, d_var_31_2, d_var_31_3, d_var_31_4, d_var_31_5, d_var_31_6, d_var_31_7, d_var_31_8, d_var_31_9, d_var_31_10, d_var_31_11, d_var_31_12, d_var_31_13, d_var_31_14, d_var_31_15, d_var_31_16, d_var_31_17, d_var_31_18, d_var_31_19);
	
	kernel_32<<<10, 10>>>(d_var_32_0, d_var_32_1, d_var_32_2, d_var_32_3, d_var_32_4, d_var_32_5, d_var_32_6, d_var_32_7, d_var_32_8, d_var_32_9, d_var_32_10, d_var_32_11, d_var_32_12, d_var_32_13, d_var_32_14, d_var_32_15, d_var_32_16, d_var_32_17, d_var_32_18, d_var_32_19);
	
	kernel_33<<<10, 10>>>(d_var_33_0, d_var_33_1, d_var_33_2, d_var_33_3, d_var_33_4, d_var_33_5, d_var_33_6, d_var_33_7, d_var_33_8, d_var_33_9, d_var_33_10, d_var_33_11, d_var_33_12, d_var_33_13, d_var_33_14, d_var_33_15, d_var_33_16, d_var_33_17, d_var_33_18, d_var_33_19);
	
	kernel_34<<<10, 10>>>(d_var_34_0, d_var_34_1, d_var_34_2, d_var_34_3, d_var_34_4, d_var_34_5, d_var_34_6, d_var_34_7, d_var_34_8, d_var_34_9, d_var_34_10, d_var_34_11, d_var_34_12, d_var_34_13, d_var_34_14, d_var_34_15, d_var_34_16, d_var_34_17, d_var_34_18, d_var_34_19);
	
	kernel_35<<<10, 10>>>(d_var_35_0, d_var_35_1, d_var_35_2, d_var_35_3, d_var_35_4, d_var_35_5, d_var_35_6, d_var_35_7, d_var_35_8, d_var_35_9, d_var_35_10, d_var_35_11, d_var_35_12, d_var_35_13, d_var_35_14, d_var_35_15, d_var_35_16, d_var_35_17, d_var_35_18, d_var_35_19);
	
	kernel_36<<<10, 10>>>(d_var_36_0, d_var_36_1, d_var_36_2, d_var_36_3, d_var_36_4, d_var_36_5, d_var_36_6, d_var_36_7, d_var_36_8, d_var_36_9, d_var_36_10, d_var_36_11, d_var_36_12, d_var_36_13, d_var_36_14, d_var_36_15, d_var_36_16, d_var_36_17, d_var_36_18, d_var_36_19);
	
	kernel_37<<<10, 10>>>(d_var_37_0, d_var_37_1, d_var_37_2, d_var_37_3, d_var_37_4, d_var_37_5, d_var_37_6, d_var_37_7, d_var_37_8, d_var_37_9, d_var_37_10, d_var_37_11, d_var_37_12, d_var_37_13, d_var_37_14, d_var_37_15, d_var_37_16, d_var_37_17, d_var_37_18, d_var_37_19);
	
	kernel_38<<<10, 10>>>(d_var_38_0, d_var_38_1, d_var_38_2, d_var_38_3, d_var_38_4, d_var_38_5, d_var_38_6, d_var_38_7, d_var_38_8, d_var_38_9, d_var_38_10, d_var_38_11, d_var_38_12, d_var_38_13, d_var_38_14, d_var_38_15, d_var_38_16, d_var_38_17, d_var_38_18, d_var_38_19);
	
	kernel_39<<<10, 10>>>(d_var_39_0, d_var_39_1, d_var_39_2, d_var_39_3, d_var_39_4, d_var_39_5, d_var_39_6, d_var_39_7, d_var_39_8, d_var_39_9, d_var_39_10, d_var_39_11, d_var_39_12, d_var_39_13, d_var_39_14, d_var_39_15, d_var_39_16, d_var_39_17, d_var_39_18, d_var_39_19);
	
	kernel_40<<<10, 10>>>(d_var_40_0, d_var_40_1, d_var_40_2, d_var_40_3, d_var_40_4, d_var_40_5, d_var_40_6, d_var_40_7, d_var_40_8, d_var_40_9, d_var_40_10, d_var_40_11, d_var_40_12, d_var_40_13, d_var_40_14, d_var_40_15, d_var_40_16, d_var_40_17, d_var_40_18, d_var_40_19);
	
	kernel_41<<<10, 10>>>(d_var_41_0, d_var_41_1, d_var_41_2, d_var_41_3, d_var_41_4, d_var_41_5, d_var_41_6, d_var_41_7, d_var_41_8, d_var_41_9, d_var_41_10, d_var_41_11, d_var_41_12, d_var_41_13, d_var_41_14, d_var_41_15, d_var_41_16, d_var_41_17, d_var_41_18, d_var_41_19);
	
	kernel_42<<<10, 10>>>(d_var_42_0, d_var_42_1, d_var_42_2, d_var_42_3, d_var_42_4, d_var_42_5, d_var_42_6, d_var_42_7, d_var_42_8, d_var_42_9, d_var_42_10, d_var_42_11, d_var_42_12, d_var_42_13, d_var_42_14, d_var_42_15, d_var_42_16, d_var_42_17, d_var_42_18, d_var_42_19);
	
	kernel_43<<<10, 10>>>(d_var_43_0, d_var_43_1, d_var_43_2, d_var_43_3, d_var_43_4, d_var_43_5, d_var_43_6, d_var_43_7, d_var_43_8, d_var_43_9, d_var_43_10, d_var_43_11, d_var_43_12, d_var_43_13, d_var_43_14, d_var_43_15, d_var_43_16, d_var_43_17, d_var_43_18, d_var_43_19);
	
	kernel_44<<<10, 10>>>(d_var_44_0, d_var_44_1, d_var_44_2, d_var_44_3, d_var_44_4, d_var_44_5, d_var_44_6, d_var_44_7, d_var_44_8, d_var_44_9, d_var_44_10, d_var_44_11, d_var_44_12, d_var_44_13, d_var_44_14, d_var_44_15, d_var_44_16, d_var_44_17, d_var_44_18, d_var_44_19);
	
	kernel_45<<<10, 10>>>(d_var_45_0, d_var_45_1, d_var_45_2, d_var_45_3, d_var_45_4, d_var_45_5, d_var_45_6, d_var_45_7, d_var_45_8, d_var_45_9, d_var_45_10, d_var_45_11, d_var_45_12, d_var_45_13, d_var_45_14, d_var_45_15, d_var_45_16, d_var_45_17, d_var_45_18, d_var_45_19);
	
	kernel_46<<<10, 10>>>(d_var_46_0, d_var_46_1, d_var_46_2, d_var_46_3, d_var_46_4, d_var_46_5, d_var_46_6, d_var_46_7, d_var_46_8, d_var_46_9, d_var_46_10, d_var_46_11, d_var_46_12, d_var_46_13, d_var_46_14, d_var_46_15, d_var_46_16, d_var_46_17, d_var_46_18, d_var_46_19);
	
	kernel_47<<<10, 10>>>(d_var_47_0, d_var_47_1, d_var_47_2, d_var_47_3, d_var_47_4, d_var_47_5, d_var_47_6, d_var_47_7, d_var_47_8, d_var_47_9, d_var_47_10, d_var_47_11, d_var_47_12, d_var_47_13, d_var_47_14, d_var_47_15, d_var_47_16, d_var_47_17, d_var_47_18, d_var_47_19);
	
	kernel_48<<<10, 10>>>(d_var_48_0, d_var_48_1, d_var_48_2, d_var_48_3, d_var_48_4, d_var_48_5, d_var_48_6, d_var_48_7, d_var_48_8, d_var_48_9, d_var_48_10, d_var_48_11, d_var_48_12, d_var_48_13, d_var_48_14, d_var_48_15, d_var_48_16, d_var_48_17, d_var_48_18, d_var_48_19);
	
	kernel_49<<<10, 10>>>(d_var_49_0, d_var_49_1, d_var_49_2, d_var_49_3, d_var_49_4, d_var_49_5, d_var_49_6, d_var_49_7, d_var_49_8, d_var_49_9, d_var_49_10, d_var_49_11, d_var_49_12, d_var_49_13, d_var_49_14, d_var_49_15, d_var_49_16, d_var_49_17, d_var_49_18, d_var_49_19);
	
	kernel_50<<<10, 10>>>(d_var_50_0, d_var_50_1, d_var_50_2, d_var_50_3, d_var_50_4, d_var_50_5, d_var_50_6, d_var_50_7, d_var_50_8, d_var_50_9, d_var_50_10, d_var_50_11, d_var_50_12, d_var_50_13, d_var_50_14, d_var_50_15, d_var_50_16, d_var_50_17, d_var_50_18, d_var_50_19);
	
	kernel_51<<<10, 10>>>(d_var_51_0, d_var_51_1, d_var_51_2, d_var_51_3, d_var_51_4, d_var_51_5, d_var_51_6, d_var_51_7, d_var_51_8, d_var_51_9, d_var_51_10, d_var_51_11, d_var_51_12, d_var_51_13, d_var_51_14, d_var_51_15, d_var_51_16, d_var_51_17, d_var_51_18, d_var_51_19);
	
	kernel_52<<<10, 10>>>(d_var_52_0, d_var_52_1, d_var_52_2, d_var_52_3, d_var_52_4, d_var_52_5, d_var_52_6, d_var_52_7, d_var_52_8, d_var_52_9, d_var_52_10, d_var_52_11, d_var_52_12, d_var_52_13, d_var_52_14, d_var_52_15, d_var_52_16, d_var_52_17, d_var_52_18, d_var_52_19);
	
	kernel_53<<<10, 10>>>(d_var_53_0, d_var_53_1, d_var_53_2, d_var_53_3, d_var_53_4, d_var_53_5, d_var_53_6, d_var_53_7, d_var_53_8, d_var_53_9, d_var_53_10, d_var_53_11, d_var_53_12, d_var_53_13, d_var_53_14, d_var_53_15, d_var_53_16, d_var_53_17, d_var_53_18, d_var_53_19);
	
	kernel_54<<<10, 10>>>(d_var_54_0, d_var_54_1, d_var_54_2, d_var_54_3, d_var_54_4, d_var_54_5, d_var_54_6, d_var_54_7, d_var_54_8, d_var_54_9, d_var_54_10, d_var_54_11, d_var_54_12, d_var_54_13, d_var_54_14, d_var_54_15, d_var_54_16, d_var_54_17, d_var_54_18, d_var_54_19);
	
	kernel_55<<<10, 10>>>(d_var_55_0, d_var_55_1, d_var_55_2, d_var_55_3, d_var_55_4, d_var_55_5, d_var_55_6, d_var_55_7, d_var_55_8, d_var_55_9, d_var_55_10, d_var_55_11, d_var_55_12, d_var_55_13, d_var_55_14, d_var_55_15, d_var_55_16, d_var_55_17, d_var_55_18, d_var_55_19);
	
	kernel_56<<<10, 10>>>(d_var_56_0, d_var_56_1, d_var_56_2, d_var_56_3, d_var_56_4, d_var_56_5, d_var_56_6, d_var_56_7, d_var_56_8, d_var_56_9, d_var_56_10, d_var_56_11, d_var_56_12, d_var_56_13, d_var_56_14, d_var_56_15, d_var_56_16, d_var_56_17, d_var_56_18, d_var_56_19);
	
	kernel_57<<<10, 10>>>(d_var_57_0, d_var_57_1, d_var_57_2, d_var_57_3, d_var_57_4, d_var_57_5, d_var_57_6, d_var_57_7, d_var_57_8, d_var_57_9, d_var_57_10, d_var_57_11, d_var_57_12, d_var_57_13, d_var_57_14, d_var_57_15, d_var_57_16, d_var_57_17, d_var_57_18, d_var_57_19);
	
	kernel_58<<<10, 10>>>(d_var_58_0, d_var_58_1, d_var_58_2, d_var_58_3, d_var_58_4, d_var_58_5, d_var_58_6, d_var_58_7, d_var_58_8, d_var_58_9, d_var_58_10, d_var_58_11, d_var_58_12, d_var_58_13, d_var_58_14, d_var_58_15, d_var_58_16, d_var_58_17, d_var_58_18, d_var_58_19);
	
	kernel_59<<<10, 10>>>(d_var_59_0, d_var_59_1, d_var_59_2, d_var_59_3, d_var_59_4, d_var_59_5, d_var_59_6, d_var_59_7, d_var_59_8, d_var_59_9, d_var_59_10, d_var_59_11, d_var_59_12, d_var_59_13, d_var_59_14, d_var_59_15, d_var_59_16, d_var_59_17, d_var_59_18, d_var_59_19);
	
	kernel_60<<<10, 10>>>(d_var_60_0, d_var_60_1, d_var_60_2, d_var_60_3, d_var_60_4, d_var_60_5, d_var_60_6, d_var_60_7, d_var_60_8, d_var_60_9, d_var_60_10, d_var_60_11, d_var_60_12, d_var_60_13, d_var_60_14, d_var_60_15, d_var_60_16, d_var_60_17, d_var_60_18, d_var_60_19);
	
	kernel_61<<<10, 10>>>(d_var_61_0, d_var_61_1, d_var_61_2, d_var_61_3, d_var_61_4, d_var_61_5, d_var_61_6, d_var_61_7, d_var_61_8, d_var_61_9, d_var_61_10, d_var_61_11, d_var_61_12, d_var_61_13, d_var_61_14, d_var_61_15, d_var_61_16, d_var_61_17, d_var_61_18, d_var_61_19);
	
	kernel_62<<<10, 10>>>(d_var_62_0, d_var_62_1, d_var_62_2, d_var_62_3, d_var_62_4, d_var_62_5, d_var_62_6, d_var_62_7, d_var_62_8, d_var_62_9, d_var_62_10, d_var_62_11, d_var_62_12, d_var_62_13, d_var_62_14, d_var_62_15, d_var_62_16, d_var_62_17, d_var_62_18, d_var_62_19);
	
	kernel_63<<<10, 10>>>(d_var_63_0, d_var_63_1, d_var_63_2, d_var_63_3, d_var_63_4, d_var_63_5, d_var_63_6, d_var_63_7, d_var_63_8, d_var_63_9, d_var_63_10, d_var_63_11, d_var_63_12, d_var_63_13, d_var_63_14, d_var_63_15, d_var_63_16, d_var_63_17, d_var_63_18, d_var_63_19);
	
	kernel_64<<<10, 10>>>(d_var_64_0, d_var_64_1, d_var_64_2, d_var_64_3, d_var_64_4, d_var_64_5, d_var_64_6, d_var_64_7, d_var_64_8, d_var_64_9, d_var_64_10, d_var_64_11, d_var_64_12, d_var_64_13, d_var_64_14, d_var_64_15, d_var_64_16, d_var_64_17, d_var_64_18, d_var_64_19);
	
	kernel_65<<<10, 10>>>(d_var_65_0, d_var_65_1, d_var_65_2, d_var_65_3, d_var_65_4, d_var_65_5, d_var_65_6, d_var_65_7, d_var_65_8, d_var_65_9, d_var_65_10, d_var_65_11, d_var_65_12, d_var_65_13, d_var_65_14, d_var_65_15, d_var_65_16, d_var_65_17, d_var_65_18, d_var_65_19);
	
	kernel_66<<<10, 10>>>(d_var_66_0, d_var_66_1, d_var_66_2, d_var_66_3, d_var_66_4, d_var_66_5, d_var_66_6, d_var_66_7, d_var_66_8, d_var_66_9, d_var_66_10, d_var_66_11, d_var_66_12, d_var_66_13, d_var_66_14, d_var_66_15, d_var_66_16, d_var_66_17, d_var_66_18, d_var_66_19);
	
	kernel_67<<<10, 10>>>(d_var_67_0, d_var_67_1, d_var_67_2, d_var_67_3, d_var_67_4, d_var_67_5, d_var_67_6, d_var_67_7, d_var_67_8, d_var_67_9, d_var_67_10, d_var_67_11, d_var_67_12, d_var_67_13, d_var_67_14, d_var_67_15, d_var_67_16, d_var_67_17, d_var_67_18, d_var_67_19);
	
	kernel_68<<<10, 10>>>(d_var_68_0, d_var_68_1, d_var_68_2, d_var_68_3, d_var_68_4, d_var_68_5, d_var_68_6, d_var_68_7, d_var_68_8, d_var_68_9, d_var_68_10, d_var_68_11, d_var_68_12, d_var_68_13, d_var_68_14, d_var_68_15, d_var_68_16, d_var_68_17, d_var_68_18, d_var_68_19);
	
	kernel_69<<<10, 10>>>(d_var_69_0, d_var_69_1, d_var_69_2, d_var_69_3, d_var_69_4, d_var_69_5, d_var_69_6, d_var_69_7, d_var_69_8, d_var_69_9, d_var_69_10, d_var_69_11, d_var_69_12, d_var_69_13, d_var_69_14, d_var_69_15, d_var_69_16, d_var_69_17, d_var_69_18, d_var_69_19);
	
	kernel_70<<<10, 10>>>(d_var_70_0, d_var_70_1, d_var_70_2, d_var_70_3, d_var_70_4, d_var_70_5, d_var_70_6, d_var_70_7, d_var_70_8, d_var_70_9, d_var_70_10, d_var_70_11, d_var_70_12, d_var_70_13, d_var_70_14, d_var_70_15, d_var_70_16, d_var_70_17, d_var_70_18, d_var_70_19);
	
	kernel_71<<<10, 10>>>(d_var_71_0, d_var_71_1, d_var_71_2, d_var_71_3, d_var_71_4, d_var_71_5, d_var_71_6, d_var_71_7, d_var_71_8, d_var_71_9, d_var_71_10, d_var_71_11, d_var_71_12, d_var_71_13, d_var_71_14, d_var_71_15, d_var_71_16, d_var_71_17, d_var_71_18, d_var_71_19);
	
	kernel_72<<<10, 10>>>(d_var_72_0, d_var_72_1, d_var_72_2, d_var_72_3, d_var_72_4, d_var_72_5, d_var_72_6, d_var_72_7, d_var_72_8, d_var_72_9, d_var_72_10, d_var_72_11, d_var_72_12, d_var_72_13, d_var_72_14, d_var_72_15, d_var_72_16, d_var_72_17, d_var_72_18, d_var_72_19);
	
	kernel_73<<<10, 10>>>(d_var_73_0, d_var_73_1, d_var_73_2, d_var_73_3, d_var_73_4, d_var_73_5, d_var_73_6, d_var_73_7, d_var_73_8, d_var_73_9, d_var_73_10, d_var_73_11, d_var_73_12, d_var_73_13, d_var_73_14, d_var_73_15, d_var_73_16, d_var_73_17, d_var_73_18, d_var_73_19);
	
	kernel_74<<<10, 10>>>(d_var_74_0, d_var_74_1, d_var_74_2, d_var_74_3, d_var_74_4, d_var_74_5, d_var_74_6, d_var_74_7, d_var_74_8, d_var_74_9, d_var_74_10, d_var_74_11, d_var_74_12, d_var_74_13, d_var_74_14, d_var_74_15, d_var_74_16, d_var_74_17, d_var_74_18, d_var_74_19);
	
	kernel_75<<<10, 10>>>(d_var_75_0, d_var_75_1, d_var_75_2, d_var_75_3, d_var_75_4, d_var_75_5, d_var_75_6, d_var_75_7, d_var_75_8, d_var_75_9, d_var_75_10, d_var_75_11, d_var_75_12, d_var_75_13, d_var_75_14, d_var_75_15, d_var_75_16, d_var_75_17, d_var_75_18, d_var_75_19);
	
	kernel_76<<<10, 10>>>(d_var_76_0, d_var_76_1, d_var_76_2, d_var_76_3, d_var_76_4, d_var_76_5, d_var_76_6, d_var_76_7, d_var_76_8, d_var_76_9, d_var_76_10, d_var_76_11, d_var_76_12, d_var_76_13, d_var_76_14, d_var_76_15, d_var_76_16, d_var_76_17, d_var_76_18, d_var_76_19);
	
	kernel_77<<<10, 10>>>(d_var_77_0, d_var_77_1, d_var_77_2, d_var_77_3, d_var_77_4, d_var_77_5, d_var_77_6, d_var_77_7, d_var_77_8, d_var_77_9, d_var_77_10, d_var_77_11, d_var_77_12, d_var_77_13, d_var_77_14, d_var_77_15, d_var_77_16, d_var_77_17, d_var_77_18, d_var_77_19);
	
	kernel_78<<<10, 10>>>(d_var_78_0, d_var_78_1, d_var_78_2, d_var_78_3, d_var_78_4, d_var_78_5, d_var_78_6, d_var_78_7, d_var_78_8, d_var_78_9, d_var_78_10, d_var_78_11, d_var_78_12, d_var_78_13, d_var_78_14, d_var_78_15, d_var_78_16, d_var_78_17, d_var_78_18, d_var_78_19);
	
	kernel_79<<<10, 10>>>(d_var_79_0, d_var_79_1, d_var_79_2, d_var_79_3, d_var_79_4, d_var_79_5, d_var_79_6, d_var_79_7, d_var_79_8, d_var_79_9, d_var_79_10, d_var_79_11, d_var_79_12, d_var_79_13, d_var_79_14, d_var_79_15, d_var_79_16, d_var_79_17, d_var_79_18, d_var_79_19);
	
	kernel_80<<<10, 10>>>(d_var_80_0, d_var_80_1, d_var_80_2, d_var_80_3, d_var_80_4, d_var_80_5, d_var_80_6, d_var_80_7, d_var_80_8, d_var_80_9, d_var_80_10, d_var_80_11, d_var_80_12, d_var_80_13, d_var_80_14, d_var_80_15, d_var_80_16, d_var_80_17, d_var_80_18, d_var_80_19);
	
	kernel_81<<<10, 10>>>(d_var_81_0, d_var_81_1, d_var_81_2, d_var_81_3, d_var_81_4, d_var_81_5, d_var_81_6, d_var_81_7, d_var_81_8, d_var_81_9, d_var_81_10, d_var_81_11, d_var_81_12, d_var_81_13, d_var_81_14, d_var_81_15, d_var_81_16, d_var_81_17, d_var_81_18, d_var_81_19);
	
	kernel_82<<<10, 10>>>(d_var_82_0, d_var_82_1, d_var_82_2, d_var_82_3, d_var_82_4, d_var_82_5, d_var_82_6, d_var_82_7, d_var_82_8, d_var_82_9, d_var_82_10, d_var_82_11, d_var_82_12, d_var_82_13, d_var_82_14, d_var_82_15, d_var_82_16, d_var_82_17, d_var_82_18, d_var_82_19);
	
	kernel_83<<<10, 10>>>(d_var_83_0, d_var_83_1, d_var_83_2, d_var_83_3, d_var_83_4, d_var_83_5, d_var_83_6, d_var_83_7, d_var_83_8, d_var_83_9, d_var_83_10, d_var_83_11, d_var_83_12, d_var_83_13, d_var_83_14, d_var_83_15, d_var_83_16, d_var_83_17, d_var_83_18, d_var_83_19);
	
	kernel_84<<<10, 10>>>(d_var_84_0, d_var_84_1, d_var_84_2, d_var_84_3, d_var_84_4, d_var_84_5, d_var_84_6, d_var_84_7, d_var_84_8, d_var_84_9, d_var_84_10, d_var_84_11, d_var_84_12, d_var_84_13, d_var_84_14, d_var_84_15, d_var_84_16, d_var_84_17, d_var_84_18, d_var_84_19);
	
	kernel_85<<<10, 10>>>(d_var_85_0, d_var_85_1, d_var_85_2, d_var_85_3, d_var_85_4, d_var_85_5, d_var_85_6, d_var_85_7, d_var_85_8, d_var_85_9, d_var_85_10, d_var_85_11, d_var_85_12, d_var_85_13, d_var_85_14, d_var_85_15, d_var_85_16, d_var_85_17, d_var_85_18, d_var_85_19);
	
	kernel_86<<<10, 10>>>(d_var_86_0, d_var_86_1, d_var_86_2, d_var_86_3, d_var_86_4, d_var_86_5, d_var_86_6, d_var_86_7, d_var_86_8, d_var_86_9, d_var_86_10, d_var_86_11, d_var_86_12, d_var_86_13, d_var_86_14, d_var_86_15, d_var_86_16, d_var_86_17, d_var_86_18, d_var_86_19);
	
	kernel_87<<<10, 10>>>(d_var_87_0, d_var_87_1, d_var_87_2, d_var_87_3, d_var_87_4, d_var_87_5, d_var_87_6, d_var_87_7, d_var_87_8, d_var_87_9, d_var_87_10, d_var_87_11, d_var_87_12, d_var_87_13, d_var_87_14, d_var_87_15, d_var_87_16, d_var_87_17, d_var_87_18, d_var_87_19);
	
	kernel_88<<<10, 10>>>(d_var_88_0, d_var_88_1, d_var_88_2, d_var_88_3, d_var_88_4, d_var_88_5, d_var_88_6, d_var_88_7, d_var_88_8, d_var_88_9, d_var_88_10, d_var_88_11, d_var_88_12, d_var_88_13, d_var_88_14, d_var_88_15, d_var_88_16, d_var_88_17, d_var_88_18, d_var_88_19);
	
	kernel_89<<<10, 10>>>(d_var_89_0, d_var_89_1, d_var_89_2, d_var_89_3, d_var_89_4, d_var_89_5, d_var_89_6, d_var_89_7, d_var_89_8, d_var_89_9, d_var_89_10, d_var_89_11, d_var_89_12, d_var_89_13, d_var_89_14, d_var_89_15, d_var_89_16, d_var_89_17, d_var_89_18, d_var_89_19);
	
	kernel_90<<<10, 10>>>(d_var_90_0, d_var_90_1, d_var_90_2, d_var_90_3, d_var_90_4, d_var_90_5, d_var_90_6, d_var_90_7, d_var_90_8, d_var_90_9, d_var_90_10, d_var_90_11, d_var_90_12, d_var_90_13, d_var_90_14, d_var_90_15, d_var_90_16, d_var_90_17, d_var_90_18, d_var_90_19);
	
	kernel_91<<<10, 10>>>(d_var_91_0, d_var_91_1, d_var_91_2, d_var_91_3, d_var_91_4, d_var_91_5, d_var_91_6, d_var_91_7, d_var_91_8, d_var_91_9, d_var_91_10, d_var_91_11, d_var_91_12, d_var_91_13, d_var_91_14, d_var_91_15, d_var_91_16, d_var_91_17, d_var_91_18, d_var_91_19);
	
	kernel_92<<<10, 10>>>(d_var_92_0, d_var_92_1, d_var_92_2, d_var_92_3, d_var_92_4, d_var_92_5, d_var_92_6, d_var_92_7, d_var_92_8, d_var_92_9, d_var_92_10, d_var_92_11, d_var_92_12, d_var_92_13, d_var_92_14, d_var_92_15, d_var_92_16, d_var_92_17, d_var_92_18, d_var_92_19);
	
	kernel_93<<<10, 10>>>(d_var_93_0, d_var_93_1, d_var_93_2, d_var_93_3, d_var_93_4, d_var_93_5, d_var_93_6, d_var_93_7, d_var_93_8, d_var_93_9, d_var_93_10, d_var_93_11, d_var_93_12, d_var_93_13, d_var_93_14, d_var_93_15, d_var_93_16, d_var_93_17, d_var_93_18, d_var_93_19);
	
	kernel_94<<<10, 10>>>(d_var_94_0, d_var_94_1, d_var_94_2, d_var_94_3, d_var_94_4, d_var_94_5, d_var_94_6, d_var_94_7, d_var_94_8, d_var_94_9, d_var_94_10, d_var_94_11, d_var_94_12, d_var_94_13, d_var_94_14, d_var_94_15, d_var_94_16, d_var_94_17, d_var_94_18, d_var_94_19);
	
	kernel_95<<<10, 10>>>(d_var_95_0, d_var_95_1, d_var_95_2, d_var_95_3, d_var_95_4, d_var_95_5, d_var_95_6, d_var_95_7, d_var_95_8, d_var_95_9, d_var_95_10, d_var_95_11, d_var_95_12, d_var_95_13, d_var_95_14, d_var_95_15, d_var_95_16, d_var_95_17, d_var_95_18, d_var_95_19);
	
	kernel_96<<<10, 10>>>(d_var_96_0, d_var_96_1, d_var_96_2, d_var_96_3, d_var_96_4, d_var_96_5, d_var_96_6, d_var_96_7, d_var_96_8, d_var_96_9, d_var_96_10, d_var_96_11, d_var_96_12, d_var_96_13, d_var_96_14, d_var_96_15, d_var_96_16, d_var_96_17, d_var_96_18, d_var_96_19);
	
	kernel_97<<<10, 10>>>(d_var_97_0, d_var_97_1, d_var_97_2, d_var_97_3, d_var_97_4, d_var_97_5, d_var_97_6, d_var_97_7, d_var_97_8, d_var_97_9, d_var_97_10, d_var_97_11, d_var_97_12, d_var_97_13, d_var_97_14, d_var_97_15, d_var_97_16, d_var_97_17, d_var_97_18, d_var_97_19);
	
	kernel_98<<<10, 10>>>(d_var_98_0, d_var_98_1, d_var_98_2, d_var_98_3, d_var_98_4, d_var_98_5, d_var_98_6, d_var_98_7, d_var_98_8, d_var_98_9, d_var_98_10, d_var_98_11, d_var_98_12, d_var_98_13, d_var_98_14, d_var_98_15, d_var_98_16, d_var_98_17, d_var_98_18, d_var_98_19);
	
	kernel_99<<<10, 10>>>(d_var_99_0, d_var_99_1, d_var_99_2, d_var_99_3, d_var_99_4, d_var_99_5, d_var_99_6, d_var_99_7, d_var_99_8, d_var_99_9, d_var_99_10, d_var_99_11, d_var_99_12, d_var_99_13, d_var_99_14, d_var_99_15, d_var_99_16, d_var_99_17, d_var_99_18, d_var_99_19);
	
    // clang-format on

    printf("Done\n");
    return 0;
}
