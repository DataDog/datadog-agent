// +build linux_bpf

package ebpf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
