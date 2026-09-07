// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gnmipb

import (
	"testing"

	officialpb "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestSubscribeRequestWireCompatibility(t *testing.T) {
	request := &SubscribeRequest{
		Request: &SubscribeRequest_Subscribe{
			Subscribe: &SubscriptionList{
				Mode:     SubscriptionList_STREAM,
				Encoding: Encoding_JSON_IETF,
				Subscription: []*Subscription{{
					Mode: SubscriptionMode_SAMPLE,
					Path: &Path{Elem: []*PathElem{{
						Name: "interface",
						Key:  map[string]string{"name": "*"},
					}}},
				}},
			},
		},
	}

	raw, err := proto.Marshal(request)
	require.NoError(t, err)

	var official officialpb.SubscribeRequest
	require.NoError(t, proto.Unmarshal(raw, &official))
	require.Equal(t, officialpb.Encoding_JSON_IETF, official.GetSubscribe().GetEncoding())
	require.Equal(t, officialpb.SubscriptionMode_SAMPLE, official.GetSubscribe().GetSubscription()[0].GetMode())
	require.Equal(t, "*", official.GetSubscribe().GetSubscription()[0].GetPath().GetElem()[0].GetKey()["name"])
}

func TestSubscribeResponseWireCompatibility(t *testing.T) {
	official := &officialpb.SubscribeResponse{
		Response: &officialpb.SubscribeResponse_Update{
			Update: &officialpb.Notification{
				Timestamp: 42,
				Prefix:    &officialpb.Path{Origin: "openconfig"},
				Update: []*officialpb.Update{{
					Path: &officialpb.Path{Elem: []*officialpb.PathElem{{Name: "hostname"}}},
					Val:  &officialpb.TypedValue{Value: &officialpb.TypedValue_StringVal{StringVal: "router"}},
				}},
			},
		},
	}

	raw, err := proto.Marshal(official)
	require.NoError(t, err)

	var response SubscribeResponse
	require.NoError(t, proto.Unmarshal(raw, &response))
	update := response.GetUpdate()
	require.NotNil(t, update)
	require.EqualValues(t, 42, update.GetTimestamp())
	require.Equal(t, "openconfig", update.GetPrefix().GetOrigin())
	require.Equal(t, "hostname", update.GetUpdate()[0].GetPath().GetElem()[0].GetName())
	require.Equal(t, "router", update.GetUpdate()[0].GetVal().GetStringVal())
}

func TestSyncResponsePreservesFalsePresence(t *testing.T) {
	official := &officialpb.SubscribeResponse{
		Response: &officialpb.SubscribeResponse_SyncResponse{SyncResponse: false},
	}
	raw, err := proto.Marshal(official)
	require.NoError(t, err)

	var response SubscribeResponse
	require.NoError(t, proto.Unmarshal(raw, &response))
	_, ok := response.GetResponse().(*SubscribeResponse_SyncResponse)
	require.True(t, ok)
	require.False(t, response.GetSyncResponse())
}

func TestTypedValuePreservesZeroValuePresence(t *testing.T) {
	tests := []struct {
		name     string
		official *officialpb.TypedValue
		assert   func(*testing.T, *TypedValue)
	}{
		{
			name:     "empty string",
			official: &officialpb.TypedValue{Value: &officialpb.TypedValue_StringVal{}},
			assert: func(t *testing.T, value *TypedValue) {
				typed, ok := value.GetValue().(*TypedValue_StringVal)
				require.True(t, ok)
				require.Empty(t, typed.StringVal)
			},
		},
		{
			name:     "zero integer",
			official: &officialpb.TypedValue{Value: &officialpb.TypedValue_IntVal{}},
			assert: func(t *testing.T, value *TypedValue) {
				typed, ok := value.GetValue().(*TypedValue_IntVal)
				require.True(t, ok)
				require.Zero(t, typed.IntVal)
			},
		},
		{
			name:     "false boolean",
			official: &officialpb.TypedValue{Value: &officialpb.TypedValue_BoolVal{}},
			assert: func(t *testing.T, value *TypedValue) {
				typed, ok := value.GetValue().(*TypedValue_BoolVal)
				require.True(t, ok)
				require.False(t, typed.BoolVal)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, err := proto.Marshal(test.official)
			require.NoError(t, err)
			var value TypedValue
			require.NoError(t, proto.Unmarshal(raw, &value))
			test.assert(t, &value)
		})
	}
}
