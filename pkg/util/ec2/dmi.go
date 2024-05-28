// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/dmi"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	"github.com/google/uuid"
)

func isBoardVendorEC2() bool {
	if !config.Datadog().GetBool("ec2_use_dmi") {
		return false
	}
	return dmi.GetBoardVendor() == DMIBoardVendor
}

// getInstanceIDFromDMI fetches the instance id for current host from DMI
//
// On AWS Nitro instances dmi information contains the instanceID for the host. We check that the board vendor is
// EC2 and that the board_asset_tag match an instanceID format before using it
func getInstanceIDFromDMI() (string, error) {
	// we don't want to collect anything on Fargate
	if fargate.IsFargateInstance() {
		return "", fmt.Errorf("host alias detection through DMI is disabled on Fargate")
	}

	if !config.Datadog().GetBool("ec2_use_dmi") {
		return "", fmt.Errorf("'ec2_use_dmi' is disabled")
	}

	if !isBoardVendorEC2() {
		isEC2UUID()
		return "", fmt.Errorf("board vendor is not AWS")
	}

	boardAssetTag := dmi.GetBoardAssetTag()
	if !strings.HasPrefix(boardAssetTag, "i-") {
		isEC2UUID()
		return "", fmt.Errorf("invalid board_asset_tag: '%s'", boardAssetTag)
	}
	setCloudProviderSource(metadataSourceDMI)
	return boardAssetTag, nil
}

// isEC2UUID returns true if the hypervisor or product UUID starts by "ec2". This doesn't tell us on which instances the
// agent is running but let us know we're on EC2. Source
// https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/identify_ec2_instances.html.
//
// Depending on the instance type either the DMI product UUID or the hypervisor UUID is available. In both case, if they
// start with "ec2" we return true.
func isEC2UUID() bool {
	if !config.Datadog().GetBool("ec2_use_dmi") {
		return false
	}

	// if we have a board vendor we can skip this UUID check
	if dmi.GetBoardVendor() != "" && !isBoardVendorEC2() {
		return false
	}

	uuidData := dmi.GetProductUUID()
	if uuidData == "" {
		uuidData = dmi.GetHypervisorUUID()
	}

	if uuidData == "" {
		return false
	}

	if strings.HasPrefix(strings.ToLower(uuidData), "ec2") {
		setCloudProviderSource(metadataSourceUUID)
		return true
	}

	// Some SMBIOS might represent the UUID in little-endian format. We swap the endianness and re-check for the
	// "ec2" prefix (according to EC2 documentation linked above).
	uuidObj, err := uuid.Parse(uuidData)
	if err != nil {
		return false
	}

	IDPart := uuidObj.ID()

	b := make([]byte, 2)
	b[0] = byte(IDPart)
	b[1] = byte(IDPart >> 8)

	swapID := fmt.Sprintf("%x", b)
	if strings.HasPrefix(strings.ToLower(swapID), "ec2") {
		setCloudProviderSource(metadataSourceUUID)
		return true
	}

	return false
}
