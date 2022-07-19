package common

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestIncompatibleListenerWarning(t *testing.T) {
	cases := []struct {
		listeners        []config.Listeners
		expectedWarnings int
	}{
		{
			listeners: []config.Listeners{
				{Name: "kubelet"},
				{Name: "foo"},
				{Name: "bar"},
			},
			expectedWarnings: 0,
		},
		{
			listeners: []config.Listeners{
				{Name: "kubelet"},
				{Name: "container"},
				{Name: "bar"},
			},
			expectedWarnings: 2,
		},
	}

	for _, tc := range cases {
		if warnings := warnOnIncompatibleListeners(tc.listeners); warnings != tc.expectedWarnings {
			t.Errorf("Expected %d incompatible warnings, but got %d warnings for input %v\n", tc.expectedWarnings, warnings, tc.listeners)
		}
	}
}

func TestIncompatibleProvidersWarning(t *testing.T) {
	cases := []struct {
		providers        map[string]config.ConfigurationProviders
		expectedWarnings int
	}{
		{
			providers: map[string]config.ConfigurationProviders{
				"kubelet": {Name: "kubelet"},
				"foo":     {Name: "foo"},
			},
			expectedWarnings: 0,
		},
		{
			providers: map[string]config.ConfigurationProviders{
				"kubelet":   {Name: "kubelet"},
				"container": {Name: "container"},
				"foo":       {Name: "foo"},
			},
			expectedWarnings: 2,
		},
	}

	for _, tc := range cases {
		if warnings := warnOnIncompatibleProviders(tc.providers); warnings != tc.expectedWarnings {
			t.Errorf("Expected %d incompatible warnings, but got %d warnings for input %v\n", tc.expectedWarnings, warnings, tc.providers)
		}
	}
}
