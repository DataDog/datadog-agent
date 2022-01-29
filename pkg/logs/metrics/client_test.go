package metrics

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func Test_findAddr(t *testing.T) {
	tests := []struct {
		name          string
		binddHost     string
		dogstatsdPort int
	}{
		{
			name:          "both set",
			binddHost:     "somehost",
			dogstatsdPort: 1234,
		},
		{
			name:      "bind_host only",
			binddHost: "somehost",
		},
		{
			name:          "dogstatsd_port only",
			dogstatsdPort: 1234,
		},
		{
			name: "both empty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.Datadog = config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
			if tt.binddHost == "" {
				tt.binddHost = "localhost"
			} else {
				config.Datadog.Set("bind_host", tt.binddHost)
			}
			if tt.dogstatsdPort == 0 {
				tt.dogstatsdPort = 8125
			} else {
				config.Datadog.Set("dogstatsd_port", tt.dogstatsdPort)
			}
			assert.Equal(t, findAddr(config.Datadog), fmt.Sprintf("%s:%d", tt.binddHost, tt.dogstatsdPort))
		})
	}
}
