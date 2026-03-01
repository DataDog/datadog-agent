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

package otel

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewayUsage_EnvVar(t *testing.T) {
	gEnv := NewGatewayUsage(false)
	var gaude, enable = gEnv.Gauge()
	require.EqualValuesf(t, 0.0, gaude, "Exected 0 value")
	require.True(t, enable)

	gEnv = NewGatewayUsage(true)
	gaude, enable = gEnv.Gauge()
	require.EqualValuesf(t, 1.0, gaude, "Exected 1 value")
	require.True(t, enable)
}

func TestDisabledGatewayUsage(t *testing.T) {
	g := NewDisabledGatewayUsage()
	gauge, enabled := g.Gauge()
	require.EqualValues(t, 0, gauge)
	require.False(t, enabled)

	require.Nil(t, g.GetHostFromAttributesHandler())
	require.EqualValues(t, 0, g.EnvVarValue())
}

func TestGetHostFromAttributesHandler(t *testing.T) {
	g := NewGatewayUsage(false)
	handler := g.GetHostFromAttributesHandler()
	require.NotNil(t, handler)
}

func TestEnvVarValue(t *testing.T) {
	g := NewGatewayUsage(false)
	require.EqualValues(t, 0, g.EnvVarValue())

	g = NewGatewayUsage(true)
	require.EqualValues(t, 1, g.EnvVarValue())
}
