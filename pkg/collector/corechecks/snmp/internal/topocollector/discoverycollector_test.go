package topocollector

//
//func TestDiscoveryCollector_Collect(t *testing.T) {
//	// Setup NetFlow feature config
//	coreconfig.Datadog.SetConfigType("yaml")
//	err := coreconfig.Datadog.MergeConfigOverride(strings.NewReader(fmt.Sprintf(`
//network_devices:
//  discovery:
//    enabled: true
//    stop_timeout: 10
//    ip_address: 127.0.0.1
//    port: 1161
//    snmp_version: 2
//    community_string: ciena-sds
//    oid_batch_size: 10
//`)))
//	require.NoError(t, err)
//
//	// Setup NetFlow Server
//	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(1 * time.Millisecond)
//	defer demux.Stop(false)
//
//	sender, err := demux.GetDefaultSender()
//	require.NoError(t, err, "cannot get default sender")
//	mainConfig, err := config.ReadConfig()
//	assert.NoError(t, err)
//
//	profiles := []string{"ciena-sds", "aos", "aos6"}
//	for _, profile := range profiles {
//		mainConfig.CommunityString = profile
//		dc := &DiscoveryCollector{
//			sender:   sender,
//			hostname: "my-hostname",
//			config:   mainConfig,
//		}
//		dc.Collect()
//	}
//}
