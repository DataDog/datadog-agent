// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installer contains tests for the datadog installer
package installer

// type configSuite struct {
// 	packageBaseSuite
// }

// func testConfig(os e2eos.Descriptor, arch e2eos.Architecture, method InstallMethodOption) packageSuite {
// 	return &configSuite{
// 		packageBaseSuite: newPackageSuite("config", os, arch, method),
// 	}
// }

// func (s *configSuite) TestConfig() {
// 	s.agent.MustInstall(agent.WithRemoteUpdates())
// 	defer s.agent.MustUninstall()

// 	err := s.backend.StartConfigExperiment(fleetbackend.ConfigOperations{
// 		DeploymentID:   "123",
// 		FileOperations: []fleetbackend.FileOperation{{FileOperationType: fleetbackend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)}},
// 	})
// 	require.NoError(s.T(), err)
// 	config, err := s.agent.Configuration()
// 	require.NoError(s.T(), err)
// 	require.Equal(s.T(), "debug", config["log_level"])
// 	err = s.backend.PromoteConfigExperiment()
// 	require.NoError(s.T(), err)

// 	config, err = s.agent.Configuration()
// 	require.NoError(s.T(), err)
// 	require.Equal(s.T(), "debug", config["log_level"])
// }

// func (s *configSuite) TestMultipleConfigs() {
// 	s.agent.MustInstall(agent.WithRemoteUpdates())
// 	defer s.agent.MustUninstall()

// 	for i := 0; i < 3; i++ {
// 		err := s.backend.StartConfigExperiment(fleetbackend.ConfigOperations{
// 			DeploymentID: fmt.Sprintf("123-%d", i),
// 			FileOperations: []fleetbackend.FileOperation{
// 				{
// 					FileOperationType: fleetbackend.FileOperationMergePatch,
// 					FilePath:          "/datadog.yaml",
// 					Patch:             []byte(fmt.Sprintf(`{"extra_tags": ["debug:step-%d"]}`, i)),
// 				},
// 			},
// 		})
// 		require.NoError(s.T(), err)
// 		config, err := s.agent.Configuration()
// 		require.NoError(s.T(), err)
// 		// Convert extra_tags to a slice of strings
// 		extraTags := config["extra_tags"].([]interface{})
// 		extraTagsStrings := make([]string, len(extraTags))
// 		for i, tag := range extraTags {
// 			var ok bool
// 			extraTagsStrings[i], ok = tag.(string)
// 			require.True(s.T(), ok, "tag %d is not a string", i)
// 		}
// 		require.Equal(s.T(), []string{fmt.Sprintf("debug:step-%d", i)}, extraTagsStrings)
// 		err = s.backend.PromoteConfigExperiment()
// 		require.NoError(s.T(), err)

// 		config, err = s.agent.Configuration()
// 		require.NoError(s.T(), err)
// 		// Convert extra_tags to a slice of strings
// 		extraTags = config["extra_tags"].([]interface{})
// 		extraTagsStrings = make([]string, len(extraTags))
// 		for i, tag := range extraTags {
// 			var ok bool
// 			extraTagsStrings[i], ok = tag.(string)
// 			require.True(s.T(), ok, "tag %d is not a string", i)
// 		}
// 		require.Equal(s.T(), []string{fmt.Sprintf("debug:step-%d", i)}, extraTagsStrings)
// 	}
// }

// func (s *configSuite) TestConfigFailureCrash() {
// 	s.agent.MustInstall(agent.WithRemoteUpdates())
// 	defer s.agent.MustUninstall()

// 	err := s.backend.StartConfigExperiment(fleetbackend.ConfigOperations{
// 		DeploymentID:   "123",
// 		FileOperations: []fleetbackend.FileOperation{{FileOperationType: fleetbackend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "ENC[invalid_secret]"}`)}},
// 	})
// 	require.NoError(s.T(), err)

// 	config, err := s.agent.Configuration()
// 	require.NoError(s.T(), err)
// 	require.Equal(s.T(), "info", config["log_level"])
// }

// func (s *configSuite) TestConfigFailureTimeout() {
// 	s.agent.MustInstall(agent.WithRemoteUpdates())
// 	defer s.agent.MustUninstall()
// 	s.agent.MustSetExperimentTimeout(60 * time.Second)
// 	defer s.agent.MustUnsetExperimentTimeout()

