// +build docker

package docker

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/docker/docker/api/types/container"
)

func TestParseContainerUTSMode(t *testing.T) {
	tests := []struct {
		name       string
		hostConfig *container.HostConfig
		want       containers.UTSMode
		wantErr    bool
	}{
		{
			name: "default",
			hostConfig: &container.HostConfig{
				UTSMode: "",
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "host",
			hostConfig: &container.HostConfig{
				UTSMode: "host",
			},
			want:    "host",
			wantErr: false,
		},
		{
			name: "attached to container",
			hostConfig: &container.HostConfig{
				UTSMode: "container:0a8f83f35f7d0161f29b819d9b533b57acade8d99609bba63664dd3326e4d301",
			},
			want:    "container:0a8f83f35f7d0161f29b819d9b533b57acade8d99609bba63664dd3326e4d301",
			wantErr: false,
		},
		{
			name: "unknown",
			hostConfig: &container.HostConfig{
				UTSMode: "Unexected unknown mode",
			},
			want:    "unknown",
			wantErr: true,
		},
		{
			name:       "nil hostConfig",
			hostConfig: nil,
			want:       "unknown",
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseContainerUTSMode(tt.hostConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseContainerUTSMode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseContainerUTSMode() = %v, want %v", got, tt.want)
			}
		})
	}
}
