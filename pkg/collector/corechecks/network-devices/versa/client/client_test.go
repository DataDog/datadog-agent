package client

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/versa/client/fixtures"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TODO: add more tests
func TestGetSLAMetrics(t *testing.T) {
	mux, handler := setupCommonServerMuxWithFixture(SLA_METRICS_URL, fixtures.GetSLAMetrics)

	// TODO: move this to always include to help debugging
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("Unexpected request to: %s", r.URL.Path)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	slaMetrics, err := client.GetSLAMetrics()

	// TODO: after actual parsing logic is better, check the contents more thoroughly
	require.Equal(t, len(slaMetrics), 1)
	require.Equal(t, slaMetrics[0].DrillKey, "test-branch-2B,Controller-2,INET-1,INET-1,fc_nc")
	require.Equal(t, 1, handler.numberOfCalls())
}
