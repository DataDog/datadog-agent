// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	ec2internal "github.com/DataDog/datadog-agent/pkg/util/ec2/internal"
)

func TestGetInstanceIdentity(t *testing.T) {
	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		content, err := os.ReadFile("payloads/instance_indentity.json")
		require.NoError(t, err, fmt.Sprintf("failed to load json in payloads/instance_indentity.json: %v", err))
		w.Write(content)
	}))
	defer ts.Close()
	ec2internal.InstanceIdentityURL = ts.URL
	conf := configmock.New(t)
	conf.SetInTest("ec2_metadata_timeout", 1000)

	assert.EventuallyWithT(
		t, func(_ *assert.CollectT) {
			val, err := GetInstanceIdentity(ctx)
			require.NoError(t, err)
			assert.Equal(t, "us-east-1", val.Region)
			assert.Equal(t, "i-aaaaaaaaaaaaaaaaa", val.InstanceID)
			assert.Equal(t, "REMOVED", val.AccountID)
		},
		10*time.Second, 1*time.Second)
}
