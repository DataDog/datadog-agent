// +build linux_bpf

package ebpf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLinuxKernelVersionCode(t *testing.T) {
	// Some sanity checks
	assert.Equal(t, linuxKernelVersionCode(2, 6, 9), uint32(132617))
	assert.Equal(t, linuxKernelVersionCode(3, 2, 12), uint32(197132))
	assert.Equal(t, linuxKernelVersionCode(4, 4, 0), uint32(263168))

	assert.Equal(t, stringToKernelCode("2.6.9"), uint32(132617))
	assert.Equal(t, stringToKernelCode("3.2.12"), uint32(197132))
	assert.Equal(t, stringToKernelCode("4.4.0"), uint32(263168))

	assert.Equal(t, kernelCodeToString(uint32(132617)), "2.6.9")
	assert.Equal(t, kernelCodeToString(uint32(197132)), "3.2.12")
	assert.Equal(t, kernelCodeToString(uint32(263168)), "4.4.0")
}

func TestHasPre410Kernel(t *testing.T) {
	oldKernels := []string{"3.10.0", "2.5.0", "4.0.10", "4.0"}
	for _, kernel := range oldKernels {
		assert.True(t, isPre410Kernel(stringToKernelCode(kernel)))
	}
	newKernels := []string{"4.1.0", "4.10.2", "4.1", "5.1"}
	for _, kernel := range newKernels {
		assert.False(t, isPre410Kernel(stringToKernelCode(kernel)))
	}
}
