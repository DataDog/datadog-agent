package embed

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/providers"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

//implementation to mock actual pipe
type MemoryPipe struct {
	pipeIn  *io.PipeReader
	pipeOut *io.PipeWriter
}

func (p *MemoryPipe) Open() error {
	// just to implement interface
	return nil
}
func (p *MemoryPipe) Ready() bool {
	if p.pipeIn != nil && p.pipeOut != nil {
		return true
	}
	return false
}
func (p *MemoryPipe) Read(b []byte) (int, error) {
	return p.pipeIn.Read(b)
}
func (p *MemoryPipe) Write(b []byte) (int, error) {
	return p.pipeOut.Write(b)
}
func (p *MemoryPipe) Close() error {
	if err := p.pipeIn.Close(); err != nil {
		return err
	}
	if err := p.pipeOut.Close(); err != nil {
		return err
	}

	return nil
}

func getFile() (string, error) {
	_, fileName, _, ok := runtime.Caller(1)
	if !ok {
		return "", errors.New("could not get current (caller) file")
	}
	return fileName, nil
}

func TestLoadCheckConfig(t *testing.T) {

	tmp, err := ioutil.TempDir("", "datadog-agent")
	if err != nil {
		t.Fatalf("unable to create temporary directory: %v", err)
	}

	defer os.RemoveAll(tmp) // clean up

	config.Datadog.Set("jmx_pipe_path", tmp)

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

	// let's swap in for a memory pipe.
	pipeI, pipeO := io.Pipe()
	jl.ipc.Close()
	jl.ipc = &MemoryPipe{pipeIn: pipeI, pipeOut: pipeO}

	// should be two valid instances
	assert.Len(t, cfgs, 2)
	for _, cfg := range cfgs {
		// parallel because reader/writers block
		go func(c check.Config) {
			_, err := jl.Load(cfg)
			assert.Nil(t, err)
		}(cfg)

		pBuf := make([]byte, 65536)
		jl.ipc.Read(pBuf)

		assert.Contains(t, string(pBuf), autoDiscoveryToken)
		assert.Contains(t, string(pBuf), cfg.Name)
	}
}
