// +build !linux !cgo

package linuxaudit

// Avoid the following error on non-supported platforms:
// "build constraints exclude all Go files in github.com\DataDog\datadog-agent\pkg\collector\corechecks\linuxaudit"
func init() {
}
