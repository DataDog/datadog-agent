package spanpointers

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/DataDog/datadog-agent/pkg/serverless/trigger/events"
	"strings"
)

const (
	s3PointerKind = "aws.s3.object"
)

// SpanPointer is a struct that stores a hash and span kind to uniquely
// identify a S3 or DynamoDB operation.
type SpanPointer struct {
	Hash string
	Kind string
}

func generateSpanPointerHash(components []string) string {
	dataToHash := strings.Join(components, "|")
	sum := sha256.Sum256([]byte(dataToHash))
	return hex.EncodeToString(sum[:])[:32]
}

func GetSpanPointersFromS3Event(event events.S3Event) []SpanPointer {
	var pointers []SpanPointer
	for _, record := range event.Records {
		bucketName := record.S3.Bucket.Name
		key := record.S3.Object.Key
		eTag := strings.Trim(record.S3.Object.ETag, "\"")

		hash := generateSpanPointerHash([]string{bucketName, key, eTag})

		spanPointer := SpanPointer{
			Hash: hash,
			Kind: s3PointerKind,
		}
		pointers = append(pointers, spanPointer)
	}
	return pointers
}
