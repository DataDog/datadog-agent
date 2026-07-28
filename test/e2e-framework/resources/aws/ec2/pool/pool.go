// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package pool discovers idle, tagged macOS EC2 instances and attaches to one via
// an S3-backed lease. For CI runs it never provisions or creates instances itself: an
// empty or fully-claimed pool is a fail-closed error. For local runs (Acquire's local
// parameter) a cache miss instead returns AcquireResult.Found=false so the caller
// (scenarios/aws/ec2/vm.go's NewVM) can provision a brand-new, owner-tagged instance
// itself via Pulumi-managed resources (ec2.NewDedicatedHost, ec2.NewInstance) — this
// package only builds the registration script (BuildRegisterScript) that seeds that
// new instance's lease, since it has no *pulumi.Context of its own to create resources
// with.
package pool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	awsec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const (
	leaseBucket = "datadog-agent-sandbox"
	leasePrefix = "macos-e2e-pool-leases/"

	maxAcquireRetries    = 10
	acquireRetryInterval = 1 * time.Minute

	replaceRootVolumePollInterval = 10 * time.Second
	replaceRootVolumeMaxPolls     = 60
)

// PoolTagKey/PoolTagValue identify every macOS instance managed by the pool, shared by
// every macOS VM request so they all draw from (and grow) the same pool.
const (
	PoolTagKey   = "dd:macos-e2e-pool-instance"
	PoolTagValue = "true"
)

// OwnerUsernameTagKey tags a pool instance with the OS username of the developer whose
// local run provisioned it, matching the "username" key
// CommonEnvironment.DefaultResourceTags() already uses for every other Pulumi-managed
// resource. Only instances provisioned for local runs carry this tag; CI-provisioned
// instances are shared across the whole pool and are matched on PoolTagKey alone.
const OwnerUsernameTagKey = "username"

// localAcquireAttempts bounds how many times Acquire re-scans a local developer's own
// instances before giving up and reporting Found=false so the caller provisions a new
// one. Unlike CI's maxAcquireRetries/acquireRetryInterval, local runs shouldn't block
// for minutes on an instance the same developer is already using elsewhere.
const localAcquireAttempts = 1

// Lease statuses stored in leaseRecord.Status. statusDevMode marks an instance
// released from a dev-mode test run: like statusInUse it is unclaimable, but it is
// tracked separately since its root volume was deliberately left unreverted for
// inspection.
const (
	statusIdle    = "idle"
	statusInUse   = "in-use"
	statusDevMode = "dev-mode"
)

// leaseRecord is the JSON body stored at leasePrefix+instanceID in leaseBucket,
// mutated via S3 conditional writes (If-Match/If-None-Match). ImageID identifies the
// baseline AMI RevertAndRelease reverts the instance to on release.
type leaseRecord struct {
	Status   string `json:"status"` // one of statusIdle, statusInUse, statusDevMode
	ImageID  string `json:"imageId,omitempty"`
	Owner    string `json:"owner,omitempty"`
	LeasedAt int64  `json:"leased_at,omitempty"`
}

// PoolInstance is one EC2 instance discovered by ListPoolInstances, with the
// Dedicated Host and subnet it currently sits on. SubnetId must be preserved on
// import: the instance's AZ is fixed by its Dedicated Host, so a subnet picked in a
// different AZ makes the EC2 instance resource non-importable/replace-triggering.
type PoolInstance struct {
	InstanceID string
	HostID     string
	SubnetID   string
}

// ListPoolInstances returns every running or stopped EC2 instance carrying every tag
// in tags (e.g. PoolTagKey=PoolTagValue, plus OwnerUsernameTagKey=<username> when
// scoping to a single developer's own instances for local runs).
func ListPoolInstances(ctx context.Context, client *awsec2.Client, tags map[string]string) ([]PoolInstance, error) {
	filters := make([]awsec2types.Filter, 0, len(tags)+1)
	for tagKey, tagValue := range tags {
		filters = append(filters, awsec2types.Filter{
			Name:   pointer.Ptr("tag:" + tagKey),
			Values: []string{tagValue},
		})
	}
	filters = append(filters, awsec2types.Filter{
		Name:   pointer.Ptr("instance-state-name"),
		Values: []string{"running", "stopped"},
	})

	out, err := client.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{Filters: filters})
	if err != nil {
		return nil, fmt.Errorf("failed to list instances tagged %v: %w", tags, err)
	}

	var instances []PoolInstance
	for _, reservation := range out.Reservations {
		for _, instance := range reservation.Instances {
			pi := PoolInstance{InstanceID: *instance.InstanceId}
			if instance.Placement != nil && instance.Placement.HostId != nil {
				pi.HostID = *instance.Placement.HostId
			}
			if instance.SubnetId != nil {
				pi.SubnetID = *instance.SubnetId
			}
			instances = append(instances, pi)
		}
	}
	return instances, nil
}

