// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

/*
Package metadata provides clients for Metadata APIs exposed by the ECS agent.
There are three versions of these APIs:

	- V1: also called introspection endpoint.

	- V2: available since ecs-agent 1.17.0 with the EC2 launch type and since
	platform version 1.1.0 with the Fargate launch type.

	- V3: available since ecs-agent 1.21.0 with the EC2 launch type and since
	platform version 1.3.0 with the Faragate launch type.

Each of these versions sits in its own subpackage.
*/
package metadata
