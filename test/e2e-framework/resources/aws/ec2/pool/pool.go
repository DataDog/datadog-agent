// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package pool implements the tag-based instance discovery described in
// MACOS_EC2_POOL_PROPOSAL.md's "Proposed architecture" section: a pool instance
// is provisioned and published (via the S3 lease object) by an external
// service/job, outside this package's responsibility. This package only
// discovers already-published, idle members by tag and attaches to one (via
// Pulumi import, or a direct SSH import for the non-Pulumi path) — it never
// creates instances itself.
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

// imageAvailableWaitTimeout bounds how long EnsureOwnedBaseline waits for a freshly
// captured AMI to become available.
const imageAvailableWaitTimeout = 20 * time.Minute

const (
	leaseBucket = "datadog-agent-sandbox"
	leasePrefix = "macos-e2e-pool-leases/"

	maxAcquireRetries    = 10
	acquireRetryInterval = 1 * time.Minute
)

// PoolTagKey/PoolTagValue identify every macOS instance managed by the pool, shared by
// every macOS VM request so they all draw from (and grow) the same pool. BaselineSourceTag
// is set on the owned baseline AMI (and its backing snapshot, see EnsureOwnedBaseline) to
// record which pool instance it was captured from.
const (
	PoolTagKey        = "dd:macos-e2e-pool-instance"
	PoolTagValue      = "true"
	BaselineSourceTag = "dd:macos-e2e-pool-source-instance-id"
)

// leaseRecord is the JSON body stored at leasePrefix+instanceID in leaseBucket,
// mutated exclusively via S3 conditional writes (If-Match/If-None-Match) so
// concurrent callers never both believe they've claimed the same instance.
type leaseRecord struct {
	Status   string `json:"status"` // "idle" or "in-use"
	Owner    string `json:"owner,omitempty"`
	LeasedAt int64  `json:"leased_at,omitempty"`
}

// FindInstanceByTag looks for a running or stopped EC2 instance carrying
// tagKey=tagValue, returning its instance ID and true if one exists. It
// returns found=false (no error) if no matching instance exists yet, which
// callers should treat as "create one and tag it," not as a failure.
func FindInstanceByTag(ctx context.Context, client *awsec2.Client, tagKey, tagValue string) (instanceID string, found bool, err error) {
	out, err := client.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{
		Filters: []awsec2types.Filter{
			{
				Name:   pointer.Ptr("tag:" + tagKey),
				Values: []string{tagValue},
			},
			{
				Name:   pointer.Ptr("instance-state-name"),
				Values: []string{"running", "stopped"},
			},
		},
	})
	if err != nil {
		return "", false, fmt.Errorf("failed to describe instances tagged %s=%s: %w", tagKey, tagValue, err)
	}

	for _, reservation := range out.Reservations {
		for _, instance := range reservation.Instances {
			return *instance.InstanceId, true, nil
		}
	}
	return "", false, nil
}

// PoolInstance is one EC2 instance discovered by ListPoolInstances, carrying the
// Dedicated Host it currently sits on so a caller reusing it can pin InstanceArgs.HostID
// to that same host instead of allocating a new one.
type PoolInstance struct {
	InstanceID string
	HostID     string
}

