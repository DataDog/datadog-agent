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
)

// PoolTagKey/PoolTagValue identify every macOS instance managed by the pool, shared by
// every macOS VM request so they all draw from (and grow) the same pool.
const (
	PoolTagKey   = "dd:macos-e2e-pool-instance"
	PoolTagValue = "true"
)

// leaseRecord is the JSON body stored at leasePrefix+instanceID in leaseBucket,
// mutated via S3 conditional writes (If-Match/If-None-Match). ImageID identifies the
// baseline AMI BuildReleaseScript reverts the instance to on release.
type leaseRecord struct {
	Status   string `json:"status"` // "idle" or "in-use"
	ImageID  string `json:"imageId,omitempty"`
	Owner    string `json:"owner,omitempty"`
	LeasedAt int64  `json:"leased_at,omitempty"`
}

// PoolInstance is one EC2 instance discovered by ListPoolInstances, with the
// Dedicated Host it currently sits on.
type PoolInstance struct {
	InstanceID string
	HostID     string
}

// ListPoolInstances returns every running or stopped EC2 instance carrying
// tagKey=tagValue.
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
			if current.Status == "in-use" {
				continue // held by someone else; try the next pool instance
			}

			body, err := json.Marshal(leaseRecord{Status: "in-use", ImageID: current.ImageID, Owner: ownerPipelineID, LeasedAt: now.Unix()})
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

// ReleaseInstance marks instanceID idle again, conditioned on leaseToken still
// matching the lease object's current ETag, and preserves the record's ImageID.
func ReleaseInstance(ctx context.Context, region, profile string, instanceID string, leaseToken string) error {
	client, err := newS3Client(ctx, region, profile)
	if err != nil {
		return err
	}

	key := leasePrefix + instanceID
	var imageID string
	if getOut, getErr := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(leaseBucket), Key: aws.String(key)}); getErr == nil {
		var current leaseRecord
		if json.NewDecoder(getOut.Body).Decode(&current) == nil {
			imageID = current.ImageID
		}
		getOut.Body.Close()
	}

	body, err := json.Marshal(leaseRecord{Status: "idle", ImageID: imageID})
	if err != nil {
		return fmt.Errorf("failed to marshal lease record for instance %s: %w", instanceID, err)
	}
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:  aws.String(leaseBucket),
		Key:     aws.String(key),
		Body:    bytes.NewReader(body),
		IfMatch: aws.String(leaseToken),
	})
	if err != nil {
		return fmt.Errorf("failed to release lease for instance %s: %w", instanceID, err)
	}
	return nil
}

// AcquireResult is a successfully claimed pool member: InstanceID/HostID to import it,
// LeaseToken to release it, and ImageID to revert it to baseline on release.
type AcquireResult struct {
	InstanceID string
	HostID     string
	LeaseToken string
	ImageID    string
}

// Acquire lists every instance tagged PoolTagKey=PoolTagValue and claims one idle
// member via AcquireIdleInstance. An empty or fully-unavailable pool is an error.
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

	instanceID, leaseToken, imageID, err := AcquireIdleInstance(ctx, region, profile, ids, ownerPipelineID)
	if err != nil {
		return AcquireResult{}, err
	}
	return AcquireResult{
		InstanceID: instanceID,
		HostID:     byID[instanceID].HostID,
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

// BuildReleaseScript returns a shell script that reverts instanceID's root volume to
// imageID's snapshot, then releases the lease at leasePrefix+instanceID conditioned on
// leaseToken. If imageID is empty, the root-volume replacement is skipped and the
// lease is released directly.
//
// This runs as a Pulumi local.Command's Delete handler, since `pulumi destroy` never
// re-invokes the Go provisioner program.
func BuildReleaseScript(instanceID, leaseToken, imageID string) string {
	return fmt.Sprintf(`set -e
INSTANCE_ID=%q
IMAGE_ID=%q
LEASE_TOKEN=%q
LEASE_BUCKET=%q
LEASE_KEY=%q

if [ -z "$IMAGE_ID" ] || [ "$IMAGE_ID" = "None" ]; then
  echo "no baseline image published for instance ${INSTANCE_ID}, skipping root volume replacement" >&2
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

BODY=$(printf '{"status":"idle","imageId":"%%s"}' "$IMAGE_ID")
aws s3api put-object --bucket "$LEASE_BUCKET" --key "$LEASE_KEY" \
  --body <(printf '%%s' "$BODY") --if-match "$LEASE_TOKEN"
`, instanceID, imageID, leaseToken, leaseBucket, leasePrefix+instanceID)
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
