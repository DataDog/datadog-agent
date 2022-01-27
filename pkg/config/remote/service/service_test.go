package service

import (
	"context"
	"encoding/base32"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	rdata "github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	"github.com/DataDog/datadog-agent/pkg/proto/msgpgo"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theupdateframework/go-tuf/data"
)

type mockAPI struct {
	mock.Mock
}

func (m *mockAPI) Fetch(ctx context.Context, request *pbgo.LatestConfigsRequest) (*pbgo.LatestConfigsResponse, error) {
	args := m.Called(ctx, request)
	return args.Get(0).(*pbgo.LatestConfigsResponse), args.Error(1)
}

type mockUptane struct {
	mock.Mock
}

func (m *mockUptane) Update(response *pbgo.LatestConfigsResponse) error {
	args := m.Called(response)
	return args.Error(0)
}

func (m *mockUptane) State() (uptane.State, error) {
	args := m.Called()
	return args.Get(0).(uptane.State), args.Error(1)
}

func (m *mockUptane) DirectorRoot(version uint64) ([]byte, error) {
	args := m.Called(version)
	return args.Get(0).([]byte), args.Error(1)
}

func (m *mockUptane) Targets() (data.TargetFiles, error) {
	args := m.Called()
	return args.Get(0).(data.TargetFiles), args.Error(1)
}

func (m *mockUptane) TargetFile(path string) ([]byte, error) {
	args := m.Called(path)
	return args.Get(0).([]byte), args.Error(1)
}

func (m *mockUptane) TargetsMeta() ([]byte, error) {
	args := m.Called()
	return args.Get(0).([]byte), args.Error(1)
}

var (
	testRCKey = msgpgo.RemoteConfigKey{
		AppKey:     "fake_key",
		OrgID:      2,
		Datacenter: "dd.com",
	}
)

func newTestService(t *testing.T, api *mockAPI, uptane *mockUptane, clock clock.Clock) *Service {
	dir, err := os.MkdirTemp("", "testdbdir")
	assert.NoError(t, err)
	config.Datadog.Set("run_path", dir)
	serializedKey, _ := testRCKey.MarshalMsg(nil)
	config.Datadog.Set("remote_configuration.key", base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(serializedKey))
	service, err := NewService()
	assert.NoError(t, err)
	service.api = api
	service.clock = clock
	service.uptane = uptane
	assert.NoError(t, err)
	return service
}

func TestService(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	lastConfigResponse := &pbgo.LatestConfigsResponse{
		TargetFiles: []*pbgo.File{{Path: "test"}},
	}
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		AgentVersion:                 version.AgentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
	}).Return(lastConfigResponse, nil)
	uptaneClient.On("State").Return(uptane.State{}, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)

	err := service.refresh()
	assert.NoError(t, err)

	api.AssertExpectations(t)
	uptaneClient.AssertExpectations(t)

	*uptaneClient = mockUptane{}
	*api = mockAPI{}

	root3 := []byte(`testroot3`)
	root4 := []byte(`testroot4`)
	targets := []byte(`testtargets`)
	client := &pbgo.Client{
		State: &pbgo.ClientState{
			RootVersion: 2,
		},
		Products: []string{
			string(rdata.ProductAPMSampling),
		},
	}
	fileAPM1 := []byte(`testapm1`)
	fileAPM2 := []byte(`testapm2`)
	uptaneClient.On("TargetsMeta").Return(targets, nil)
	uptaneClient.On("Targets").Return(data.TargetFiles{"datadog/2/APM_SAMPLING/id/1": {}, "datadog/2/TESTING1/id/1": {}, "datadog/2/APM_SAMPLING/id/2": {}, "datadog/2/APPSEC/id/1": {}}, nil)
	uptaneClient.On("State").Return(uptane.State{ConfigRootVersion: 1, ConfigSnapshotVersion: 2, DirectorRootVersion: 4, DirectorTargetsVersion: 5}, nil)
	uptaneClient.On("DirectorRoot", uint64(3)).Return(root3, nil)
	uptaneClient.On("DirectorRoot", uint64(4)).Return(root4, nil)
	uptaneClient.On("TargetFile", "datadog/2/APM_SAMPLING/id/1").Return(fileAPM1, nil)
	uptaneClient.On("TargetFile", "datadog/2/APM_SAMPLING/id/2").Return(fileAPM2, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		AgentVersion:                 version.AgentVersion,
		CurrentConfigRootVersion:     1,
		CurrentConfigSnapshotVersion: 2,
		CurrentDirectorRootVersion:   4,
		Products:                     []string{},
		NewProducts: []string{
			string(rdata.ProductAPMSampling),
		},
		ActiveClients: []*pbgo.Client{client},
	}).Return(lastConfigResponse, nil)

	configResponse, err := service.ClientGetConfigs(&pbgo.ClientGetConfigsRequest{Client: client})
	assert.NoError(t, err)
	assert.ElementsMatch(t, []*pbgo.TopMeta{{Version: 3, Raw: root3}, {Version: 4, Raw: root4}}, configResponse.Roots)
	assert.ElementsMatch(t, []*pbgo.File{{Path: "datadog/2/APM_SAMPLING/id/1", Raw: fileAPM1}, {Path: "datadog/2/APM_SAMPLING/id/2", Raw: fileAPM2}}, configResponse.TargetFiles)
	assert.Equal(t, &pbgo.TopMeta{Version: 5, Raw: targets}, configResponse.Targets)
	err = service.refresh()
	assert.NoError(t, err)

	api.AssertExpectations(t)
	uptaneClient.AssertExpectations(t)
}
