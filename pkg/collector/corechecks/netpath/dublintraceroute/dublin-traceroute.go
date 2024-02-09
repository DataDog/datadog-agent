/* SPDX-License-Identifier: BSD-2-Clause */

package dublintraceroute

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/netpath/dublintraceroute/results"
	"time"
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
