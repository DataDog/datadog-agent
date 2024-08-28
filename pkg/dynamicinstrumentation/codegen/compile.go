//go:build linux_bpf

package codegen

//go:generate $GOPATH/bin/include_headers pkg/dynamicinstrumentation/codegen/c/dynamicinstrumentation.c pkg/ebpf/bytecode/build/runtime/dynamicinstrumentation_template.c pkg/ebpf/c
//go:generate $GOPATH/bin/integrity pkg/dynamicinstrumentation/codegen/c/dynamicinstrumentation.c pkg/ebpf/bytecode/runtime/dynamicinstrumentation.go runtime
