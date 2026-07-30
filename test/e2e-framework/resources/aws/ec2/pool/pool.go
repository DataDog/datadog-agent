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
// locally-provisioned instance (see LocalProvisionOptions), so developers sharing an
// account never claim each other's. Its value is the owner's OS username.
const OwnerUsernameTagKey = "dd:macos-e2e-pool-owner"

// Lease statuses stored in leaseRecord.Status: an instance is either free to claim
// or held by a run.
const (
	statusIdle  = "idle"
	statusInUse = "in-use"
)

// leaseRecord is the JSON body stored at leasePrefix+instanceID in leaseBucket,
// mutated via S3 conditional writes (If-Match/If-None-Match). ImageID identifies the
// baseline AMI RevertAndRelease reverts the instance to on release.
type leaseRecord struct {
	Status   string `json:"status"` // one of statusIdle, statusInUse
	ImageID  string `json:"imageId,omitempty"`
	Owner    string `json:"owner,omitempty"`
	LeasedAt int64  `json:"leased_at,omitempty"`

	// Persistent marks a lease whose root volume must never be reverted by
	// RevertAndRelease, whoever holds it. Local provisioning sets it; absent means
	// false, i.e. revert.
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
				// A missing lease object means "not a pool member yet", which is not an
				// error. Anything else -- credentials, permissions, network -- must not
				// masquerade as an unavailable member.
				if !isNotFound(getErr) {
					return "", "", "", fmt.Errorf("failed to read lease record for instance %s: %w", id, getErr)
				}
				continue
			}
			var current leaseRecord
			decodeErr := json.NewDecoder(getOut.Body).Decode(&current)
			getOut.Body.Close()
			if decodeErr != nil {
				// A lease that exists but will not parse is corruption, not contention.
				return "", "", "", fmt.Errorf("failed to decode lease record for instance %s: %w", id, decodeErr)
			}
			if current.Status != statusIdle {
				continue // held by another run; try the next pool instance
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
				// Losing the conditional write means someone claimed it between our
				// GetObject and PutObject -- try the next member. Anything else is real.
				if !isConditionalWriteConflict(putErr) {
					return "", "", "", fmt.Errorf("failed to claim lease for instance %s: %w", id, putErr)
				}
				continue
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

	imageID := current.ImageID
	if imageID != "" && !current.Persistent && !devMode {
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
// image, like RevertAndRelease, but re-publishes the lease as statusInUse instead of
// releasing it. Returns the new lease token, which the caller must keep to release.
func RevertInPlace(ctx context.Context, region, profile, instanceID, leaseToken string) (string, error) {
	s3Client, err := newS3Client(ctx, region, profile)
	if err != nil {
		return "", err
	}

	key := leasePrefix + instanceID
	getOut, err := s3Client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(leaseBucket), Key: aws.String(key)})
	if err != nil {
		return "", fmt.Errorf("failed to read lease record for instance %s: %w", instanceID, err)
	}
	var current leaseRecord
	decodeErr := json.NewDecoder(getOut.Body).Decode(&current)
	getOut.Body.Close()
	if decodeErr != nil {
		return "", fmt.Errorf("failed to decode lease record for instance %s: %w", instanceID, decodeErr)
	}

	if current.ImageID == "" {
		return "", fmt.Errorf("instance %s has no baseline image to revert to", instanceID)
	}

	ec2Client, err := NewEC2Client(ctx, region, profile)
	if err != nil {
		return "", err
	}
	if revertErr := revertRootVolume(ctx, ec2Client, instanceID, current.ImageID); revertErr != nil {
		return "", fmt.Errorf("failed to revert root volume for instance %s: %w", instanceID, revertErr)
	}

	body, err := json.Marshal(leaseRecord{Status: statusInUse, ImageID: current.ImageID, Owner: current.Owner, LeasedAt: time.Now().Unix(), Persistent: current.Persistent})
	if err != nil {
		return "", fmt.Errorf("failed to marshal lease record for instance %s: %w", instanceID, err)
	}
	putOut, err := s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:  aws.String(leaseBucket),
		Key:     aws.String(key),
		Body:    bytes.NewReader(body),
		IfMatch: aws.String(leaseToken),
	})
	if err != nil {
		return "", fmt.Errorf("failed to re-publish lease for instance %s: %w", instanceID, err)
	}
	return aws.ToString(putOut.ETag), nil
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

// ErrLeaseAlreadyExists reports that PublishInitialLease lost its conditional create,
// i.e. the instance is already registered. Callers recover with CurrentLeaseToken.
var ErrLeaseAlreadyExists = errors.New("lease record already exists")