// ListPoolInstances returns every running or stopped EC2 instance carrying
// tagKey=tagValue, unlike FindInstanceByTag which returns only the first match.
func ListPoolInstances(ctx context.Context, client *awsec2.Client, tagKey, tagValue string) ([]PoolInstance, error) {
	out, err := client.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{
		Filters: []awsec2types.Filter{
			{
				Name:   pointer.Ptr("tag:" + tagKey),
				Values: []string{tagValue},
			},
			{
				Name:   pointer.Ptr("instance-state-name"),
				Values: []string{"running", "stopped"},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list instances tagged %s=%s: %w", tagKey, tagValue, err)
	}

	var instances []PoolInstance
	for _, reservation := range out.Reservations {
		for _, instance := range reservation.Instances {
			pi := PoolInstance{InstanceID: *instance.InstanceId}
			if instance.Placement != nil && instance.Placement.HostId != nil {
				pi.HostID = *instance.Placement.HostId
			}
			instances = append(instances, pi)
		}
	}
	return instances, nil
}

// EnsureOwnedBaseline finds an AMI this account owns (as opposed to the instance's
// original launch AMI) previously captured from instanceID and tagged with
// sourceTag=instanceID, reusing it if found. Otherwise it captures a fresh one from
// instanceID's current root volume and waits for it to become available.
//
// Capturing directly from the instance's current state, with no cleanup step
// beforehand, is safe: whatever this returns becomes the revert target for every
// future cycle, including the very next revert the caller performs, so there is
// nothing to pre-clean.
func EnsureOwnedBaseline(ctx context.Context, client *awsec2.Client, instanceID, sourceTag string) (string, error) {
	describeOut, err := client.DescribeImages(ctx, &awsec2.DescribeImagesInput{
		Owners: []string{"self"},
		Filters: []awsec2types.Filter{
			{
				Name:   pointer.Ptr("tag:" + sourceTag),
				Values: []string{instanceID},
			},
			{
				Name:   pointer.Ptr("state"),
				Values: []string{string(awsec2types.ImageStateAvailable)},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to describe owned baseline images for instance %s: %w", instanceID, err)
	}
	if len(describeOut.Images) > 0 {
		return *describeOut.Images[0].ImageId, nil
	}

	createOut, err := client.CreateImage(ctx, &awsec2.CreateImageInput{
		InstanceId: &instanceID,
		Name:       pointer.Ptr("pool-baseline-" + instanceID),
		NoReboot:   pointer.Ptr(true),
		TagSpecifications: []awsec2types.TagSpecification{
			{
				ResourceType: awsec2types.ResourceTypeImage,
				Tags:         []awsec2types.Tag{{Key: pointer.Ptr(sourceTag), Value: &instanceID}},
			},
			{
				ResourceType: awsec2types.ResourceTypeSnapshot,
				Tags:         []awsec2types.Tag{{Key: pointer.Ptr(sourceTag), Value: &instanceID}},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to create owned baseline image for instance %s: %w", instanceID, err)
	}
	imageID := *createOut.ImageId

	waiter := awsec2.NewImageAvailableWaiter(client)
	if err := waiter.Wait(ctx, &awsec2.DescribeImagesInput{ImageIds: []string{imageID}}, imageAvailableWaitTimeout); err != nil {
		return "", fmt.Errorf("owned baseline image %s did not become available within %s: %w", imageID, imageAvailableWaitTimeout, err)
	}
	return imageID, nil
}

// AcquireIdleInstance attempts to claim one idle instance from pool via a
// conditional S3 write (If-Match on the lease object's current ETag),
// returning the instance ID and a lease token (its new ETag) on success. A
// pool instance with no lease object yet is treated as not-yet-published by
// the external provisioning job (not as a fresh instance to claim) and is
// skipped. It retries the whole-pool scan up to maxAcquireRetries times,
// acquireRetryInterval apart, since any instance could become idle or get
// published between attempts. It does not reclaim leases stranded by a
// non-graceful failure (deferred: time-based stale-lease reclaim, see
// MACOS_EC2_POOL_PROPOSAL.md).
func AcquireIdleInstance(ctx context.Context, region, profile string, pool []string, ownerPipelineID string) (instanceID string, leaseToken string, err error) {
	client, err := newS3Client(ctx, region, profile)
	if err != nil {
		return "", "", err
	}

	for attempt := 0; attempt < maxAcquireRetries; attempt++ {
		now := time.Now()

		for _, id := range pool {
			key := leasePrefix + id

			getOut, getErr := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(leaseBucket), Key: aws.String(key)})
			if getErr != nil {
				// No lease object yet: the external provisioning job hasn't published
				// this instance as available yet. Not claimable.
				continue
			}
			var current leaseRecord
			decodeErr := json.NewDecoder(getOut.Body).Decode(&current)
			getOut.Body.Close()
			if decodeErr != nil {
				continue
			}
			if current.Status == "in-use" {
				continue // held by someone else; try the next pool instance
			}

			body, err := json.Marshal(leaseRecord{Status: "in-use", Owner: ownerPipelineID, LeasedAt: now.Unix()})
			if err != nil {
				return "", "", fmt.Errorf("failed to marshal lease record for instance %s: %w", id, err)
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
			return id, aws.ToString(putOut.ETag), nil
		}

		if attempt < maxAcquireRetries-1 {
			select {
			case <-time.After(acquireRetryInterval):
			case <-ctx.Done():
				return "", "", ctx.Err()
			}
		}
	}
	return "", "", fmt.Errorf("no idle instance available in pool of %d", len(pool))
}

// ReleaseInstance marks instanceID idle again, conditioned on leaseToken still
// matching the lease object's current ETag. Callers must revert the instance's
// root volume before calling this.
func ReleaseInstance(ctx context.Context, region, profile string, instanceID string, leaseToken string) error {
	client, err := newS3Client(ctx, region, profile)
	if err != nil {
		return err
	}

	body, err := json.Marshal(leaseRecord{Status: "idle"})
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

// AcquireResult is what Acquire returns for a successfully claimed pool member: enough
// to import the existing instance (InstanceID/HostID) and later release it (LeaseToken).
type AcquireResult struct {
	InstanceID string
	HostID     string
	LeaseToken string
}

// Acquire lists every instance tagged PoolTagKey=PoolTagValue and attempts to claim one
// idle, already-published member via AcquireIdleInstance. Instance creation and initial
// publication (the S3 lease object) are owned by an external service/job, not by this
// package, so an empty or fully-unavailable pool is an error, not a signal to create one.
func Acquire(ctx context.Context, region, profile string, client *awsec2.Client, ownerPipelineID string) (AcquireResult, error) {
	instances, err := ListPoolInstances(ctx, client, PoolTagKey, PoolTagValue)
	if err != nil {
		return AcquireResult{}, err
	}
	if len(instances) == 0 {
		return AcquireResult{}, fmt.Errorf("no macOS pool instances found (tag %s=%s)", PoolTagKey, PoolTagValue)
	}

	byID := make(map[string]PoolInstance, len(instances))
	ids := make([]string, 0, len(instances))
	for _, pi := range instances {
		byID[pi.InstanceID] = pi
		ids = append(ids, pi.InstanceID)
	}

	instanceID, leaseToken, err := AcquireIdleInstance(ctx, region, profile, ids, ownerPipelineID)
	if err != nil {
		return AcquireResult{}, err
	}
	return AcquireResult{
		InstanceID: instanceID,
		HostID:     byID[instanceID].HostID,
		LeaseToken: leaseToken,
	}, nil
}

// NewEC2Client builds an EC2 API client scoped to e's region/profile, for callers
// (outside this package) that need to list or tag pool instances themselves.
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

// BuildReleaseScript returns a shell script that restores instanceID to its owned
// baseline image via boot-disk (root volume) replacement and, once the instance is
// reachable again, marks it idle in the S3 lease store by writing directly to
// leasePrefix+instanceID (matching ReleaseInstance's semantics), conditioned on
// leaseToken so a stale/duplicate release never clobbers a newer claim.
//
// This is a shell script, not a Go function, because it must run as a Pulumi
// local.Command's Delete handler: `pulumi destroy` never re-invokes the Go
// provisioner program, so any cleanup-on-release logic needs to live in each
// resource's own provider-level delete action (see root_volume.go's
// ReplaceRootVolumeToLaunchState for the same constraint applied to boot-disk
// replacement itself).
func BuildReleaseScript(instanceID, leaseToken string) string {
	return fmt.Sprintf(`set -e
INSTANCE_ID=%q
BASELINE_SOURCE_TAG=%q
LEASE_TOKEN=%q
LEASE_BUCKET=%q
LEASE_KEY=%q

IMAGE_ID=$(aws ec2 describe-images --owners self \
  --filters "Name=tag:${BASELINE_SOURCE_TAG},Values=${INSTANCE_ID}" "Name=state,Values=available" \
  --query 'Images[0].ImageId' --output text)

if [ -z "$IMAGE_ID" ] || [ "$IMAGE_ID" = "None" ]; then
  echo "no owned baseline image found for instance ${INSTANCE_ID}, skipping root volume replacement" >&2
else
  SNAPSHOT_ID=$(aws ec2 describe-images --image-ids "$IMAGE_ID" \
    --query 'Images[0].BlockDeviceMappings[0].Ebs.SnapshotId' --output text)

  TASK_ID=$(aws ec2 create-replace-root-volume-task \
    --instance-id "$INSTANCE_ID" --snapshot-id "$SNAPSHOT_ID" \
    --query 'ReplaceRootVolumeTask.ReplaceRootVolumeTaskId' --output text)

  for i in $(seq 1 60); do
    STATE=$(aws ec2 describe-replace-root-volume-tasks --replace-root-volume-task-ids "$TASK_ID" \
      --query 'ReplaceRootVolumeTasks[0].TaskState' --output text)
    case "$STATE" in
      succeeded) break ;;
      failed|failing) echo "replace-root-volume-task ${TASK_ID} ended in state ${STATE}" >&2; exit 1 ;;
      *) sleep 10 ;;
    esac
  done
fi

BODY=$(printf '{"status":"idle"}')
aws s3api put-object --bucket "$LEASE_BUCKET" --key "$LEASE_KEY" \
  --body <(printf '%%s' "$BODY") --if-match "$LEASE_TOKEN"
`, instanceID, BaselineSourceTag, leaseToken, leaseBucket, leasePrefix+instanceID)
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
