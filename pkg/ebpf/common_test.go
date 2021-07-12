package ebpf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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

func TestProcessHeaders(t *testing.T) {
	testFile := "pkg/ebpf/testdata/test-asset.c"
	source, err := processHeaders(testFile)

	sourceString := source.String()

	// Assert err is nil
	assert.Nil(t, err, err)

	// Assert negative examples source should not contain
	assert.NotContains(t, sourceString, "linux/sched.h")
	assert.NotContains(t, sourceString, "linux/stdio.h")

	// Assert examples of what source should contain
	assert.Contains(t, sourceString, "linux/oom.h")
	assert.Contains(t, sourceString, "linux/tcp.h")
	assert.Contains(t, sourceString, "linux/bpf.h")

	// Assert test-header.h content is present
	assert.Contains(t, sourceString, "TEST_H")
	assert.Contains(t, sourceString, "linux/types.h")
	assert.Contains(t, sourceString, "SOME_CONSTANT")
	assert.Contains(t, sourceString, "test_struct")
}
