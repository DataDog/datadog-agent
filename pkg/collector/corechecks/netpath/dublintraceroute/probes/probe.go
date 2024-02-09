/* SPDX-License-Identifier: BSD-2-Clause */

package probes

// Probe represents a sent probe. Every protocol-specific probe has to implement
// this interface
type Probe interface {
	Validate() error
}

// ProbeResponse represents a response to a sent probe. Every protocol-specific
// probe response has to implement this interface
type ProbeResponse interface {
	Validate() error
	Matches(Probe) bool
}