// 	err := s.backend.StartConfigExperiment(fleetbackend.ConfigOperations{
// 		DeploymentID:   "123",
// 		FileOperations: []fleetbackend.FileOperation{{FileOperationType: fleetbackend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)}},
// 	})
// 	require.NoError(s.T(), err)
// 	config, err := s.agent.Configuration()
// 	require.NoError(s.T(), err)
// 	require.Equal(s.T(), "debug", config["log_level"])

// 	time.Sleep(60 * time.Second)
// 	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
// 		config, err := s.agent.Configuration()
// 		require.NoError(c, err)
// 		require.Equal(c, "info", config["log_level"])
// 	}, 60*time.Second, 5*time.Second)
// }

// func (s *configSuite) TestConfigFailureHealth() {
// 	if s.Env().RemoteHost.OSFlavor == e2eos.CentOS && s.Env().RemoteHost.OSVersion == e2eos.CentOS7.Version {
// 		s.T().Skip("FIXME: Broken on CentOS 7 for some unknown reason")
// 	}
// 	s.agent.MustInstall(agent.WithRemoteUpdates())
// 	defer s.agent.MustUninstall()

// 	err := s.backend.StartConfigExperiment(fleetbackend.ConfigOperations{
// 		DeploymentID:   "123",
// 		FileOperations: []fleetbackend.FileOperation{{FileOperationType: fleetbackend.FileOperationMergePatch, FilePath: "/datadog.yaml", Patch: []byte(`{"log_level": "debug"}`)}},
// 	})
// 	require.NoError(s.T(), err)
// 	config, err := s.agent.Configuration()
// 	require.NoError(s.T(), err)
// 	require.Equal(s.T(), "debug", config["log_level"])

// 	err = s.backend.StopConfigExperiment()
// 	require.NoError(s.T(), err)
// 	config, err = s.agent.Configuration()
// 	require.NoError(s.T(), err)
// 	require.Equal(s.T(), "info", config["log_level"])
// }

// func (s *configSuite) TestConfigFilePermissions() {
// 	// Skip on Windows as POSIX permissions don't apply
// 	if s.Env().RemoteHost.OSFamily == e2eos.WindowsFamily {
// 		s.T().Skip("Skipping test on Windows - POSIX permissions not applicable")
// 	}

// 	s.agent.MustInstall(agent.WithRemoteUpdates())
// 	defer s.agent.MustUninstall()

// 	// Configure multiple files with different permission requirements
// 	nginxConfig := `{
// 		"init_config": {},
// 		"instances": [
// 			{
// 				"nginx_status_url": "http://localhost:8080/status"
// 			}
// 		]
// 	}`

// 	err := s.backend.StartConfigExperiment(fleetbackend.ConfigOperations{
// 		DeploymentID: "file-permissions",
// 		FileOperations: []fleetbackend.FileOperation{
// 			{
// 				FileOperationType: fleetbackend.FileOperationMergePatch,
// 				FilePath:          "/datadog.yaml",
// 				Patch:             []byte(`{"log_level": "debug"}`),
// 			},
// 			{
// 				FileOperationType: fleetbackend.FileOperationMergePatch,
// 				FilePath:          "/application_monitoring.yaml",
// 				Patch:             []byte(`{"enabled": true}`),
// 			},
// 			{
// 				FileOperationType: fleetbackend.FileOperationMergePatch,
// 				FilePath:          "/conf.d/nginx.yaml",
// 				Patch:             []byte(nginxConfig),
// 			},
// 		},
// 	})
// 	require.NoError(s.T(), err)

// 	// Check file permissions in experiment directory
// 	state := s.host.State()
// 	state.AssertFileExists("/etc/datadog-agent-exp/datadog.yaml", 0640, "dd-agent", "dd-agent")
// 	state.AssertFileExists("/etc/datadog-agent-exp/application_monitoring.yaml", 0644, "root", "root")
// 	state.AssertFileExists("/etc/datadog-agent-exp/conf.d/nginx.yaml", 0640, "dd-agent", "dd-agent")

// 	// Promote and verify permissions persist
// 	err = s.backend.PromoteConfigExperiment()
// 	require.NoError(s.T(), err)

// 	// Verify permissions after promotion
// 	state = s.host.State()
// 	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")
// 	state.AssertFileExists("/etc/datadog-agent/application_monitoring.yaml", 0644, "root", "root")
// 	state.AssertFileExists("/etc/datadog-agent/conf.d/nginx.yaml", 0640, "dd-agent", "dd-agent")
// }
