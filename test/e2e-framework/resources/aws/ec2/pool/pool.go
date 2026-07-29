// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package pool discovers idle, tagged macOS EC2 instances and attaches to one via
// an S3-backed lease. It never provisions or creates instances itself.
package pool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

	maxAcquireRetries    = 30
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

// OwnerUsernameTagKey additionally scopes discovery to a single developer's own
// locally-provisioned instance (see LocalProvisionOptions), so local runs never
// discover another developer's instance or a CI pool member.
const OwnerUsernameTagKey = "username"

// Lease statuses stored in leaseRecord.Status. statusDevMode marks an instance
// released from a dev-mode run: unclaimable like statusInUse, but tracked
// separately since its root volume is left unreverted for inspection.
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

	// Persistent marks a locally-provisioned, developer-owned instance whose
	// root volume must never be reverted by RevertAndRelease; see
	// ScheduleRegisterOnCreate.
	Persistent bool `json:"persistent,omitempty"`
}

// PoolInstance is one EC2 instance discovered by ListPoolInstances, with the
// Dedicated Host and subnet it currently sits on. SubnetId must be preserved on
// import since the instance's AZ is fixed by its Dedicated Host.
type PoolInstance struct {
	InstanceID string
	HostID     string
	SubnetID   string
}

// ListPoolInstances returns every running or stopped EC2 instance carrying every
// tag in tags.
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

// errPoolExhausted signals that every pool member stayed claimed for the whole
// retry budget. It is deliberately distinct from a failure to reach AWS, which must
// never be read as "the pool has no free member".
var errPoolExhausted = errors.New("no idle instance available")

// AcquireIdleInstance claims one idle instance from pool via a conditional S3 write
// (If-Match on the lease object's current ETag), retrying the whole-pool scan up to
// maxAcquireRetries times, acquireRetryInterval apart, then failing errPoolExhausted.
func AcquireIdleInstance(ctx context.Context, region, profile string, pool []string, ownerPipelineID string) (instanceID string, leaseToken string, imageID string, err error) {
	client, err := newS3Client(ctx, region, profile)
	if err != nil {
		return "", "", "", err
	}

	for attempt := 0; attempt < maxAcquireRetries; attempt++ {
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

			body, err := json.Marshal(leaseRecord{Status: statusInUse, ImageID: current.ImageID, Owner: ownerPipelineID, LeasedAt: now.Unix(), Persistent: current.Persistent})
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

		if attempt < maxAcquireRetries-1 {
			select {
			case <-time.After(acquireRetryInterval):
			case <-ctx.Done():
				return "", "", "", ctx.Err()
			}
		}
	}
	return "", "", "", fmt.Errorf("%w in pool of %d", errPoolExhausted, len(pool))
}

// RevertAndRelease reverts instanceID's root volume to the lease's current baseline
// image and releases the lease, conditioned on leaseToken matching the lease
// object's current ETag. The revert is skipped in dev mode or for a persistent lease.
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
		return releaseLease(ctx, s3Client, instanceID, leaseToken, statusDevMode, current.ImageID, current.Owner, current.Persistent)
	}

	imageID := current.ImageID
	if imageID != "" && !current.Persistent {
		ec2Client, err := NewEC2Client(ctx, region, profile)
		if err != nil {
			return err
		}

		if revertErr := revertRootVolume(ctx, ec2Client, instanceID, imageID); revertErr != nil {
			if errors.Is(revertErr, errImageUnresolvable) {
				imageID = "" // baseline no longer resolvable: skip revert, clear it, still release
			} else {
				return fmt.Errorf("failed to revert root volume for instance %s: %w", instanceID, revertErr)
			}
		}
	}

	return releaseLease(ctx, s3Client, instanceID, leaseToken, statusIdle, imageID, current.Owner, current.Persistent)
}

// RevertInPlace reverts instanceID's root volume to the lease's current baseline
// image, like RevertAndRelease, but re-publishes the lease as statusInUse instead
// of releasing it.
func RevertInPlace(ctx context.Context, region, profile, instanceID, leaseToken string) error {
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

	if current.ImageID == "" {
		return fmt.Errorf("instance %s has no baseline image to revert to", instanceID)
	}

	ec2Client, err := NewEC2Client(ctx, region, profile)
	if err != nil {
		return err
	}
	if revertErr := revertRootVolume(ctx, ec2Client, instanceID, current.ImageID); revertErr != nil {
		return fmt.Errorf("failed to revert root volume for instance %s: %w", instanceID, revertErr)
	}

	body, err := json.Marshal(leaseRecord{Status: statusInUse, ImageID: current.ImageID, Owner: current.Owner, LeasedAt: current.LeasedAt, Persistent: current.Persistent})
	if err != nil {
		return fmt.Errorf("failed to marshal lease record for instance %s: %w", instanceID, err)
	}
	if _, err := s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:  aws.String(leaseBucket),
		Key:     aws.String(key),
		Body:    bytes.NewReader(body),
		IfMatch: aws.String(leaseToken),
	}); err != nil {
		return fmt.Errorf("failed to re-publish lease for instance %s: %w", instanceID, err)
	}
	return nil
}

// errImageUnresolvable marks a revertRootVolume failure caused by imageID no longer
// resolving to an existing AMI/snapshot, as opposed to an actual revert failure.
var errImageUnresolvable = errors.New("baseline image no longer resolvable")

