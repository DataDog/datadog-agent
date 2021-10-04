package config

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/stretchr/testify/assert"
)

func TestTopMetaCopy(t *testing.T) {
	assert := assert.New(t)
	old := &pbgo.TopMeta{
		Version: 3,
		Raw:     []byte("bananas"),
	}
	new := topMetaCopy(old)
	assert.Equal(old, new)
	old.Raw = []byte("2")
	assert.NotEqual(old, new)
}

func TestGetUpload(t *testing.T) {
	assert := assert.New(t)

	roots := []*pbgo.TopMeta{
		{
			Version: 0,
			Raw:     []byte("root0"),
		},
		{
			Version: 1,
			Raw:     []byte("root1"),
		},
		{
			Version: 2,
			Raw:     []byte("root2"),
		},
		{
			Version: 3,
			Raw:     []byte("root3"),
		},
		{
			Version: 4,
			Raw:     []byte("root4"),
		},
	}

	targets1 := &pbgo.TopMeta{
		Version: 29191,
		Raw:     []byte("targets1"),
	}
	targets2 := &pbgo.TopMeta{
		Version: 39191,
		Raw:     []byte("targets2"),
	}
	targets3 := &pbgo.TopMeta{
		Version: 588191,
		Raw:     []byte("targets3"),
	}

	targetFiles := []*pbgo.File{
		{
			Path: "a/b/c",
			Raw:  []byte("aaah"),
		},
		{
			Path: "b/b/b",
			Raw:  []byte("bb"),
		},
	}

	var testSteps = []struct {
		name         string
		remoteUpdate *pbgo.ConfigResponse
		req          *pbgo.GetConfigsRequest
		expectedRes  *pbgo.ConfigResponse
		expectedErr  error
	}{
		{
			name: "not allowed product",
			req: &pbgo.GetConfigsRequest{
				Product: pbgo.Product_APM_SAMPLING,
			},
			expectedErr: errors.New("not allowed"),
		},
		{
			name: "no stored config",
			req: &pbgo.GetConfigsRequest{
				Product: pbgo.Product_LIVE_DEBUGGING,
			},
		},
		{
			name: "remote update 2 root",
			remoteUpdate: &pbgo.ConfigResponse{
				ConfigDelegatedTargetVersion: 5,
				DirectoryRoots:               roots[1:3],
				DirectoryTargets:             targets1,
				TargetFiles:                  targetFiles,
			},
			req: &pbgo.GetConfigsRequest{
				Product: pbgo.Product_LIVE_DEBUGGING,
			},
			expectedRes: &pbgo.ConfigResponse{
				ConfigDelegatedTargetVersion: 5,
				DirectoryRoots:               roots[1:3],
				DirectoryTargets:             targets1,
				TargetFiles:                  targetFiles,
			},
		},
		{
			name: "remote update disabled all target files",
			remoteUpdate: &pbgo.ConfigResponse{
				ConfigDelegatedTargetVersion: 7,
				DirectoryTargets:             targets2,
			},
			req: &pbgo.GetConfigsRequest{
				Product: pbgo.Product_LIVE_DEBUGGING,
			},
			expectedRes: &pbgo.ConfigResponse{
				ConfigDelegatedTargetVersion: 7,
				DirectoryRoots:               roots[1:3],
				DirectoryTargets:             targets2,
			},
		},
		{
			name: "client root up to date",
			req: &pbgo.GetConfigsRequest{
				Product:                    pbgo.Product_LIVE_DEBUGGING,
				CurrentDirectorRootVersion: 2,
			},
			expectedRes: &pbgo.ConfigResponse{
				ConfigDelegatedTargetVersion: 7,
				DirectoryTargets:             targets2,
			},
		},
		{
			name: "client up to date",
			req: &pbgo.GetConfigsRequest{
				CurrentDirectorRootVersion:  2,
				CurrentConfigProductVersion: 7,
				Product:                     pbgo.Product_LIVE_DEBUGGING,
			},
		},
		{
			name: "new roots",
			remoteUpdate: &pbgo.ConfigResponse{
				ConfigDelegatedTargetVersion: 25,
				DirectoryRoots:               roots[3:5],
				DirectoryTargets:             targets3,
				TargetFiles:                  targetFiles,
			},
			req: &pbgo.GetConfigsRequest{
				CurrentDirectorRootVersion:  1,
				CurrentConfigProductVersion: 7,
				Product:                     pbgo.Product_LIVE_DEBUGGING,
			},
			expectedRes: &pbgo.ConfigResponse{
				ConfigDelegatedTargetVersion: 25,
				DirectoryRoots:               roots[2:5],
				DirectoryTargets:             targets3,
				TargetFiles:                  targetFiles,
			},
		},
	}
	s := NewSubscriber()
	for _, step := range testSteps {
		t.Log(step.name)
		if step.remoteUpdate != nil {
			assert.Nil(s.loadNewConfig(step.remoteUpdate))
		}
		if step.req != nil {
			res, err := s.Get(step.req)
			assert.Equal(step.expectedErr, err)
			assert.Equal(step.expectedRes, res)
		}
	}
}