// AcquireIdleInstance claims one idle instance from pool via a conditional S3 write
// (If-Match on the lease object's current ETag), returning its instance ID, lease
// token (new ETag), and image ID on success. It retries the whole-pool scan up to
// maxRetries times, acquireRetryInterval apart. It does not reclaim leases
// stranded by a non-graceful failure.
//
// TODO: leaseRecord.LeasedAt is written on acquire but never read back here, so a
// lease stranded by a crashed job (before Destroy/the delete handler runs) stays
// "in-use" forever, permanently shrinking the pool. Add a staleness/TTL check (or an
// owner+age-based override) so such leases can be automatically reclaimed.
func AcquireIdleInstance(ctx context.Context, region, profile string, pool []string, ownerPipelineID string, maxRetries int) (instanceID string, leaseToken string, imageID string, err error) {
	client, err := newS3Client(ctx, region, profile)
	if err != nil {
		return "", "", "", err
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		now := time.Now()

		for _, id := range pool {
			key := leasePrefix + id

			getOut, getErr := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(leaseBucket), Key: aws.String(key)})
			if getErr != nil {
				// No lease object yet: not claimable.
				continue
			}
			var current leaseRecord
			decodeErr := json.NewDecoder(getOut.Body).Decode(&current)
			getOut.Body.Close()
			if decodeErr != nil {
				continue
			}
			if current.Status != statusIdle {
				continue // held by someone else, or left in dev-mode; try the next pool instance
			}

			body, err := json.Marshal(leaseRecord{Status: statusInUse, ImageID: current.ImageID, Owner: ownerPipelineID, LeasedAt: now.Unix()})
			if err != nil {
				return "", "", "", fmt.Errorf("failed to marshal lease record for instance %s: %w", id, err)
			}
			putOut, putErr := client.PutObject(ctx, &s3.PutObjectInput{
				Bucket:  aws.String(leaseBucket),
				Key:     aws.String(key),
				Body:    bytes.NewReader(body),
				IfMatch: getOut.ETag,
			})
			if putErr != nil {
				continue // precondition failed: someone else claimed it between our GetObject and PutObject
			}
			return id, aws.ToString(putOut.ETag), current.ImageID, nil
		}

		if attempt < maxRetries-1 {
			select {
			case <-time.After(acquireRetryInterval):
			case <-ctx.Done():
				return "", "", "", ctx.Err()
			}
		}
	}
	return "", "", "", fmt.Errorf("no idle instance available in pool of %d", len(pool))
}

// RevertAndRelease reverts instanceID's root volume to the lease's current baseline
// image (read fresh from S3, not the value cached at acquire time, so a baseline
// rotated mid-checkout is still honored) and releases the lease, conditioned on
// leaseToken still matching the lease object's current ETag.
//
// If devMode is true, the revert is skipped (the point of dev mode is to leave the
// instance's state alone for inspection) and the lease is marked statusDevMode
// instead of statusIdle, so the instance remains unclaimable by other jobs while
// staying visibly distinguishable from a normal in-use lease.
//
// If the lease's current imageId is empty, or no longer resolves to an existing
// AMI/snapshot, the root-volume revert is skipped, imageId is cleared before it's
// written back to the lease, and the lease is released directly.
func RevertAndRelease(ctx context.Context, region, profile, instanceID, leaseToken string, devMode bool) error {
	s3Client, err := newS3Client(ctx, region, profile)
	if err != nil {
		return err
	}

	key := leasePrefix + instanceID
	getOut, err := s3Client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(leaseBucket), Key: aws.String(key)})
	if err != nil {
		return fmt.Errorf("failed to read lease record for instance %s: %w", instanceID, err)
	}
	var current leaseRecord
	decodeErr := json.NewDecoder(getOut.Body).Decode(&current)
	getOut.Body.Close()
	if decodeErr != nil {
		return fmt.Errorf("failed to decode lease record for instance %s: %w", instanceID, decodeErr)
	}

	if devMode {
		return releaseLease(ctx, s3Client, instanceID, leaseToken, statusDevMode, current.ImageID)
	}

	imageID := current.ImageID
	if imageID != "" {
		ec2Client, err := NewEC2Client(ctx, region, profile)
		if err != nil {
			return err
		}

		snapshotID, resolveErr := resolveSnapshotID(ctx, ec2Client, imageID)
		if resolveErr != nil {
			imageID = "" // baseline no longer resolvable: skip revert, clear it, still release
		} else if err := replaceRootVolume(ctx, ec2Client, instanceID, snapshotID); err != nil {
			return fmt.Errorf("failed to revert root volume for instance %s: %w", instanceID, err)
		}
	}

	return releaseLease(ctx, s3Client, instanceID, leaseToken, statusIdle, imageID)
}

