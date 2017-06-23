// +build freebsd netbsd openbsd solaris dragonfly

package resources

// GetPayload currently just a stub.
func GetPayload(hostname string) *Payload {

	//unimplemented for misc platforms.
	return &Payload{}
}
