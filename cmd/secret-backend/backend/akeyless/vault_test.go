// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package akeyless

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DataDog/datadog-secret-backend/secret"
	"github.com/go-chi/chi"
	"github.com/stretchr/testify/assert"
)

const testToken = "token"

func mockAkeylessServer(secrets map[string]interface{}) *httptest.Server {
	router := chi.NewRouter()
	router.Post("/get-secret-value", func(w http.ResponseWriter, req *http.Request) {
		secretRequest := secretRequest{}
		err := json.NewDecoder(req.Body).Decode(&secretRequest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// check token
		if secretRequest.Token != testToken {
			http.Error(w, "invalid token", http.StatusUnauthorized)
		}

		resp := secretResponse{}
		for _, name := range secretRequest.Names {
			if val, ok := secrets[name]; ok {
				resp[name] = val.(string)
			}
		}
		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(resp)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	router.Post("/auth", func(w http.ResponseWriter, _ *http.Request) {
		response := authResponse{
			Token: testToken,
		}
		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(response)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	ts := httptest.NewServer(router)
	return ts
}

func TestAkeylessBackend(t *testing.T) {
	ts := mockAkeylessServer(map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
	})
	defer ts.Close()

	akeylessBackendParams := map[string]interface{}{
		"backend_type": "akeyless",
		"akeyless_url": ts.URL,
	}

	akeylessBackend, err := NewAkeylessBackend("akeyless-backend", akeylessBackendParams)
	if err != nil {
		t.Fatalf("failed to create akeyless backend: %v", err)
	}

	secretOutput := akeylessBackend.GetSecretOutput("key1")
	assert.Equal(t, "value1", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)

	secretOutput = akeylessBackend.GetSecretOutput("key2")
	assert.Equal(t, "value2", *secretOutput.Value)
	assert.Nil(t, secretOutput.Error)

	secretOutput = akeylessBackend.GetSecretOutput("key3")
	assert.Nil(t, secretOutput.Value)
	assert.Equal(t, secret.ErrKeyNotFound.Error(), *secretOutput.Error)
}