// revertRootVolume resolves imageID to its root EBS snapshot and replaces
// instanceID's root volume with it, blocking until the replacement completes.
func revertRootVolume(ctx context.Context, client *awsec2.Client, instanceID, imageID string) error {
	snapshotID, resolveErr := resolveSnapshotID(ctx, client, imageID)
	if resolveErr != nil {
		return fmt.Errorf("%w: %s: %w", errImageUnresolvable, imageID, resolveErr)
	}
	return replaceRootVolume(ctx, client, instanceID, snapshotID)
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
// on leaseToken matching the lease object's current ETag. owner/persistent are
// carried forward from the current record rather than defaulted to zero values.
func releaseLease(ctx context.Context, client *s3.Client, instanceID, leaseToken, status, imageID, owner string, persistent bool) error {
	body, err := json.Marshal(leaseRecord{Status: status, ImageID: imageID, Owner: owner, Persistent: persistent})
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

// AcquireResult is a successfully claimed pool member. Found is false only when
// local is non-nil and no instance was claimable, signaling the caller to provision
// one; every other failure is reported as an error instead.
type AcquireResult struct {
	Found      bool
	InstanceID string
	HostID     string
	SubnetID   string
	LeaseToken string
	ImageID    string
}

// LocalProvisionOptions scopes Acquire to a single developer's own
// locally-provisioned instance, and turns an empty/fully-claimed pool from an
// error into AcquireResult{Found: false} so the caller can provision one instead.
type LocalProvisionOptions struct {
	// Username identifies the developer's own instance via OwnerUsernameTagKey,
	// e.g. aws.Environment.DefaultResourceTags()["username"].
	Username string
}

// Acquire lists every instance tagged PoolTagKey=PoolTagValue (additionally scoped
// to OwnerUsernameTagKey=local.Username when local is non-nil) and claims one idle
// member via AcquireIdleInstance.
func Acquire(ctx context.Context, region, profile string, client *awsec2.Client, ownerPipelineID string, local *LocalProvisionOptions) (AcquireResult, error) {
	tags := map[string]string{PoolTagKey: PoolTagValue}
	if local != nil {
		tags[OwnerUsernameTagKey] = local.Username
	}

	instances, err := ListPoolInstances(ctx, client, tags)
	if err != nil {
		return AcquireResult{}, err
	}
	if len(instances) == 0 {
		if local != nil {
			return AcquireResult{Found: false}, nil
		}
		return AcquireResult{}, fmt.Errorf("no macOS pool instances found (tags %v)", tags)
	}

	byID := make(map[string]PoolInstance, len(instances))
	ids := make([]string, 0, len(instances))
	for _, pi := range instances {
		byID[pi.InstanceID] = pi
		ids = append(ids, pi.InstanceID)
	}

	instanceID, leaseToken, imageID, err := AcquireIdleInstance(ctx, region, profile, ids, ownerPipelineID)
	if err != nil {
		// Only a genuinely exhausted pool means "provision one". Any other error
		// (AWS credentials, context cancellation) must propagate: reporting
		// Found: false would create a second billable Dedicated Host instead.
		if local != nil && errors.Is(err, errPoolExhausted) {
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

// BuildRegisterScript returns a shell script that bakes instanceID's current disk
// state into a golden AMI, tags the instance as a pool member owned by username,
// and publishes its first lease record to S3 with Persistent: true.
func BuildRegisterScript(instanceID, ownerPipelineID, username string) string {
	return fmt.Sprintf(`set -e
INSTANCE_ID=%q
OWNER=%q
USERNAME=%q
POOL_TAG_KEY=%q
POOL_TAG_VALUE=%q
OWNER_TAG_KEY=%q
LEASE_BUCKET=%q
LEASE_KEY=%q

IMAGE_ID=$(aws ec2 create-image --instance-id "$INSTANCE_ID" \
  --name "macos-e2e-pool-${USERNAME}-${INSTANCE_ID}" --no-reboot \
  --query 'ImageId' --output text)

for i in $(seq 1 60); do
  STATE=$(aws ec2 describe-images --image-ids "$IMAGE_ID" --query 'Images[0].State' --output text)
  case "$STATE" in
    available) break ;;
    failed) echo "image ${IMAGE_ID} failed to bake" >&2; exit 1 ;;
    *) sleep 10 ;;
  esac
done

aws ec2 create-tags --resources "$INSTANCE_ID" --tags \
  Key="$POOL_TAG_KEY",Value="$POOL_TAG_VALUE" \
  Key="$OWNER_TAG_KEY",Value="$USERNAME" \
  Key=Name,Value="macos-e2e-pool-$USERNAME"

BODY=$(printf '{"status":"in-use","imageId":"%%s","owner":"%%s","persistent":true}' "$IMAGE_ID" "$OWNER")
aws s3api put-object --bucket "$LEASE_BUCKET" --key "$LEASE_KEY" \
  --body <(printf '%%s' "$BODY") --if-none-match "*" --query 'ETag' --output text
`, instanceID, ownerPipelineID, username, PoolTagKey, PoolTagValue, OwnerUsernameTagKey, leaseBucket, leasePrefix+instanceID)
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
