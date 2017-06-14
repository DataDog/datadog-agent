package embed

import (
	"bytes"
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/providers"
	"github.com/stretchr/testify/assert"
)

func getFile() (string, error) {
	_, fileName, _, ok := runtime.Caller(1)
	if !ok {
		return "", errors.New("could not get current (caller) file")
	}
	return fileName, nil
}

func TestLoadCheckConfig(t *testing.T) {

	jl := NewJMXCheckLoader()
	assert.NotNil(t, jl)

	f, err := getFile()
	if err != nil {
		t.FailNow()
	}
	d := filepath.Dir(f)

	paths := []string{filepath.Join(d, "fixtures/")}
	fp := providers.NewFileConfigProvider(paths)
	assert.NotNil(t, fp)

	cfgs, err := fp.Collect()
	assert.Nil(t, err)

	// should be two valid instances
	assert.Len(t, cfgs, 2)
	for _, cfg := range cfgs {
		_, err := jl.Load(cfg)
		assert.Nil(t, err)

		pBuf := bytes.NewBuffer(make([]byte, 0, bytes.MinRead))
		_, err = pBuf.ReadFrom(jl.ipc)

		assert.Contains(t, string(pBuf.Bytes()), autoDiscoveryToken)
		assert.Contains(t, string(pBuf.Bytes()), cfg.Name)
	}

}
