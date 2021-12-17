package service

import (
	"encoding/base32"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
)

func TestRemoteConfigKey(t *testing.T) {
	tests := []struct {
		input  string
		err    bool
		output *pbgo.RemoteConfigKey
	}{
		{input: generateKey(t, 2, "datadoghq.com", "58d58c60b8ac337293ce2ca6b28b19eb"), output: &pbgo.RemoteConfigKey{AppKey: "58d58c60b8ac337293ce2ca6b28b19eb", OrgId: 2, Datacenter: "datadoghq.com"}},
		{input: generateKey(t, 2, "datadoghq.com", ""), err: true},
		{input: generateKey(t, 2, "", "app_Key"), err: true},
		{input: generateKey(t, 0, "datadoghq.com", "app_Key"), err: true},
	}
	for _, test := range tests {
		t.Run(test.input, func(tt *testing.T) {
			output, err := parseRemoteConfigKey(test.input)
			if test.err {
				assert.Error(tt, err)
			} else {
				assert.Equal(tt, test.output, output)
				assert.NoError(tt, err)
			}
		})
	}
}

func generateKey(t *testing.T, orgID int64, datacenter string, appKey string) string {
	key := pbgo.RemoteConfigKey{
		AppKey:     appKey,
		OrgId:      orgID,
		Datacenter: datacenter,
	}
	rawKey, err := proto.Marshal(&key)
	if err != nil {
		t.Fatal(err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(rawKey)
}
