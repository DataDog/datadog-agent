// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

func init() {
	registerGroup(&dsdGroup)
}

var dsdGroup = Group{
	Name:        "Dogstatsd",
	Description: "Dogstatsd enables you to submit custom metrics, via UDP or Unix Sockets",
	Priority:    20,
	Options: []*Option{
		{
			Name:         "use_dogstatsd",
			DefaultValue: true,
			Description:  "Run Dogstatsd for custom metrics + JMX",
		},
		{
			Name:         "dogstatsd_port",
			DefaultValue: int64(8125),
			Description:  "UDP port Dogstatsd will listen to (0 to disable)",
		},
		{
			Name:         "dogstatsd_non_local_traffic",
			DefaultValue: false,
			Description:  "Whether dogstatsd should listen to UDP traffic on all interfaces",
		},
		{
			Name:         "bind_host",
			DefaultValue: "localhost",
			Description:  "If dogstatsd_non_local_traffic is disabled, host to bind to",
		},
		{
			Name:         "dogstatsd_buffer_size",
			DefaultValue: int64(8192), // 8 KiB
			Description:  "The buffer size use to receive statsd packet, in bytes",
			YAMLDesc: `
The buffer size use to receive statsd packet, in bytes.
Adjust accordingly to your client-side buffering settings.`,
		},
		{
			Name:         "dogstatsd_socket",
			DefaultValue: "",
			Description:  "Path of the Unix Socket to listen to (empty string to disable)",
		},
		{
			Name:         "dogstatsd_origin_detection",
			DefaultValue: false,
			Description:  "Enable origin detection and container tagging for Unix Socket traffic",
		},
		{
			Name:         "statsd_metric_namespace",
			DefaultValue: "",
			Description:  "Prefix to add to all custom metric names",
		},
		{
			Name:         "statsd_forward_host",
			DefaultValue: "",
			Description:  "Hostname to forward all incomping statsd packets to",
			YAMLDesc: `
Dogstatsd can forward every packet received to another statsd server.
WARNING: Make sure that forwarded packets are regular statsd packets and not "dogstatsd" packets,
as your other statsd server might not be able to handle them.`,
		},
		{
			Name:         "statsd_forward_port",
			DefaultValue: int64(0),
			Description:  "Port to forward all incomping statsd packets to",
			SkipYAMLDesc: true,
		},
		{
			Name:         "dogstatsd_expiry_seconds",
			DefaultValue: float64(300),
			Description:  "Duration idle counters are kept before expiring",
		},
		{
			Name:         "dogstatsd_stats_enable",
			DefaultValue: false,
			Description:  "Publish internal stats as Go epxvars",
		},
		{
			Name:         "dogstatsd_stats_port",
			DefaultValue: int64(5000),
			Description:  "The port to expose stats to",
		},
		{
			Name:         "dogstatsd_stats_buffer",
			DefaultValue: int64(10),
			Description:  "How many stat batches to keep in memory",
		},
	},
}
