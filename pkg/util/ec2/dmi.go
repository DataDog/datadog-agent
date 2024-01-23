// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

func isBoardVendorEC2() bool {
	panic("not called")
}

// getInstanceIDFromDMI fetches the instance id for current host from DMI
//
// On AWS Nitro instances dmi information contains the instanceID for the host. We check that the board vendor is
// EC2 and that the board_asset_tag match an instanceID format before using it
func getInstanceIDFromDMI() (string, error) {
	panic("not called")
}

// isEC2UUID returns true if the hypervisor or product UUID starts by "ec2". This doesn't tell us on which instances the
// agent is running but let us know we're on EC2. Source
// https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/identify_ec2_instances.html.
//
// Depending on the instance type either the DMI product UUID or the hypervisor UUID is available. In both case, if they
// start with "ec2" we return true.
func isEC2UUID() bool {
	panic("not called")
}
