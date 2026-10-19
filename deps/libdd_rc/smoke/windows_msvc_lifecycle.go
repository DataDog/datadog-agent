package main

import (
	"fmt"

	"github.com/DataDog/libdd-rc/ffi-hosts/go/rcx509"
)

func main() {
	client, err := rcx509.NewClient("ws://127.0.0.1:1", "datadog-agent", "bazel-windows-msvc-smoke")
	if err != nil {
		panic(fmt.Sprintf("initialize public rcx509 client: %v", err))
	}
	if err := client.Close(); err != nil {
		panic(fmt.Sprintf("close public rcx509 client: %v", err))
	}
	fmt.Println("MinGW host completed the public rcx509 lifecycle through the MSVC FFI")
}
