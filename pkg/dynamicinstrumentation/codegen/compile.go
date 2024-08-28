//go:build linux_bpf

package codegen

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
)

//go:generate $GOPATH/bin/include_headers pkg/dynamicinstrumentation/codegen/c/template.c pkg/ebpf/bytecode/build/runtime/dynamicinstrumentation_template.c pkg/ebpf/c

func getRuntimeCompileDI(config *config.Config) (runtime.CompiledOutput, error) {
	return nil, nil
}
