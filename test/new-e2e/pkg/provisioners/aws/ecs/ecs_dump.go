// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ecs

import (
	"context"
	"fmt"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	awsecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// fakeintakeClusterNamePrefix identifies the shared, long-lived cluster pool
// that the Fakeintake Fargate service is deployed into (one is chosen at
// random per-stack, see aws.Environment.ECSFargateFakeintakeClusterArn).
// Unlike the per-job ephemeral clusters used by the rest of the stack, this
// pool isn't known ahead of time from the stack name alone, so on failure we
// scan every cluster matching this prefix for a service belonging to this
// stack.
const fakeintakeClusterNamePrefix = "fakeintake-ecs"

func ecsFakeintakeDiagnoseFunc(ctx context.Context, stackName string) (string, error) {
	dumpResult, err := DumpECSFakeintakeState(ctx, stackName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Dumping fakeintake ECS service state:\n%s", dumpResult), nil
}

// DumpECSFakeintakeState dumps ECS service/task diagnostics (status, recent
// events, task stoppedReason/exitCode/ENI attachment info) for the fakeintake
// Fargate service belonging to the given stack. Exported so other provisioners
// that also deploy a fakeintake ECS service (Kind, EKS, Docker) can fold this
// into their own Diagnose callback.
func DumpECSFakeintakeState(ctx context.Context, stackName string) (string, error) {
	var out strings.Builder

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to load AWS config: %v", err)
	}
	client := awsecs.NewFromConfig(cfg)

	clusterArns, err := listAllClusterArns(ctx, client)
	if err != nil {
		return "", fmt.Errorf("failed to list ECS clusters: %v", err)
	}

	found := false
	for _, clusterArn := range clusterArns {
		if !strings.Contains(clusterArn, fakeintakeClusterNamePrefix) {
			continue
		}

		serviceArns, err := listAllServiceArns(ctx, client, clusterArn)
		if err != nil {
			fmt.Fprintf(&out, "failed to list services in cluster %s: %v\n", clusterArn, err)
			continue
		}

		var matchingServiceArns []string
		for _, svcArn := range serviceArns {
			if strings.Contains(svcArn, stackName) {
				matchingServiceArns = append(matchingServiceArns, svcArn)
			}
		}
		if len(matchingServiceArns) == 0 {
			continue
		}
		found = true

		if err := dumpServices(ctx, client, clusterArn, matchingServiceArns, &out); err != nil {
			fmt.Fprintf(&out, "failed to describe services %v in cluster %s: %v\n", matchingServiceArns, clusterArn, err)
		}
	}

	if !found {
		fmt.Fprintf(&out, "no fakeintake service matching stack %q found in any cluster prefixed %q (already cleaned up, or provisioning never reached fakeintake service creation)\n", stackName, fakeintakeClusterNamePrefix)
	}

	return out.String(), nil
}

func listAllClusterArns(ctx context.Context, client *awsecs.Client) ([]string, error) {
	var arns []string
	var nextToken *string
	for {
		out, err := client.ListClusters(ctx, &awsecs.ListClustersInput{NextToken: nextToken})
		if err != nil {
			return nil, err
		}
		arns = append(arns, out.ClusterArns...)
		if out.NextToken == nil {
			return arns, nil
		}
		nextToken = out.NextToken
	}
}

func listAllServiceArns(ctx context.Context, client *awsecs.Client, clusterArn string) ([]string, error) {
	var arns []string
	var nextToken *string
	for {
		out, err := client.ListServices(ctx, &awsecs.ListServicesInput{Cluster: &clusterArn, NextToken: nextToken})
		if err != nil {
			return nil, err
		}
		arns = append(arns, out.ServiceArns...)
		if out.NextToken == nil {
			return arns, nil
		}
		nextToken = out.NextToken
	}
}

func dumpServices(ctx context.Context, client *awsecs.Client, clusterArn string, serviceArns []string, out *strings.Builder) error {
	servicesDesc, err := client.DescribeServices(ctx, &awsecs.DescribeServicesInput{
		Cluster:  &clusterArn,
		Services: serviceArns,
	})
	if err != nil {
		return err
	}

	for _, failure := range servicesDesc.Failures {
		fmt.Fprintf(out, "describe-services failure: arn=%s reason=%s detail=%s\n", deref(failure.Arn), deref(failure.Reason), deref(failure.Detail))
	}

	for _, svc := range servicesDesc.Services {
		fmt.Fprintf(out, "=== service %s (cluster %s) ===\n", deref(svc.ServiceName), clusterArn)
		fmt.Fprintf(out, "status=%s desiredCount=%d runningCount=%d pendingCount=%d\n",
			deref(svc.Status), svc.DesiredCount, svc.RunningCount, svc.PendingCount)

		fmt.Fprintln(out, "recent events:")
		for i, ev := range svc.Events {
			if i >= 10 {
				break
			}
			createdAt := "unknown"
			if ev.CreatedAt != nil {
				createdAt = ev.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
			}
			fmt.Fprintf(out, "  [%s] %s\n", createdAt, deref(ev.Message))
		}

		if err := dumpServiceTasks(ctx, client, clusterArn, deref(svc.ServiceName), out); err != nil {
			fmt.Fprintf(out, "  failed to dump tasks for service %s: %v\n", deref(svc.ServiceName), err)
		}
	}

	return nil
}

func dumpServiceTasks(ctx context.Context, client *awsecs.Client, clusterArn, serviceName string, out *strings.Builder) error {
	for _, desiredStatus := range []awsecstypes.DesiredStatus{awsecstypes.DesiredStatusRunning, awsecstypes.DesiredStatusStopped} {
		tasksOut, err := client.ListTasks(ctx, &awsecs.ListTasksInput{
			Cluster:       &clusterArn,
			ServiceName:   &serviceName,
			DesiredStatus: desiredStatus,
		})
		if err != nil {
			return err
		}
		if len(tasksOut.TaskArns) == 0 {
			continue
		}

		tasksDesc, err := client.DescribeTasks(ctx, &awsecs.DescribeTasksInput{
			Cluster: &clusterArn,
			Tasks:   tasksOut.TaskArns,
		})
		if err != nil {
			return err
		}

		for _, task := range tasksDesc.Tasks {
			fmt.Fprintf(out, "  task %s: lastStatus=%s desiredStatus=%s stopCode=%s stoppedReason=%s\n",
				deref(task.TaskArn), deref(task.LastStatus), deref(task.DesiredStatus), task.StopCode, deref(task.StoppedReason))

			for _, c := range task.Containers {
				exitCode := "n/a"
				if c.ExitCode != nil {
					exitCode = fmt.Sprintf("%d", *c.ExitCode)
				}
				fmt.Fprintf(out, "    container %s: lastStatus=%s reason=%s exitCode=%s\n",
					deref(c.Name), deref(c.LastStatus), deref(c.Reason), exitCode)
			}

			for _, att := range task.Attachments {
				fmt.Fprintf(out, "    attachment type=%s status=%s\n", deref(att.Type), deref(att.Status))
				for _, d := range att.Details {
					fmt.Fprintf(out, "      %s=%s\n", deref(d.Name), deref(d.Value))
				}
			}
		}
	}
	return nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
