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

	// Persistent marks a locally-provisioned, developer-owned instance whose
	// root volume must never be reverted by RevertAndRelease: its ImageID is a
	// golden baseline captured once at creation time (see ScheduleRegisterOnCreate),
	// and reverting to it on every teardown would defeat the point of reusing the
	// same instance across local runs.
	Persistent bool `json:"persistent,omitempty"`
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

// AcquireIdleInstance claims one idle instance from pool via a conditional S3 write
// (If-Match on the lease object's current ETag), returning its instance ID, lease
// token (new ETag), and image ID on success. It retries the whole-pool scan up to
// maxAcquireRetries times, acquireRetryInterval apart. It does not reclaim leases
// stranded by a non-graceful failure.
//
// TODO: leaseRecord.LeasedAt is written on acquire but never read back here, so a
// lease stranded by a crashed job (before Destroy/the delete handler runs) stays
// "in-use" forever, permanently shrinking the pool. Add a staleness/TTL check (or an
// owner+age-based override) so such leases can be automatically reclaimed.
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
// If the lease's Persistent field is true (a locally-provisioned, developer-owned
// instance; see leaseRecord.Persistent), the revert is likewise skipped, since its
// ImageID is a golden baseline captured once at creation time, not a snapshot that
// should be reapplied on every teardown. The lease is still released to statusIdle
// so a later local run can reacquire the same instance.
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
// image, like RevertAndRelease, but re-publishes the lease as statusInUse under its
// current owner instead of releasing it. It's used to reset a developer's own
// drifted local instance back to its golden baseline immediately before a test run
// (see parameters.RevertBeforeRun), rather than on teardown.
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
// on leaseToken still matching the lease object's current ETag. owner/persistent must
// be carried forward from the lease record being replaced (rather than defaulted to
// zero values) so a release never silently drops a persistent local instance's
// Owner/Persistent flags.
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

// AcquireResult is a successfully claimed pool member: InstanceID/HostID/SubnetID to
// import it, LeaseToken to release it, and ImageID to revert it to baseline on
// release. Found is false only when local is non-nil and the developer's own
// instance doesn't exist yet, signaling the caller to provision one.
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
//
// When local is nil (CI), an empty or fully-unavailable pool is an error, and the
// retry budget/error behavior are unchanged from before LocalProvisionOptions was
// introduced.
//
// When local is non-nil, the same maxAcquireRetries/acquireRetryInterval budget is
// reused (a developer can run tests in parallel and contend with their own sibling
// runs for the same instance, so shortening the local retry budget would be
// incorrect) but exhausting it returns AcquireResult{Found: false} instead of an
// error, signaling the caller to provision a new instance for this developer.
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

// BuildRegisterScript returns a shell script that bakes instanceID's current disk
// state into a golden AMI, tags the instance as a pool member owned by username,
// and publishes its first lease record to S3 (Persistent: true, so RevertAndRelease
// never reverts it and it's revertable only via RevertInPlace).
//
// This is a shell script, not a Go function, because it must run as a Pulumi
// local.Command's Create handler (see ScheduleRegisterOnCreate): pool.go has no
// *pulumi.Context, and the instance's ID is only known as a pulumi.StringOutput at
// this point in NewVM, so the registration logic has to be driven from within
// Pulumi's own resource graph rather than called directly as a Go function.
//
// CreateImage is called with --no-reboot: this runs against the very instance the
// current test run is about to use, so rebooting it here to guarantee a
// crash-consistent image (the default CreateImage behavior) would disrupt the
// live SSH connection InitHost just established. The current-run risk of a
// slightly inconsistent baseline is accepted in exchange for not breaking the run
// that's capturing it.
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
