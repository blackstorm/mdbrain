package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"mdbrain.dev/internal/config"
	"mdbrain.dev/internal/domain/store"
)

type s3API interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
	CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error)
}

type S3Store struct {
	client    s3API
	bucket    string
	publicURL string
}

type endpointInfo struct {
	Protocol string
	Hostname string
	Port     int
}

func NewS3Store(cfg *config.Config) (*S3Store, error) {
	parsed, err := parseEndpoint(cfg.S3Endpoint)
	if err != nil {
		return nil, err
	}
	client := createS3Client(cfg, parsed)
	return newS3StoreWithClient(client, cfg.S3Bucket, cfg.S3PublicURL)
}

func newS3StoreWithClient(client s3API, bucket, publicURL string) (*S3Store, error) {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return nil, errors.New("S3 bucket is required")
	}
	store := &S3Store{
		client:    client,
		bucket:    bucket,
		publicURL: strings.TrimSpace(publicURL),
	}
	if err := store.ensureBucket(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

func createS3Client(cfg *config.Config, endpoint endpointInfo) *s3.Client {
	url := fmt.Sprintf("%s://%s:%d", endpoint.Protocol, endpoint.Hostname, endpoint.Port)
	awsCfg := aws.Config{
		Region:      cfg.S3Region,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.S3AccessKey, cfg.S3SecretKey, ""),
		EndpointResolverWithOptions: aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			if service != s3.ServiceID {
				return aws.Endpoint{}, &aws.EndpointNotFoundError{}
			}
			return aws.Endpoint{
				URL:               url,
				HostnameImmutable: true,
			}, nil
		}),
	}

	return s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.UsePathStyle = true
	})
}

func parseEndpoint(endpoint string) (endpointInfo, error) {
	raw := strings.TrimSpace(endpoint)
	if raw == "" {
		return endpointInfo{}, errors.New("S3_ENDPOINT is required for S3 storage")
	}

	withScheme := raw
	if !strings.HasPrefix(strings.ToLower(withScheme), "http://") && !strings.HasPrefix(strings.ToLower(withScheme), "https://") {
		withScheme = "http://" + withScheme
	}

	parsed, err := url.Parse(withScheme)
	if err != nil {
		return endpointInfo{}, fmt.Errorf("parse S3 endpoint: %w", err)
	}
	protocol := strings.ToLower(parsed.Scheme)
	if protocol == "" {
		protocol = "http"
	}
	hostname := parsed.Hostname()
	if hostname == "" {
		return endpointInfo{}, fmt.Errorf("invalid S3 endpoint: %s", endpoint)
	}
	port := parsed.Port()
	if port == "" {
		if protocol == "https" {
			return endpointInfo{Protocol: protocol, Hostname: hostname, Port: 443}, nil
		}
		return endpointInfo{Protocol: protocol, Hostname: hostname, Port: 9000}, nil
	}
	n, err := strconv.Atoi(port)
	if err != nil || n <= 0 {
		return endpointInfo{}, fmt.Errorf("invalid S3 endpoint port: %s", endpoint)
	}
	return endpointInfo{
		Protocol: protocol,
		Hostname: hostname,
		Port:     n,
	}, nil
}

func (s *S3Store) PutObject(vaultID, objectKey string, content []byte, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.fullKey(vaultID, objectKey)),
		Body:   bytes.NewReader(content),
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}
	_, err := s.client.PutObject(context.Background(), input)
	return err
}

func (s *S3Store) GetObject(vaultID, objectKey string) (*store.Object, error) {
	out, err := s.client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.fullKey(vaultID, objectKey)),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &store.Object{
		Body:          out.Body,
		ContentLength: aws.ToInt64(out.ContentLength),
		ContentType:   aws.ToString(out.ContentType),
		LastModified:  out.LastModified,
	}, nil
}

func (s *S3Store) DeleteObject(vaultID, objectKey string) error {
	_, err := s.client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.fullKey(vaultID, objectKey)),
	})
	if isNotFound(err) {
		return nil
	}
	return err
}

func (s *S3Store) HeadObject(vaultID, objectKey string) (*store.Metadata, error) {
	out, err := s.client.HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.fullKey(vaultID, objectKey)),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &store.Metadata{
		ContentLength: aws.ToInt64(out.ContentLength),
		ContentType:   aws.ToString(out.ContentType),
		LastModified:  out.LastModified,
	}, nil
}

func (s *S3Store) DeleteVaultObjects(vaultID string) error {
	prefix := store.VaultPrefix(vaultID)
	var token *string

	for {
		out, err := s.client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: token,
		})
		if err != nil {
			return err
		}

		for _, object := range out.Contents {
			if object.Key == nil || *object.Key == "" {
				continue
			}
			if _, err := s.client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
				Bucket: aws.String(s.bucket),
				Key:    object.Key,
			}); err != nil && !isNotFound(err) {
				return err
			}
		}

		if aws.ToBool(out.IsTruncated) {
			token = out.NextContinuationToken
			if token != nil && *token != "" {
				continue
			}
		}
		return nil
	}
}

func (s *S3Store) PublicAssetURL(vaultID, objectKey string) string {
	base := strings.TrimRight(strings.TrimSpace(s.publicURL), "/")
	if base == "" {
		return ""
	}
	return base + "/" + s.bucket + "/" + s.fullKey(vaultID, objectKey)
}

func (s *S3Store) fullKey(vaultID, objectKey string) string {
	return store.VaultPrefix(vaultID) + objectKey
}

func (s *S3Store) ensureBucket(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	})
	if err == nil {
		return nil
	}
	if !isNotFound(err) {
		return fmt.Errorf("S3 connection failed: %w", err)
	}

	_, err = s.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(s.bucket),
	})
	if err != nil {
		var owned *types.BucketAlreadyOwnedByYou
		if errors.As(err, &owned) {
			return nil
		}
		return fmt.Errorf("create S3 bucket: %w", err)
	}
	return nil
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}

	var noSuchKey *types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return true
	}
	var noSuchBucket *types.NoSuchBucket
	if errors.As(err, &noSuchBucket) {
		return true
	}
	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return true
	}

	var apiError smithy.APIError
	if errors.As(err, &apiError) {
		switch apiError.ErrorCode() {
		case "NotFound", "NoSuchKey", "NoSuchBucket", "404":
			return true
		}
	}
	return false
}
