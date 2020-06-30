// +build !linux !cgo

package ebpf

// Avoid the following error on non-supported platforms:
// "build constraints exclude all Go files in github.com\DataDog\datadog-agent\pkg\collector\corechecks\ebpf"
func init() {
}
