// +build linux_bpf

package ebpf

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/stretchr/testify/assert"
)

func TestUbuntuKernelsNotSupported(t *testing.T) {
	for i := byte(114); i < byte(128); i++ {
		ok, msg := verifyOSVersion(kernel.VersionCode(4, 4, i), "ubuntu", nil)
		assert.False(t, ok)
		assert.NotEmpty(t, msg)
	}

	for i := byte(100); i < byte(114); i++ {
		ok, msg := verifyOSVersion(kernel.VersionCode(4, 4, i), "ubuntu", nil)
		assert.True(t, ok)
		assert.Empty(t, msg)
	}

	for i := byte(128); i < byte(255); i++ {
		ok, msg := verifyOSVersion(kernel.VersionCode(4, 4, i), "ubuntu", nil)
		assert.True(t, ok)
		assert.Empty(t, msg)
	}
}

func TestExcludedKernelVersion(t *testing.T) {
	exclusionList := []string{"5.5.1", "6.3.2"}
	ok, msg := verifyOSVersion(kernel.VersionCode(4, 4, 121), "ubuntu", exclusionList)
	assert.False(t, ok)
	assert.NotEmpty(t, msg)

	ok, msg = verifyOSVersion(kernel.VersionCode(5, 5, 1), "debian", exclusionList)
	assert.False(t, ok)
	assert.NotEmpty(t, msg)

	ok, msg = verifyOSVersion(kernel.VersionCode(6, 3, 2), "debian", exclusionList)
	assert.False(t, ok)
	assert.NotEmpty(t, msg)

	ok, msg = verifyOSVersion(kernel.VersionCode(6, 3, 1), "debian", exclusionList)
	assert.True(t, ok)
	assert.Empty(t, msg)

	ok, msg = verifyOSVersion(kernel.VersionCode(5, 5, 2), "debian", exclusionList)
	assert.True(t, ok)
	assert.Empty(t, msg)

	ok, msg = verifyOSVersion(kernel.VersionCode(3, 10, 0), "Linux-3.10.0-957.5.1.el7.x86_64-x86_64-with-centos-7.6.1810-Core", exclusionList)
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
