// +build freebsd netbsd openbsd solaris dragonfly

package v5

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	CommonPayload
	HostPayload
	ResourcesPayload
	// TODO: host-tags
	// TODO: external_host_tags
	// TODO: gohai alternative (or fix gohai)
	// TODO: agent_checks
}