// apiError structurally matches smithy.APIError, so the S3 error code can be inspected
// without promoting smithy-go from an indirect dependency.
type apiError interface {
	ErrorCode() string
}

// isNotFound reports whether err is S3's "object does not exist". GetObject answers
// NoSuchKey; HeadObject answers NotFound.
func isNotFound(err error) bool {
	var apiErr apiError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.ErrorCode() {
	case "NoSuchKey", "NotFound":
		return true
	}
	return false
}

// isConditionalWriteConflict reports whether err is a lost S3 conditional write: 412
// PreconditionFailed for If-Match/If-None-Match, or 409 ConditionalRequestConflict for a
// concurrent write to the same key.
func isConditionalWriteConflict(err error) bool {
	var apiErr apiError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.ErrorCode() {
	case "PreconditionFailed", "ConditionalRequestConflict":
		return true
	}
	return false
}

// PublishInitialLease writes instanceID's first lease record, held by owner and marked
// Persistent, and returns the lease token. Fails ErrLeaseAlreadyExists if a lease is
// already present.
func PublishInitialLease(ctx context.Context, region, profile, instanceID, imageID, owner string) (string, error) {
	client, err := newS3Client(ctx, region, profile)
	if err != nil {
		return "", err
	}

	body, err := json.Marshal(leaseRecord{
		Status:     statusInUse,
		ImageID:    imageID,
		Owner:      owner,
		LeasedAt:   time.Now().Unix(),
		Persistent: true,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal initial lease record for instance %s: %w", instanceID, err)
	}

	putOut, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(leaseBucket),
		Key:         aws.String(leasePrefix + instanceID),
		Body:        bytes.NewReader(body),
		IfNoneMatch: aws.String("*"),
	})
	if err != nil {
		// S3 answers a lost If-None-Match with 412 PreconditionFailed. A 409
		// ConditionalRequestConflict means a concurrent write and is not the same
		// thing, so it propagates as a plain error.
		var apiErr apiError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "PreconditionFailed" {
			return "", fmt.Errorf("%w for instance %s", ErrLeaseAlreadyExists, instanceID)
		}
		return "", fmt.Errorf("failed to publish initial lease record for instance %s: %w", instanceID, err)
	}
	return aws.ToString(putOut.ETag), nil
}

// CurrentLeaseToken returns instanceID's current lease ETag, letting a caller that hit
// ErrLeaseAlreadyExists adopt the existing lease instead of failing.
func CurrentLeaseToken(ctx context.Context, region, profile, instanceID string) (string, error) {
	client, err := newS3Client(ctx, region, profile)
	if err != nil {
		return "", err
	}
	headOut, err := client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(leaseBucket),
		Key:    aws.String(leasePrefix + instanceID),
	})
	if err != nil {
		return "", fmt.Errorf("failed to read lease token for instance %s: %w", instanceID, err)
	}
	return aws.ToString(headOut.ETag), nil
}

// AcquireResult is a successfully claimed pool member. Found is false only when
// local is non-nil and the developer owns no pool instance yet, signaling the caller
// to provision one; every other failure is reported as an error instead.
type AcquireResult struct {
	Found      bool
	InstanceID string
	HostID     string
	SubnetID   string
	LeaseToken string
	ImageID    string
}

// LocalProvisionOptions scopes Acquire to a single developer's own
// locally-provisioned instance, and turns an empty pool from an error into
// AcquireResult{Found: false} so the caller can provision one instead.
type LocalProvisionOptions struct {
	// Username identifies the developer's own instance via OwnerUsernameTagKey,
	// e.g. aws.Environment.Username(). Must be non-empty.
	Username string
}

// Acquire lists every instance tagged PoolTagKey=PoolTagValue (additionally scoped
// to OwnerUsernameTagKey=local.Username when local is non-nil) and claims one idle
// member. An empty pool yields Found: false for a local run; all else is an error.
func Acquire(ctx context.Context, region, profile string, client *awsec2.Client, ownerPipelineID string, local *LocalProvisionOptions) (AcquireResult, error) {
	tags := map[string]string{PoolTagKey: PoolTagValue}
	if local != nil {
		// An empty owner filter matches no instance, which would look like an empty
		// pool and silently provision a redundant Dedicated Host. Fail instead.
		if local.Username == "" {
			return AcquireResult{}, errors.New("local pool provisioning requires a non-empty username")
		}
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
