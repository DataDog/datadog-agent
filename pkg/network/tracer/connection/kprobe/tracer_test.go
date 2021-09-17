//+build linux_bpf

package kprobe

import (
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

func testConfig() *config.Config {
	cfg := config.New()
	//if os.Getenv(runtimeCompilationEnvVar) != "" {
	//	cfg.EnableRuntimeCompiler = true
	//	cfg.AllowPrecompiledFallback = false
	//}
	return cfg
}
