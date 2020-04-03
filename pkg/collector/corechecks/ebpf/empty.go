// +build !linux

package ebpf

// Avoid the following error on Windows:
// cmd\agent\app\run.go:49:2: build constraints exclude all Go files in C:\omnibus-ruby\src\datadog-puppy\src\github.com\DataDog\datadog-agent\pkg\collector\corechecks\ebpf
func init() {
}
