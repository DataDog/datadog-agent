// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package eks

import (
	"net/netip"
	"testing"

	awsEc2 "github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func TestNoInternetEgressRules(t *testing.T) {
	// These are the QA VPC's primary, node, and custom-CNI pod networks.
	// Restricting egress to only CidrBlock prevents nodes from joining EKS.
	vpc := &awsEc2.LookupVpcResult{
		CidrBlock: "10.1.48.0/21",
		CidrBlockAssociations: []awsEc2.GetVpcCidrBlockAssociation{
			{CidrBlock: "10.1.48.0/21"},
			{CidrBlock: "10.255.96.0/19"},
			{CidrBlock: "100.64.0.0/16"},
		},
	}
	rules := noInternetEgressRules(vpc, "pl-s3")
	if len(rules) != 2 {
		t.Fatalf("expected VPC and S3 rules, got %d", len(rules))
	}
	vpcRule := rules[0].(awsEc2.SecurityGroupEgressArgs)
	if vpcRule.Protocol != pulumi.String("-1") || vpcRule.FromPort != pulumi.Int(0) || vpcRule.ToPort != pulumi.Int(0) {
		t.Fatal("VPC networks must permit node, pod, and control-plane traffic")
	}
	cidrs := vpcRule.CidrBlocks.(pulumi.StringArray)
	if len(cidrs) != 3 {
		t.Fatalf("expected three distinct VPC CIDRs, got %v", cidrs)
	}
	for _, tc := range []struct {
		address string
		allowed bool
	}{
		{"10.1.48.2", true},
		{"10.255.99.67", true},
		{"100.64.0.1", true},
		{"8.8.8.8", false},
		{"10.2.0.1", false},
	} {
		allowed := false
		for _, cidr := range cidrs {
			allowed = allowed || netip.MustParsePrefix(string(cidr.(pulumi.String))).Contains(netip.MustParseAddr(tc.address))
		}
		if allowed != tc.allowed {
			t.Errorf("egress to %s: allowed=%v, want %v", tc.address, allowed, tc.allowed)
		}
	}
	s3Rule := rules[1].(awsEc2.SecurityGroupEgressArgs)
	if s3Rule.Protocol != pulumi.String("tcp") || s3Rule.FromPort != pulumi.Int(443) || s3Rule.ToPort != pulumi.Int(443) {
		t.Fatal("S3 egress must be limited to HTTPS")
	}
	if ids := s3Rule.PrefixListIds.(pulumi.StringArray); len(ids) != 1 || ids[0] != pulumi.String("pl-s3") {
		t.Fatalf("unexpected S3 destinations: %v", ids)
	}
	if s3Rule.CidrBlocks != nil {
		t.Fatal("S3 egress must use the service prefix list, not arbitrary CIDRs")
	}
}
