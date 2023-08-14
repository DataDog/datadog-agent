// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package languagedetection

func TestBinaryAnalysisClient(t *testing.T) {
	socketPath := startTestUnixServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		b, err := proto.Marshal(&languagepb.DetectLanguageResponse{
			Languages: []*languagepb.Language{
				{
					Name:    string(languagemodels.Go),
					Version: "1.19",
				},
				{
					Name:    string(languagemodels.Go),
					Version: "1.20",
				},
				{
					Name:    string(languagemodels.Go),
					Version: "1.13",
				},
			},
		})
		require.NoError(t, err)

		_, err = writer.Write(b)
		require.NoError(t, err)
	}))

	var procs []*procutil.Process
	for _, command := range [][]string{
		{"python3", "--version"},
		{"go", "run", "main.go"},
		{"java", "-c", "org.elasticsearch.bootstrap.Elasticsearch"},
		{"process-agent"},
		{"my-internal-go-service", "-p", "8080"},
		{"xonotic"},
	} {
		procs = append(procs, makeProcess(command, command[0]))
	}

	cfg := config.Mock(t)
	cfg.Set("system_probe_config.language_detection.enabled", true)
	cfg.Set("system_probe_config.sysprobe_socket", socketPath)

	res := DetectLanguage(procs, cfg)
	assert.Equal(t, languagemodels.Python, res[0].Name)

	assert.Equal(t, languagemodels.Go, res[1].Name)
	assert.Equal(t, "1.19", res[1].Version)

	assert.Equal(t, languagemodels.Java, res[2].Name)

	assert.Equal(t, languagemodels.Go, res[3].Name)
	assert.Equal(t, "1.20", res[3].Version)

	assert.Equal(t, languagemodels.Go, res[4].Name)
	assert.Equal(t, "1.13", res[4].Version)

	assert.Equal(t, languagemodels.Unknown, res[5].Name)
}
