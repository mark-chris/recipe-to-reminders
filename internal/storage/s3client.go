package storage

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"
)

// S3Client wraps the AWS SDK v2 S3 client to implement S3API.
type S3Client struct {
	client *s3.Client
}

// NewS3Client creates an S3Client from an AWS SDK v2 S3 client.
func NewS3Client(client *s3.Client) *S3Client {
	return &S3Client{client: client}
}

func (c *S3Client) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, string, error) {
	out, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}

	etag := ""
	if out.ETag != nil {
		etag = *out.ETag
	}
	return out.Body, etag, nil
}

func (c *S3Client) PutObject(ctx context.Context, bucket, key string, data []byte, ifMatch string) error {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/json"),
	}
	if ifMatch != "" {
		input.IfMatch = aws.String(ifMatch)
	}

	_, err := c.client.PutObject(ctx, input)
	if err != nil {
		if isPreconditionFailed(err) {
			return ErrConflict
		}
		return err
	}
	return nil
}

func (c *S3Client) GetObjectVersion(ctx context.Context, bucket, key, versionID string) (io.ReadCloser, error) {
	out, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket:    aws.String(bucket),
		Key:       aws.String(key),
		VersionId: aws.String(versionID),
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

func (c *S3Client) ListObjectVersions(ctx context.Context, bucket, key string, maxKeys int) ([]ObjectVersion, error) {
	out, err := c.client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{
		Bucket:  aws.String(bucket),
		Prefix:  aws.String(key),
		MaxKeys: aws.Int32(int32(maxKeys)),
	})
	if err != nil {
		return nil, err
	}

	var versions []ObjectVersion
	for _, v := range out.Versions {
		if v.Key != nil && *v.Key == key {
			vid := ""
			if v.VersionId != nil {
				vid = *v.VersionId
			}
			isLatest := v.IsLatest != nil && *v.IsLatest
			versions = append(versions, ObjectVersion{
				VersionID: vid,
				IsLatest:  isLatest,
			})
		}
	}
	return versions, nil
}

// isPreconditionFailed checks for S3 PreconditionFailed errors, which occur
// when a conditional PutObject with IfMatch fails due to an ETag mismatch.
func isPreconditionFailed(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode() == "PreconditionFailed" {
		return true
	}
	return false
}
