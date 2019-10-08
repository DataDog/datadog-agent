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

func TestUbuntuKernelsNotSupported(t *testing.T) {
	for i := uint32(114); i < 128; i++ {
		ok, msg := verifyOSVersion(linuxKernelVersionCode(4, 4, i), "linux-4.4-with-ubuntu", nil)
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

func TestIsCentOS(t *testing.T) {
	// python -m platform
	assert.True(t, isCentOS("Linux-3.10.0-957.21.3.el7.x86_64-x86_64-with-centos-7.6.1810-Core"))
	// lsb_release -a
	assert.True(t, isCentOS("Description:    CentOS Linux release 7.6.1810 (Core)"))
}

func TestIsRHEL(t *testing.T) {
	// python -m platform
	assert.True(t, isRHEL("Linux-3.10.0-957.el7.x86_64-x86_64-with-redhat-7.6-Maipo"))
	// uname -a
	assert.True(t, isRHEL("Linux rhel7.localdomain 3.10.0-957.el7.x86_64 #1 SMP Thu Oct 4 20:48:51 UTC 2018 x86_64 x86_64 x86_64 GNU/Linux"))
	// cat /etc/redhat-release
	assert.True(t, isRHEL("Red Hat Enterprise Linux Server release 7.6 (Maipo)"))
}
