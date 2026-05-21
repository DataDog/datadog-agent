// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package main is the Pulumi entrypoint for the netpath-vm scenario: a
// long-lived AWS EC2 VM running the Datadog Agent with Cloud Network
// Monitoring (NPM) and Network Path dynamic tests enabled, plus a periodic
// outbound TCP/UDP workload so dynamic paths have live flows to discover.
//
// One Pulumi stack per Datadog environment (e.g. us1-prod, eu1-staging).
// The program is fully self-contained — it does not depend on the
// e2e-framework, so it works in any AWS account where the caller has EC2
// + IAM permissions. Access to the VM is via AWS Systems Manager Session
// Manager (no SSH keypair, no SSH ingress required).
package main

import (
	"encoding/base64"
	_ "embed"
	"fmt"
	"strconv"
	"strings"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ssm"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

//go:embed assets/targets.txt
var defaultTargets string

//go:embed assets/conn-gen.sh
var connGenScript string

//go:embed assets/conn-gen.service
var connGenServiceUnit string

//go:embed assets/conn-gen.timer
var connGenTimerUnit string

//go:embed assets/network_path.yaml
var defaultNetworkPathIntegration string

const (
	configNamespace     = "netpath"
	defaultSite         = "datadoghq.com"
	defaultInstanceType = "t3.2xlarge"
	defaultName         = "netpath-vm"

	// SSM public parameter for the latest Amazon Linux 2023 x86_64 AMI.
	al2023AMIParameter = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64"

	// AWS-managed policy granting just enough perms for SSM Session Manager
	// agent to register the instance and accept sessions.
	ssmManagedInstanceCorePolicy = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"

	ec2AssumeRolePolicy = `{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {"Service": "ec2.amazonaws.com"},
    "Action": "sts:AssumeRole"
  }]
}`
)

func main() {
	pulumi.Run(run)
}

func run(ctx *pulumi.Context) error {
	cfg := config.New(ctx, configNamespace)

	apiKey := cfg.RequireSecret("apiKey")

	site := getOr(cfg, "site", defaultSite)
	name := getOr(cfg, "name", defaultName)
	instanceType := getOr(cfg, "instanceType", defaultInstanceType)
	targets := getOr(cfg, "targets", defaultTargets)
	tags := splitAndTrim(cfg.Get("tags"), ",")

	scheduledIntegration := ""
	if getBoolOr(cfg, "enableScheduled", true) {
		scheduledIntegration = getOr(cfg, "scheduledConfig", defaultNetworkPathIntegration)
	}

	defaultVpc, err := ec2.LookupVpc(ctx, &ec2.LookupVpcArgs{
		Default: pulumi.BoolRef(true),
	}, nil)
	if err != nil {
		return fmt.Errorf("lookup default VPC: %w", err)
	}

	subnets, err := ec2.GetSubnets(ctx, &ec2.GetSubnetsArgs{
		Filters: []ec2.GetSubnetsFilter{
			{Name: "vpc-id", Values: []string{defaultVpc.Id}},
		},
	}, nil)
	if err != nil {
		return fmt.Errorf("list subnets in default VPC %s: %w", defaultVpc.Id, err)
	}
	if len(subnets.Ids) == 0 {
		return fmt.Errorf("no subnets found in default VPC %s", defaultVpc.Id)
	}

	ami, err := ssm.LookupParameter(ctx, &ssm.LookupParameterArgs{
		Name: al2023AMIParameter,
	}, nil)
	if err != nil {
		return fmt.Errorf("lookup Amazon Linux 2023 AMI: %w", err)
	}

	role, err := iam.NewRole(ctx, name+"-role", &iam.RoleArgs{
		AssumeRolePolicy: pulumi.String(ec2AssumeRolePolicy),
		Description:      pulumi.Sprintf("EC2 instance role for %s (SSM Session Manager access)", name),
	})
	if err != nil {
		return err
	}

	_, err = iam.NewRolePolicyAttachment(ctx, name+"-ssm", &iam.RolePolicyAttachmentArgs{
		Role:      role.Name,
		PolicyArn: pulumi.String(ssmManagedInstanceCorePolicy),
	})
	if err != nil {
		return err
	}

	instanceProfile, err := iam.NewInstanceProfile(ctx, name+"-profile", &iam.InstanceProfileArgs{
		Role: role.Name,
	})
	if err != nil {
		return err
	}

	// Egress-only security group: SSM Session Manager doesn't need any
	// inbound rules — the SSM agent on the instance opens an outbound
	// connection to the SSM service.
	sg, err := ec2.NewSecurityGroup(ctx, name+"-sg", &ec2.SecurityGroupArgs{
		VpcId:       pulumi.String(defaultVpc.Id),
		Description: pulumi.Sprintf("Egress-only SG for %s (SSM access)", name),
		Egress: ec2.SecurityGroupEgressArray{
			&ec2.SecurityGroupEgressArgs{
				Protocol:    pulumi.String("-1"),
				FromPort:    pulumi.Int(0),
				ToPort:      pulumi.Int(0),
				CidrBlocks:  pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				Description: pulumi.String("All egress"),
			},
		},
		Tags: pulumi.StringMap{"Name": pulumi.String(name + "-sg")},
	})
	if err != nil {
		return err
	}

	userData := apiKey.ApplyT(func(k string) string {
		return buildUserData(k, site, tags, targets, scheduledIntegration)
	}).(pulumi.StringOutput)

	instance, err := ec2.NewInstance(ctx, name, &ec2.InstanceArgs{
		Ami:                      pulumi.String(ami.Value),
		InstanceType:             pulumi.String(instanceType),
		IamInstanceProfile:       instanceProfile.Name,
		SubnetId:                 pulumi.String(subnets.Ids[0]),
		VpcSecurityGroupIds:      pulumi.StringArray{sg.ID()},
		AssociatePublicIpAddress: pulumi.Bool(true),
		UserData:                 userData,
		UserDataReplaceOnChange:  pulumi.Bool(true),
		Tags: pulumi.StringMap{
			"Name":    pulumi.String(name),
			"purpose": pulumi.String("netpath-demo"),
		},
	})
	if err != nil {
		return err
	}

	ctx.Export("vmInstanceId", instance.ID())
	ctx.Export("vmPublicIp", instance.PublicIp)
	ctx.Export("ssmCommand", pulumi.Sprintf("aws ssm start-session --target %s", instance.ID()))

	return nil
}

// buildUserData composes the cloud-init shell script that the EC2 instance
// runs on first boot. The order matters: install workload deps, drop the
// conn-gen assets, install the agent (creating the dd-agent user), then
// overwrite the agent's baseline config with our network-path-aware version
// and start everything up.
func buildUserData(apiKey, site string, tags []string, targets, scheduledIntegration string) string {
	datadogYAML := buildDatadogYAML(apiKey, site, tags)
	systemProbeYAML := "network_config:\n  enabled: true\ntraceroute:\n  enabled: true\n"

	scheduledBlock := ""
	if scheduledIntegration != "" {
		scheduledBlock = fmt.Sprintf(`
mkdir -p /etc/datadog-agent/conf.d/network_path.d
echo %s | base64 -d > /etc/datadog-agent/conf.d/network_path.d/conf.yaml
chown -R dd-agent:dd-agent /etc/datadog-agent/conf.d/network_path.d
chmod 0640 /etc/datadog-agent/conf.d/network_path.d/conf.yaml
`, b64(scheduledIntegration))
	}

	return fmt.Sprintf(`#!/bin/bash
set -euxo pipefail

dnf install -y bind-utils

mkdir -p /etc/datadog-agent
echo %s | base64 -d > /etc/datadog-agent/conn-gen-targets.txt
chmod 0644 /etc/datadog-agent/conn-gen-targets.txt
echo %s | base64 -d > /usr/local/bin/conn-gen.sh
chmod 0755 /usr/local/bin/conn-gen.sh
echo %s | base64 -d > /etc/systemd/system/conn-gen.service
echo %s | base64 -d > /etc/systemd/system/conn-gen.timer

DD_API_KEY=%q DD_SITE=%q DD_INSTALL_ONLY=true \
  bash -c "$(curl -L https://install.datadoghq.com/scripts/install_script_agent7.sh)"

echo %s | base64 -d > /etc/datadog-agent/datadog.yaml
chown dd-agent:dd-agent /etc/datadog-agent/datadog.yaml
chmod 0640 /etc/datadog-agent/datadog.yaml

echo %s | base64 -d > /etc/datadog-agent/system-probe.yaml
chown dd-agent:dd-agent /etc/datadog-agent/system-probe.yaml
chmod 0640 /etc/datadog-agent/system-probe.yaml
%s
systemctl enable --now datadog-agent
systemctl enable --now datadog-agent-sysprobe

systemctl daemon-reload
systemctl enable --now conn-gen.timer
`,
		b64(targets),
		b64(connGenScript),
		b64(connGenServiceUnit),
		b64(connGenTimerUnit),
		apiKey,
		site,
		b64(datadogYAML),
		b64(systemProbeYAML),
		scheduledBlock,
	)
}

func buildDatadogYAML(apiKey, site string, tags []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "api_key: %s\nsite: %s\n", apiKey, site)
	if len(tags) > 0 {
		b.WriteString("tags:\n")
		for _, t := range tags {
			fmt.Fprintf(&b, "  - %s\n", t)
		}
	}
	b.WriteString(`network_path:
  connections_monitoring:
    enabled: true
  collector:
    workers: 4
    pathtest_interval: 5m
    pathtest_ttl: 70m
    timeout: 1000
synthetics:
  collector:
    enabled: true
`)
	return b.String()
}

func b64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func getOr(cfg *config.Config, key, fallback string) string {
	if v := cfg.Get(key); v != "" {
		return v
	}
	return fallback
}

func getBoolOr(cfg *config.Config, key string, fallback bool) bool {
	v := cfg.Get(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func splitAndTrim(s, sep string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
