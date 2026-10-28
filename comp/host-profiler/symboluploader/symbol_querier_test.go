// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package symboluploader

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/DataDog/jsonapi"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/require"
)

// TestQuerySymbolsSendsCandidates verifies schema of an outgoing query.
func TestQuerySymbolsSendsCandidates(t *testing.T) {
	httpmock.Activate()
	t.Cleanup(httpmock.DeactivateAndReset)

	querier, err := NewDatadogSymbolQuerier("datad0g.com", "api-key")
	require.NoError(t, err)

	emptyResponse, err := jsonapi.Marshal(&[]SymbolFile{})
	require.NoError(t, err)

	var rawBody []byte
	httpmock.RegisterResponder(http.MethodPost, buildSymbolQueryURL("datad0g.com"),
		func(req *http.Request) (*http.Response, error) {
			b, readErr := io.ReadAll(req.Body)
			if readErr != nil {
				return nil, readErr
			}
			rawBody = b
			return httpmock.NewBytesResponse(http.StatusOK, emptyResponse), nil
		})

	candidates := []SymbolCandidate{
		{BuildId: "buildid1", FileName: "foo", SymbolSource: "symbol_table"},
	}
	_, err = querier.QuerySymbols(t.Context(), candidates, "arch1")
	require.NoError(t, err)
	require.NotEmpty(t, rawBody)

	body := string(rawBody)
	require.Contains(t, body, `"arch":"arch1"`)
	require.Contains(t, body, `"buildId":"buildid1"`)
	require.Contains(t, body, `"fileName":"foo"`)
	require.Contains(t, body, `"symbolSource":"symbol_table"`)
	require.NotContains(t, body, `"buildIds"`)
}

// TestQuerySymbolsDecodesDecisions verifies that decoding a response tolerates attributes
// the agent does not model
func TestQuerySymbolsDecodesDecisions(t *testing.T) {
	httpmock.Activate()
	t.Cleanup(httpmock.DeactivateAndReset)

	querier, err := NewDatadogSymbolQuerier("datad0g.com", "api-key")
	require.NoError(t, err)

	want := []SymbolFile{
		{ID: "0", BuildId: "buildid1", SymbolSource: "symbol_table", BuildIdType: "gnu_build_id", Decision: decisionUpload},
		{ID: "1", BuildId: "buildid2", SymbolSource: "debug_info", BuildIdType: "", Decision: decisionNoUpload},
	}
	body, err := jsonapi.Marshal(&want)
	require.NoError(t, err)

	withExtra := bytes.ReplaceAll(body,
		[]byte(`"decision":"no_upload"`),
		[]byte(`"decision":"no_upload","retryAfterSeconds":5,"reason":"unexpected"`))
	require.NotEqual(t, body, withExtra, "fixture must actually inject the extra attributes")

	httpmock.RegisterResponder(http.MethodPost, buildSymbolQueryURL("datad0g.com"),
		httpmock.NewBytesResponder(http.StatusOK, withExtra))

	files, err := querier.QuerySymbols(t.Context(),
		[]SymbolCandidate{{BuildId: "buildid1"}, {BuildId: "buildid2"}}, "arch1")
	require.NoError(t, err)
	require.Equal(t, want, files)
}
