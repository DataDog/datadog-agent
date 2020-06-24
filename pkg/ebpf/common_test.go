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

func TestSnakeToCamel(t *testing.T) {
	for test, exp := range map[string]string{
		"closed_conn_dropped":      "ClosedConnDropped",
		"closed_conn_polling_lost": "ClosedConnPollingLost",
	} {
		assert.Equal(t, exp, snakeToCapInitialCamel(test))
	}
}
