// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostname

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"hostname"},
		printHostname,
		func(_ *cliParams, _ core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, false, secretParams.Enabled)
		})
}

func hostnameHandler(hostname string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agent/hostname" || r.Method != http.MethodGet {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		if hostname == "" {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		hname, err := json.Marshal(hostname)
		if err != nil {
			http.Error(w, "Internal Server Error (marshalling)", http.StatusInternalServerError)
			return
		}
		w.Write(hname)
	})
}

func TestGetHostname(t *testing.T) {
	testCases := []struct {
		name           string
		forceLocal     bool
		remoteHostname string
	}{
		{
			name:           "remote",
			forceLocal:     false,
			remoteHostname: "remotehostname",
		},
		{
			name:           "forceLocal",
			forceLocal:     true,
			remoteHostname: "remotehostname",
		},
		{
			name:           "remoteFallbackLocal",
			forceLocal:     false,
			remoteHostname: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := config.NewMock(t)
			logmock.New(t)
			cliParams := &cliParams{
				GlobalParams: &command.GlobalParams{},
				forceLocal:   tc.forceLocal,
			}

			authComp := ipcmock.New(t)
			server := authComp.NewMockServer(hostnameHandler(tc.remoteHostname))

			serverURL, err := url.Parse(server.URL)
			require.NoError(t, err)

			localHostname := "localhostname"
			config.Set("hostname", localHostname, model.SourceFile)
			config.Set("cmd_host", serverURL.Hostname(), model.SourceFile)
			config.Set("cmd_port", serverURL.Port(), model.SourceFile)

			hname, err := getHostname(cliParams, authComp.GetClient())
			require.NoError(t, err)

			expectedHostname := localHostname
			if !tc.forceLocal && tc.remoteHostname != "" {
				expectedHostname = tc.remoteHostname
			}
			require.Equal(t, expectedHostname, hname)
		})
	}
}
