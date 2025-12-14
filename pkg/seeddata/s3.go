package seeddata

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	awshttp "github.com/aws/smithy-go/transport/http"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultS3Bucket is the default S3 bucket for xatu-cbt seed data.
	DefaultS3Bucket = "ethpandaops-platform-production-public"

	// DefaultS3PublicDomain is the public domain for the S3 bucket.
	DefaultS3PublicDomain = "data.ethpandaops.io"

	// DefaultS3Prefix is the default path prefix in the S3 bucket.
	DefaultS3Prefix = "xatu-cbt"

	// DefaultS3Region is the default region (required by SDK, but endpoint controls routing).
	DefaultS3Region = "us-east-1"

	// DefaultS3Endpoint is the default S3 endpoint (Cloudflare R2).
	DefaultS3Endpoint = "https://539bc53131934672bf85e7260ec0b218.r2.cloudflarestorage.com"

	// EnvS3Endpoint is the environment variable for custom S3 endpoint.
	EnvS3Endpoint = "S3_ENDPOINT"

	// EnvS3Bucket is the environment variable for custom S3 bucket name.
	EnvS3Bucket = "S3_BUCKET"
)

// S3Uploader handles uploading parquet files to S3.
type S3Uploader struct {
	log          logrus.FieldLogger
	client       *s3.Client
	bucket       string
	publicDomain string
	prefix       string
}

// NewS3Uploader creates a new S3 uploader.
// It reads AWS credentials from environment variables or AWS_PROFILE.
// For S3-compatible services (DigitalOcean Spaces, MinIO, etc.), set S3_ENDPOINT.
// To use a custom bucket, set S3_BUCKET.
func NewS3Uploader(ctx context.Context, log logrus.FieldLogger) (*S3Uploader, error) {
	// Load AWS config - always set region (required by SDK even with custom endpoint)
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(DefaultS3Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Use custom endpoint or default to DigitalOcean Spaces AMS3
	endpoint := os.Getenv(EnvS3Endpoint)
	if endpoint == "" {
		endpoint = DefaultS3Endpoint
	}

	log.WithField("endpoint", endpoint).Debug("using S3 endpoint")

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = &endpoint
		o.UsePathStyle = true // Required for most S3-compatible services
	})

	// Check for custom bucket
	bucket := DefaultS3Bucket
	if customBucket := os.Getenv(EnvS3Bucket); customBucket != "" {
		bucket = customBucket
	}

	return &S3Uploader{
		log:          log.WithField("component", "s3uploader"),
		client:       client,
		bucket:       bucket,
		publicDomain: DefaultS3PublicDomain,
		prefix:       DefaultS3Prefix,
	}, nil
}

// UploadOptions contains options for uploading to S3.
type UploadOptions struct {
	LocalPath string // Path to local file
	Network   string // Network name (e.g., "mainnet", "sepolia")
	Spec      string // Fork spec (e.g., "pectra", "fusaka")
	Model     string // Model name (e.g., "beacon_api_eth_v1_events_block")
	Filename  string // Custom filename (without .parquet extension, defaults to Model)
}

// UploadResult contains the result of an S3 upload.
type UploadResult struct {
	S3URL     string // S3 URL (s3://bucket/path)
	PublicURL string // Public HTTPS URL
}

// Upload uploads a parquet file to S3.
func (u *S3Uploader) Upload(ctx context.Context, opts UploadOptions) (*UploadResult, error) {
	// Use custom filename or default to model name
	filename := opts.Filename
	if filename == "" {
		filename = opts.Model
	}

	// Build S3 key
	key := fmt.Sprintf("%s/%s/%s/%s.parquet", u.prefix, opts.Network, opts.Spec, filename)

	u.log.WithFields(logrus.Fields{
		"bucket": u.bucket,
		"key":    key,
		"file":   opts.LocalPath,
	}).Debug("uploading to S3")

	// Open local file
	file, err := os.Open(opts.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Get file size for explicit ContentLength
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	fileSize := fileInfo.Size()

	u.log.WithFields(logrus.Fields{
		"file": opts.LocalPath,
		"size": fileSize,
		"key":  key,
	}).Debug("uploading file to S3")

	// Upload to S3 with explicit content length
	_, err = u.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(u.bucket),
		Key:           aws.String(key),
		Body:          file,
		ContentType:   aws.String("application/octet-stream"),
		ContentLength: aws.Int64(fileSize),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to upload to S3: %w", err)
	}

	// Verify upload by checking object metadata
	headResp, headErr := u.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(u.bucket),
		Key:    aws.String(key),
	})
	if headErr != nil {
		u.log.WithError(headErr).Warn("failed to verify upload")
	} else if headResp.ContentLength != nil && *headResp.ContentLength != fileSize {
		return nil, fmt.Errorf("upload verification failed: expected %d bytes but S3 reports %d bytes",
			fileSize, *headResp.ContentLength)
	} else {
		u.log.WithField("verified_size", *headResp.ContentLength).Debug("upload verified")
	}

	return &UploadResult{
		S3URL:     fmt.Sprintf("s3://%s/%s", u.bucket, key),
		PublicURL: fmt.Sprintf("https://%s/%s", u.publicDomain, key),
	}, nil
}

// SetBucket sets a custom S3 bucket (for testing or custom destinations).
func (u *S3Uploader) SetBucket(bucket string) {
	u.bucket = bucket
}

// SetPrefix sets a custom S3 prefix (for testing or custom destinations).
func (u *S3Uploader) SetPrefix(prefix string) {
	u.prefix = prefix
}

// ObjectExists checks if an object already exists at the given path.
// Returns true if the object exists, false otherwise.
func (u *S3Uploader) ObjectExists(ctx context.Context, network, spec, filename string) (bool, error) {
	key := fmt.Sprintf("%s/%s/%s/%s.parquet", u.prefix, network, spec, filename)

	_, err := u.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(u.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		// Check if it's a "not found" error
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return false, nil
		}

		// Check for NoSuchKey error (some S3-compatible services use this)
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			return false, nil
		}

		// For other errors, check if it's a 404 status code
		var respErr *awshttp.ResponseError
		if errors.As(err, &respErr) && respErr.HTTPStatusCode() == 404 {
			return false, nil
		}

		return false, fmt.Errorf("failed to check object existence: %w", err)
	}

	return true, nil
}

// GetPublicURL returns the public URL for an object without uploading.
func (u *S3Uploader) GetPublicURL(network, spec, filename string) string {
	key := fmt.Sprintf("%s/%s/%s/%s.parquet", u.prefix, network, spec, filename)

	return fmt.Sprintf("https://%s/%s", u.publicDomain, key)
}

// CheckAccess verifies the uploader has write access to the S3 bucket.
// It attempts to list objects at the prefix to verify credentials and permissions.
func (u *S3Uploader) CheckAccess(ctx context.Context) error {
	u.log.WithFields(logrus.Fields{
		"bucket": u.bucket,
		"prefix": u.prefix,
	}).Debug("checking S3 access")

	// Try to list objects at the prefix - this verifies:
	// 1. AWS credentials are valid
	// 2. User has at least read access to the bucket
	// Note: This doesn't guarantee write access, but catches most common issues
	_, err := u.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(u.bucket),
		Prefix:  aws.String(u.prefix + "/"),
		MaxKeys: aws.Int32(1),
	})
	if err != nil {
		return fmt.Errorf("S3 access check failed (bucket: %s): %w", u.bucket, err)
	}

	return nil
}