// resolveSnapshotID returns the root EBS snapshot ID backing imageID.
func resolveSnapshotID(ctx context.Context, client *awsec2.Client, imageID string) (string, error) {
	out, err := client.DescribeImages(ctx, &awsec2.DescribeImagesInput{ImageIds: []string{imageID}})
	if err != nil {
		return "", fmt.Errorf("failed to describe image %s: %w", imageID, err)
	}
	if len(out.Images) == 0 || len(out.Images[0].BlockDeviceMappings) == 0 || out.Images[0].BlockDeviceMappings[0].Ebs == nil || out.Images[0].BlockDeviceMappings[0].Ebs.SnapshotId == nil {
		return "", fmt.Errorf("image %s has no root snapshot", imageID)
	}
	return *out.Images[0].BlockDeviceMappings[0].Ebs.SnapshotId, nil
}

// replaceRootVolume creates a replace-root-volume task for instanceID from
// snapshotID and blocks until it succeeds, fails, or times out.
func replaceRootVolume(ctx context.Context, client *awsec2.Client, instanceID, snapshotID string) error {
	createOut, err := client.CreateReplaceRootVolumeTask(ctx, &awsec2.CreateReplaceRootVolumeTaskInput{
		InstanceId: aws.String(instanceID),
		SnapshotId: aws.String(snapshotID),
	})
	if err != nil {
		return fmt.Errorf("failed to create replace-root-volume task: %w", err)
	}
	taskID := createOut.ReplaceRootVolumeTask.ReplaceRootVolumeTaskId

	for i := 0; i < replaceRootVolumeMaxPolls; i++ {
		describeOut, err := client.DescribeReplaceRootVolumeTasks(ctx, &awsec2.DescribeReplaceRootVolumeTasksInput{
			ReplaceRootVolumeTaskIds: []string{*taskID},
		})
		if err != nil {
			return fmt.Errorf("failed to poll replace-root-volume task %s: %w", *taskID, err)
		}
		if len(describeOut.ReplaceRootVolumeTasks) == 0 {
			return fmt.Errorf("replace-root-volume task %s disappeared while polling", *taskID)
		}

		switch describeOut.ReplaceRootVolumeTasks[0].TaskState {
		case awsec2types.ReplaceRootVolumeTaskStateSucceeded:
			return nil
		case awsec2types.ReplaceRootVolumeTaskStateFailed, awsec2types.ReplaceRootVolumeTaskStateFailing:
			return fmt.Errorf("replace-root-volume task %s ended in state %s", *taskID, describeOut.ReplaceRootVolumeTasks[0].TaskState)
		}

		select {
		case <-time.After(replaceRootVolumePollInterval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return fmt.Errorf("replace-root-volume task %s did not complete within timeout", *taskID)
}

// releaseLease writes status/imageID back to instanceID's lease record, conditioned
// on leaseToken still matching the lease object's current ETag.
func releaseLease(ctx context.Context, client *s3.Client, instanceID, leaseToken, status, imageID string) error {
	body, err := json.Marshal(leaseRecord{Status: status, ImageID: imageID})
	if err != nil {
		return fmt.Errorf("failed to marshal lease record for instance %s: %w", instanceID, err)
	}
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:  aws.String(leaseBucket),
		Key:     aws.String(leasePrefix + instanceID),
		Body:    bytes.NewReader(body),
		IfMatch: aws.String(leaseToken),
	})
	if err != nil {
		return fmt.Errorf("failed to release lease for instance %s: %w", instanceID, err)
	}
	return nil
}

// AcquireResult is a successfully claimed pool member: InstanceID/HostID/SubnetID to
// import it, LeaseToken to release it, and ImageID to revert it to baseline on
// release. Found is false only when local != nil and no idle instance owned by
// local.Username exists: the caller (NewVM) must then provision a new instance itself
// via Pulumi-managed resources and register it with BuildRegisterScript.
type AcquireResult struct {
	Found      bool
	InstanceID string
	HostID     string
	SubnetID   string
	LeaseToken string
	ImageID    string
}

// LocalProvisionOptions scopes Acquire's discovery to a single local developer's own
// instances (via OwnerUsernameTagKey) instead of the whole shared pool CI draws from.
// Passing a nil *LocalProvisionOptions to Acquire keeps the CI behavior: discovery-only,
// fail-closed on an empty/unavailable pool, matching on PoolTagKey alone.
type LocalProvisionOptions struct {
	// Username should be the same value as
	// CommonEnvironment.DefaultResourceTags()["username"], used both to scope instance
	// selection and to tag a freshly provisioned instance as this developer's own.
	Username string
}

// Acquire lists idle members of the macOS pool and claims one via AcquireIdleInstance.
// For CI runs (local == nil) it matches on PoolTagKey=PoolTagValue alone, and an empty
// or fully-unavailable pool is a fail-closed error, per this package's discovery-only
// CI contract.
//
// For local runs (local != nil) it additionally scopes discovery to instances tagged
// OwnerUsernameTagKey=local.Username, so a developer only ever claims their own
// previously-provisioned instance, never another developer's. If that scoped scan
// finds nothing idle, it returns AcquireResult{Found: false}, nil instead of an
// error: the caller must provision a new instance itself (this package has no
// *pulumi.Context to create Pulumi-managed resources with) and register it via
// BuildRegisterScript.
func Acquire(ctx context.Context, region, profile string, client *awsec2.Client, ownerPipelineID string, local *LocalProvisionOptions) (AcquireResult, error) {
	tagFilters := map[string]string{PoolTagKey: PoolTagValue}
	if local != nil {
		tagFilters[OwnerUsernameTagKey] = local.Username
	}

	instances, err := ListPoolInstances(ctx, client, tagFilters)
	if err != nil {
		return AcquireResult{}, err
	}
	if len(instances) == 0 {
		if local != nil {
			return AcquireResult{Found: false}, nil
		}
		return AcquireResult{}, fmt.Errorf("no macOS pool instances found (tag %s=%s)", PoolTagKey, PoolTagValue)
	}

	byID := make(map[string]PoolInstance, len(instances))
	ids := make([]string, 0, len(instances))
	for _, pi := range instances {
		byID[pi.InstanceID] = pi
		ids = append(ids, pi.InstanceID)
	}

	retries := maxAcquireRetries
	if local != nil {
		retries = localAcquireAttempts
	}
	instanceID, leaseToken, imageID, err := AcquireIdleInstance(ctx, region, profile, ids, ownerPipelineID, retries)
	if err != nil {
		if local != nil {
			return AcquireResult{Found: false}, nil
		}
		return AcquireResult{}, err
	}
	return AcquireResult{
		Found:      true,
		InstanceID: instanceID,
		HostID:     byID[instanceID].HostID,
		SubnetID:   byID[instanceID].SubnetID,
		LeaseToken: leaseToken,
		ImageID:    imageID,
	}, nil
}

// BuildRegisterScript returns a shell script that tags a brand-new instance as a pool
// member (PoolTagKey=PoolTagValue, OwnerUsernameTagKey=username) and publishes its
// lease record as already claimed by ownerPipelineID, printing the lease token (the
// new lease object's ETag) to stdout.
//
// This is a shell script, not a Go function, because it must run as a Pulumi
// local.Command's Create handler so it can be sequenced after (and take as input) the
// freshly created EC2 instance's ID, which is only known as a pulumi.StringOutput
// inside the Pulumi program — see resources/aws/ec2/vm.go's use of it alongside
// ec2.NewInstance for a local run's cache-miss path.
func BuildRegisterScript(instanceID, ownerPipelineID, username, imageID string) string {
	body, _ := json.Marshal(leaseRecord{Status: statusInUse, ImageID: imageID, Owner: ownerPipelineID, LeasedAt: 0})
	return fmt.Sprintf(`set -e
INSTANCE_ID=%q
POOL_TAG_KEY=%q
POOL_TAG_VALUE=%q
OWNER_TAG_KEY=%q
OWNER_TAG_VALUE=%q
LEASE_BUCKET=%q
LEASE_KEY=%q
BODY=%q

aws ec2 create-tags --resources "$INSTANCE_ID" \
  --tags Key="$POOL_TAG_KEY",Value="$POOL_TAG_VALUE" Key="$OWNER_TAG_KEY",Value="$OWNER_TAG_VALUE" Key=Name,Value="macos-e2e-pool-$OWNER_TAG_VALUE"

aws s3api put-object --bucket "$LEASE_BUCKET" --key "$LEASE_KEY" \
  --body <(printf '%%s' "$BODY") --if-none-match "*" \
  --query 'ETag' --output text
`, instanceID, PoolTagKey, PoolTagValue, OwnerUsernameTagKey, username, leaseBucket, leasePrefix+instanceID, string(body))
}

// NewEC2Client builds an EC2 API client scoped to region/profile.
func NewEC2Client(ctx context.Context, region, profile string) (*awsec2.Client, error) {
	cfg, err := awsConfig.LoadDefaultConfig(ctx,
		awsConfig.WithRegion(region),
		awsConfig.WithSharedConfigProfile(profile),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for EC2 pool client: %w", err)
	}
	return awsec2.NewFromConfig(cfg), nil
}

func newS3Client(ctx context.Context, region, profile string) (*s3.Client, error) {
	cfg, err := awsConfig.LoadDefaultConfig(ctx,
		awsConfig.WithRegion(region),
		awsConfig.WithSharedConfigProfile(profile),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for S3 lease client: %w", err)
	}
	return s3.NewFromConfig(cfg), nil
}
