// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"
	"time"

	cjson "github.com/tent/canonical-json-go"
	"github.com/theupdateframework/go-tuf/data"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
)

func TestService(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "store")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	defer os.Remove(tmpFile.Name())

	config.DetectFeatures()

	service, err := NewService(Opts{
		DBPath:                 tmpFile.Name(),
		RemoteConfigurationKey: "test.com/1/1234",
		RefreshInterval:        time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	// override client with test client
	service.client = &testClient{}

	configChan := make(chan *pbgo.ConfigResponse)
	callback := func(config *pbgo.ConfigResponse) error {
		configChan <- config
		return nil
	}

	subscriber := NewSubscriber(pbgo.Product_APPSEC, time.Second*10, callback)
	service.RegisterSubscriber(subscriber)

	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	timeout := 5 * time.Second
	select {
	case <-configChan:
	case <-time.After(timeout):
		// TODO(lebauce): update root, use fixtures and do not skip test
		t.Skipf("did not receive any config in %s", timeout)
	}
}

type testClient struct {
}

func (c *testClient) Fetch(_ context.Context, configStates *pbgo.ClientLatestConfigsRequest) (*pbgo.LatestConfigsResponse, error) {
	return newTestConfig(1)
}

func newTestConfig(version uint64) (*pbgo.LatestConfigsResponse, error) {
	timestampRaw := []byte(`{
 "signatures": [
  {
   "keyid": "701f7e8d4451d55f834606807ee782e9f508ef6f2400bc111b0532c6817b0a0d",
   "sig": "fb13749afeecad4fcd79d18bc573cfb9fe320f5b740ad53cb61576b1fef6e0a83e18c3e1d7036b679af2651f1e0539f6c70c439c874f25ca5b6a040b806af802"
  }
 ],
 "signed": {
  "_type": "timestamp",
  "expires": "2021-07-27T12:39:09Z",
  "meta": {
   "snapshot.json": {
    "hashes": {
     "sha256": "f174a84f5cac393a90f7c97706c4b819138a15b15959663e217f2f55cf139a50",
     "sha512": "9e7d17f0ba11c1f4b1273637a427a35bbe4e805a9cf4c26a081365328d9734f11a20a66d75034f812b4f8fb21cb4a8cce9bd6d5a217fdc37d240779f16a6b2de"
    },
    "length": 431,
    "version": 1
   }
  },
  "spec_version": "1.0.0",
  "version": 1
 }
}`)

	s := &data.Signed{}
	if err := json.Unmarshal(timestampRaw, s); err != nil {
		return nil, err
	}

	var timestamp data.Timestamp
	if err := json.Unmarshal(s.Signed, &timestamp); err != nil {
		return nil, err
	}

	var err error
	timestampRaw, err = cjson.Marshal(s)
	if err != nil {
		return nil, err
	}

	snapshotRaw := []byte(`{
 "signatures": [
  {
   "keyid": "c3a7fa32c0417e270b6c1450005369c94bfad6aa761b623a8ef859df65846b71",
   "sig": "95e304aa6f2f2a97eae42ea937f8e51a20fbdedeba4e71d7b2117f22200b7f1bd908f9fb55b6fbcd2fdd002125a6f683a2e8c3248f3409ef4d18e2075cc3f902"
  }
 ],
 "signed": {
  "_type": "snapshot",
  "expires": "2021-08-02T12:39:09Z",
  "meta": {
   "targets.json": {
    "version": 1
   }
  },
  "spec_version": "1.0.0",
  "version": 1
 }
}`)

	targetsRaw := []byte(`{
 "signatures": [
  {
   "keyid": "c72f27ac9d5e5169d18f4f5ecab24bc659abb86374e8e696603c64e5ff0fdd13",
   "sig": "aaf0f994a833c7b03b3b5481c8bdc0f9d15e7227f4087084fc5e3178f48faac575acebc03a995ac69c67fbdf3564ba803f26cd9d11c56101f6ca598081efdb03"
  }
 ],
 "signed": {
  "_type": "targets",
  "delegations": {
   "keys": {},
   "roles": []
  },
  "expires": "2021-10-25T20:06:19Z",
  "spec_version": "1.0.0",
  "targets": {},
  "version": 1
 }
}`)

	return &pbgo.LatestConfigsResponse{
		DirectorMetas: &pbgo.DirectorMetas{
			Timestamp: &pbgo.TopMeta{
				Version: 1,
				Raw:     timestampRaw,
			},
			Snapshot: &pbgo.TopMeta{
				Version: 1,
				Raw:     snapshotRaw,
			},
			Targets: &pbgo.TopMeta{
				Version: 1,
				Raw:     targetsRaw,
			},
		},
		TargetFiles: []*pbgo.File{{
			Path: "APPSEC",
			Raw:  []byte("test"),
		}},
	}, nil
}
