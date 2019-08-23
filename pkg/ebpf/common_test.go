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
}

func TestUbuntu44119NotSupported(t *testing.T) {
	for i := uint32(119); i < 127; i++ {
		ok, msg := verifyOSVersion(linuxKernelVersionCode(4, 4, i), "linux-4.4-with-ubuntu", nil)
		assert.False(t, ok)
		assert.NotEmpty(t, msg)
	}
}

func TestLinuxAWSPreceding441060NotSupported(t *testing.T) {
	for i := uint32(120); i < 128; i++ {
		ok, msg := verifyOSVersion(linuxKernelVersionCode(4, 4, i), "Linux-4.4.0-1060-aws-x86_64-with-Ubuntu-16.04-xenial", nil)
		assert.False(t, ok)
		assert.NotEmpty(t, msg)
	}
}

func TestExcludedKernelVersion(t *testing.T) {
	exclusionList := []string{"5.5.1", "6.3.2"}
	ok, msg := verifyOSVersion(linuxKernelVersionCode(4, 4, 121), "ubuntu", exclusionList)
	assert.False(t, ok)
	assert.NotEmpty(t, msg)

	ok, msg = verifyOSVersion(linuxKernelVersionCode(5, 5, 1), "debian", exclusionList)
	assert.False(t, ok)
	assert.NotEmpty(t, msg)

	ok, msg = verifyOSVersion(linuxKernelVersionCode(6, 3, 2), "debian", exclusionList)
	assert.False(t, ok)
	assert.NotEmpty(t, msg)

	ok, msg = verifyOSVersion(linuxKernelVersionCode(6, 3, 1), "debian", exclusionList)
	assert.True(t, ok)
	assert.Empty(t, msg)

	ok, msg = verifyOSVersion(linuxKernelVersionCode(5, 5, 2), "debian", exclusionList)
	assert.True(t, ok)
	assert.Empty(t, msg)

	ok, msg = verifyOSVersion(linuxKernelVersionCode(3, 10, 0), "Linux-3.10.0-957.5.1.el7.x86_64-x86_64-with-centos-7.6.1810-Core", exclusionList)
	assert.True(t, ok)
	assert.Empty(t, msg)
}

func TestVerifyKernelFuncs(t *testing.T) {
	missing, err := verifyKernelFuncs("./testdata/kallsyms.supported")
	assert.Empty(t, missing)
	assert.Empty(t, err)

	missing, err = verifyKernelFuncs("./testdata/kallsyms.unsupported")
	assert.NotEmpty(t, missing)
	assert.Empty(t, err)

	missing, err = verifyKernelFuncs("./testdata/kallsyms.empty")
	assert.NotEmpty(t, missing)
	assert.Empty(t, err)

	_, err = verifyKernelFuncs("./testdata/kallsyms.d_o_n_o_t_e_x_i_s_t")
	assert.NotEmpty(t, err)
}
