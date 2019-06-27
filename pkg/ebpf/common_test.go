package ebpf

import (
	"io/ioutil"
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
		ok, err := verifyOSVersion(linuxKernelVersionCode(4, 4, i), "linux-4.4-with-ubuntu", nil)
		assert.False(t, ok)
		assert.Error(t, err)
	}
}

func TestExcludedKernelVersion(t *testing.T) {
	exclusionList := []string{"5.5.1", "6.3.2"}
	ok, err := verifyOSVersion(linuxKernelVersionCode(4, 4, 121), "ubuntu", exclusionList)
	assert.False(t, ok)
	assert.Error(t, err)

	ok, err = verifyOSVersion(linuxKernelVersionCode(5, 5, 1), "debian", exclusionList)
	assert.False(t, ok)
	assert.Error(t, err)

	ok, err = verifyOSVersion(linuxKernelVersionCode(6, 3, 2), "debian", exclusionList)
	assert.False(t, ok)
	assert.Error(t, err)

	ok, err = verifyOSVersion(linuxKernelVersionCode(6, 3, 1), "debian", exclusionList)
	assert.True(t, ok)
	assert.Nil(t, err)

	ok, err = verifyOSVersion(linuxKernelVersionCode(5, 5, 2), "debian", exclusionList)
	assert.True(t, ok)
	assert.Nil(t, err)

	ok, err = verifyOSVersion(linuxKernelVersionCode(3, 10, 0), "Linux-3.10.0-957.5.1.el7.x86_64-x86_64-with-centos-7.6.1810-Core", exclusionList)
	assert.True(t, ok)
	assert.Nil(t, err)
}

func TestVerifyKernelFuncs(t *testing.T) {
	kallsyms, err := ioutil.ReadFile("./testdata/kallsyms.supported")
	assert.NoError(t, err)

	assert.True(t, verifyKernelFuncs(string(kallsyms)))

	kallsymsUnsupported, err := ioutil.ReadFile("./testdata/kallsyms.unsupported")
	assert.NoError(t, err)

	assert.False(t, verifyKernelFuncs(string(kallsymsUnsupported)))
}
