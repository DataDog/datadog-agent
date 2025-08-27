package servicemonitor_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/servicemonitor"
)

func TestStore(t *testing.T) {
	tests := []struct {
		name             string
		initialMonitors  []servicemonitor.DatadogServiceMonitor
		expectedMonitors []servicemonitor.DatadogServiceMonitor
	}{
		{
			name:             "no monitors",
			initialMonitors:  []servicemonitor.DatadogServiceMonitor{},
			expectedMonitors: []servicemonitor.DatadogServiceMonitor{},
		},
		{
			name: "one monitor",
			initialMonitors: []servicemonitor.DatadogServiceMonitor{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "test"},
					Spec: servicemonitor.DatadogServiceMonitorSpec{
						Name:     "test",
						Priority: 1,
					},
				},
			},
			expectedMonitors: []servicemonitor.DatadogServiceMonitor{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "test"},
					Spec: servicemonitor.DatadogServiceMonitorSpec{
						Name:     "test",
						Priority: 1,
					},
				},
			},
		},
		{
			name: "two monitors with different priorities",
			initialMonitors: []servicemonitor.DatadogServiceMonitor{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "test"},
					Spec: servicemonitor.DatadogServiceMonitorSpec{
						Name:     "test",
						Priority: 10,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "test2"},
					Spec: servicemonitor.DatadogServiceMonitorSpec{
						Name:     "test2",
						Priority: 2,
					},
				},
			},
			expectedMonitors: []servicemonitor.DatadogServiceMonitor{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "test2"},
					Spec: servicemonitor.DatadogServiceMonitorSpec{
						Name:     "test2",
						Priority: 2,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "test"},
					Spec: servicemonitor.DatadogServiceMonitorSpec{
						Name:     "test",
						Priority: 10,
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := servicemonitor.NewStore()

			listener := make(chan []servicemonitor.DatadogServiceMonitor, 10)
			store.AddListener(listener)

			for i, monitor := range test.initialMonitors {
				store.SetDatadogServiceMonitor(monitor)

				select {
				case monitors := <-listener:
					assert.Equal(t, i+1, len(monitors))
				case <-time.After(1 * time.Second):
					t.Fatalf("timeout waiting for monitors")
				}
			}

			assert.Equal(t, test.expectedMonitors, store.GetDatadogServiceMonitors())
		})
	}
}
