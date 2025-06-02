// Copyright  OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package attributes

import "sync"

// GatewayUsage is a HostFromAttributesHandler that detects if the setup is a gateway.
// If two attributes have different hostnames, then we consider the setup is a gateway.
type GatewayUsage struct {
	firstHostname string
	gatewayUsage  bool
	m             sync.Mutex
}

var _ HostFromAttributesHandler = (*GatewayUsage)(nil)

// NewGatewayUsage returns a new GatewayUsage.
// If two attributes have different hostnames, then we consider the setup is a gateway.
func NewGatewayUsage() *GatewayUsage {
	return &GatewayUsage{}
}

// OnHost implements HostFromAttributesHandler.
func (g *GatewayUsage) OnHost(host string) {
	g.m.Lock()
	defer g.m.Unlock()
	if g.firstHostname == "" {
		g.firstHostname = host
	} else if g.firstHostname != host {
		g.gatewayUsage = true
	}
}

// GatewayUsage returns true if the GatewayUsage was detected.
func (g *GatewayUsage) GatewayUsage() bool {
	g.m.Lock()
	defer g.m.Unlock()
	return g.gatewayUsage
}

// Gauge returns 1 if the GatewayUsage was detected, 0 otherwise.
func (g *GatewayUsage) Gauge() float64 {
	if g.GatewayUsage() {
		return 1
	}
	return 0
}
