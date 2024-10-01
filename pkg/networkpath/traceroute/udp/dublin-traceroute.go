/* SPDX-License-Identifier: BSD-2-Clause */

// Package dublintraceroute provides the common interface for Dublin Traceroute
package dublintraceroute

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/udp/results"
)

// default values and constants
const (
	DefaultReadTimeout = time.Millisecond * 3000
)

// DublinTraceroute is the common interface that every Dublin Traceroute
// probe type has to implement
type DublinTraceroute interface {
	Validate() error
	Traceroute() (*results.Results, error)
}
