package ebpf

import (
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
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
	testBPFDir, err := ioutil.TempDir("", "test-bpfdir")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testBPFDir)

	assetSource := `#include <linux/bpf.h>
#include <linux/tcp.h>
#include <linux/oom.h>

#include "test-header.h"
`

	assetHeader := `#ifndef TEST_H
#define TEST_H

#include <linux/types.h>

#ifndef SOME_CONSTANT
#define SOME_CONSTANT 10
#endif

struct test_struct {
  __u32 id;
};

#endif /* defined(TEST_H) */
`

	if err := ioutil.WriteFile(path.Join(testBPFDir, "test-asset.c"), []byte(assetSource), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ioutil.WriteFile(path.Join(testBPFDir, "test-header.h"), []byte(assetHeader), 0644); err != nil {
		t.Fatal(err)
	}

	bytecode.DefaultBPFDir = testBPFDir
	source, err := processHeaders("test-asset.c")

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
