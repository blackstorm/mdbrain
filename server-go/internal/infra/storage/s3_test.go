package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

type mockS3Client struct {
	putObjectFn      func(ctx context.Context, in *s3.PutObjectInput) (*s3.PutObjectOutput, error)
	getObjectFn      func(ctx context.Context, in *s3.GetObjectInput) (*s3.GetObjectOutput, error)
	deleteObjectFn   func(ctx context.Context, in *s3.DeleteObjectInput) (*s3.DeleteObjectOutput, error)
	headObjectFn     func(ctx context.Context, in *s3.HeadObjectInput) (*s3.HeadObjectOutput, error)
	listObjectsV2Fn  func(ctx context.Context, in *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error)
	headBucketFn     func(ctx context.Context, in *s3.HeadBucketInput) (*s3.HeadBucketOutput, error)
	createBucketFn   func(ctx context.Context, in *s3.CreateBucketInput) (*s3.CreateBucketOutput, error)
	receivedPutBody  []byte
	receivedPutKey   string
	receivedGetKey   string
	receivedHeadKey  string
	receivedDelKeys  []string
	receivedListPref []string
}

func (m *mockS3Client) PutObject(ctx context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if in != nil && in.Body != nil {
		body, _ := io.ReadAll(in.Body)
		m.receivedPutBody = body
	}
	if in != nil {
		m.receivedPutKey = aws.ToString(in.Key)
	}
	if m.putObjectFn != nil {
		return m.putObjectFn(ctx, in)
	}
	return &s3.PutObjectOutput{}, nil
}

func (m *mockS3Client) GetObject(ctx context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if in != nil {
		m.receivedGetKey = aws.ToString(in.Key)
	}
	if m.getObjectFn != nil {
		return m.getObjectFn(ctx, in)
	}
	return &s3.GetObjectOutput{}, nil
}

func (m *mockS3Client) DeleteObject(ctx context.Context, in *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	if in != nil && in.Key != nil {
		m.receivedDelKeys = append(m.receivedDelKeys, *in.Key)
	}
	if m.deleteObjectFn != nil {
		return m.deleteObjectFn(ctx, in)
	}
	return &s3.DeleteObjectOutput{}, nil
}

func (m *mockS3Client) HeadObject(ctx context.Context, in *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if in != nil {
		m.receivedHeadKey = aws.ToString(in.Key)
	}
	if m.headObjectFn != nil {
		return m.headObjectFn(ctx, in)
	}
	return &s3.HeadObjectOutput{}, nil
}

func (m *mockS3Client) ListObjectsV2(ctx context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if in != nil {
		m.receivedListPref = append(m.receivedListPref, aws.ToString(in.Prefix))
	}
	if m.listObjectsV2Fn != nil {
		return m.listObjectsV2Fn(ctx, in)
	}
	return &s3.ListObjectsV2Output{}, nil
}

func (m *mockS3Client) HeadBucket(ctx context.Context, in *s3.HeadBucketInput, _ ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	if m.headBucketFn != nil {
		return m.headBucketFn(ctx, in)
	}
	return &s3.HeadBucketOutput{}, nil
}

func (m *mockS3Client) CreateBucket(ctx context.Context, in *s3.CreateBucketInput, _ ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	if m.createBucketFn != nil {
		return m.createBucketFn(ctx, in)
	}
	return &s3.CreateBucketOutput{}, nil
}

func newDirectS3Store(client s3API) *S3Store {
	return &S3Store{
		client:    client,
		bucket:    "test-bucket",
		publicURL: "https://s3.example.com",
	}
}

func TestParseEndpoint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		protocol string
		hostname string
		port     int
	}{
		{name: "http with port", input: "http://localhost:9000", protocol: "http", hostname: "localhost", port: 9000},
		{name: "https with port", input: "https://minio.example.com:443", protocol: "https", hostname: "minio.example.com", port: 443},
		{name: "http default port", input: "http://localhost", protocol: "http", hostname: "localhost", port: 9000},
		{name: "https default port", input: "https://s3.amazonaws.com", protocol: "https", hostname: "s3.amazonaws.com", port: 443},
		{name: "trailing path", input: "http://storage.local:9000/path/ignored", protocol: "http", hostname: "storage.local", port: 9000},
		{name: "missing scheme", input: "storage.local:9000/path/ignored", protocol: "http", hostname: "storage.local", port: 9000},
		{name: "ip endpoint", input: "http://192.168.1.100:9000", protocol: "http", hostname: "192.168.1.100", port: 9000},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseEndpoint(tc.input)
			if err != nil {
				t.Fatalf("parse endpoint: %v", err)
			}
			if got.Protocol != tc.protocol || got.Hostname != tc.hostname || got.Port != tc.port {
				t.Fatalf("unexpected endpoint: %#v", got)
			}
		})
	}
}

func TestNewS3StoreWithClientEnsuresBucket(t *testing.T) {
	t.Parallel()

	t.Run("bucket exists", func(t *testing.T) {
		t.Parallel()
		headCalls := 0
		createCalls := 0
		client := &mockS3Client{
			headBucketFn: func(ctx context.Context, in *s3.HeadBucketInput) (*s3.HeadBucketOutput, error) {
				headCalls++
				if got := aws.ToString(in.Bucket); got != "test-bucket" {
					t.Fatalf("unexpected bucket: %s", got)
				}
				return &s3.HeadBucketOutput{}, nil
			},
			createBucketFn: func(ctx context.Context, in *s3.CreateBucketInput) (*s3.CreateBucketOutput, error) {
				createCalls++
				return &s3.CreateBucketOutput{}, nil
			},
		}
		_, err := newS3StoreWithClient(client, "test-bucket", "https://s3.example.com")
		if err != nil {
			t.Fatalf("new store: %v", err)
		}
		if headCalls != 1 || createCalls != 0 {
			t.Fatalf("expected head=1 create=0, got head=%d create=%d", headCalls, createCalls)
		}
	})

	t.Run("bucket missing then create", func(t *testing.T) {
		t.Parallel()
		headCalls := 0
		createCalls := 0
		client := &mockS3Client{
			headBucketFn: func(ctx context.Context, in *s3.HeadBucketInput) (*s3.HeadBucketOutput, error) {
				headCalls++
				return nil, &types.NotFound{}
			},
			createBucketFn: func(ctx context.Context, in *s3.CreateBucketInput) (*s3.CreateBucketOutput, error) {
				createCalls++
				if got := aws.ToString(in.Bucket); got != "test-bucket" {
					t.Fatalf("unexpected bucket: %s", got)
				}
				return &s3.CreateBucketOutput{}, nil
			},
		}
		_, err := newS3StoreWithClient(client, "test-bucket", "https://s3.example.com")
		if err != nil {
			t.Fatalf("new store: %v", err)
		}
		if headCalls != 1 || createCalls != 1 {
			t.Fatalf("expected head=1 create=1, got head=%d create=%d", headCalls, createCalls)
		}
	})

	t.Run("connection failure", func(t *testing.T) {
		t.Parallel()
		client := &mockS3Client{
			headBucketFn: func(ctx context.Context, in *s3.HeadBucketInput) (*s3.HeadBucketOutput, error) {
				return nil, errors.New("dial tcp: connection refused")
			},
		}
		_, err := newS3StoreWithClient(client, "test-bucket", "https://s3.example.com")
		if err == nil || !strings.Contains(err.Error(), "S3 connection failed") {
			t.Fatalf("expected connection error, got: %v", err)
		}
	})
}

func TestS3PutObject(t *testing.T) {
	t.Parallel()

	client := &mockS3Client{}
	s := newDirectS3Store(client)
	content := []byte("Binary\x00Data")
	if err := s.PutObject("test-vault-123", "assets/file.bin", content, "application/octet-stream"); err != nil {
		t.Fatalf("put object: %v", err)
	}

	expectedKey := "testvault123/assets/file.bin"
	if client.receivedPutKey != expectedKey {
		t.Fatalf("unexpected key: %s", client.receivedPutKey)
	}
	if !bytes.Equal(client.receivedPutBody, content) {
		t.Fatalf("unexpected body: %#v", client.receivedPutBody)
	}
}

func TestS3GetObject(t *testing.T) {
	t.Parallel()

	modTime := time.Now().UTC()
	client := &mockS3Client{
		getObjectFn: func(ctx context.Context, in *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{
				Body:          io.NopCloser(strings.NewReader("hello")),
				ContentLength: aws.Int64(5),
				ContentType:   aws.String("text/plain"),
				LastModified:  &modTime,
			}, nil
		},
	}
	s := newDirectS3Store(client)
	obj, err := s.GetObject("test-vault-123", "hello.txt")
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	if obj == nil || obj.ContentLength != 5 || obj.ContentType != "text/plain" {
		t.Fatalf("unexpected object: %#v", obj)
	}
	if client.receivedGetKey != "testvault123/hello.txt" {
		t.Fatalf("unexpected key: %s", client.receivedGetKey)
	}
}

func TestS3GetObjectNotFound(t *testing.T) {
	t.Parallel()

	client := &mockS3Client{
		getObjectFn: func(ctx context.Context, in *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
			return nil, &types.NoSuchKey{}
		},
	}
	s := newDirectS3Store(client)
	obj, err := s.GetObject("test-vault-123", "missing.txt")
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	if obj != nil {
		t.Fatalf("expected nil object, got: %#v", obj)
	}
}

func TestS3HeadObject(t *testing.T) {
	t.Parallel()

	modTime := time.Now().UTC()
	client := &mockS3Client{
		headObjectFn: func(ctx context.Context, in *s3.HeadObjectInput) (*s3.HeadObjectOutput, error) {
			return &s3.HeadObjectOutput{
				ContentLength: aws.Int64(1024),
				ContentType:   aws.String("image/png"),
				LastModified:  &modTime,
			}, nil
		},
	}
	s := newDirectS3Store(client)
	meta, err := s.HeadObject("test-vault-123", "image.png")
	if err != nil {
		t.Fatalf("head object: %v", err)
	}
	if meta == nil || meta.ContentLength != 1024 || meta.ContentType != "image/png" {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
	if client.receivedHeadKey != "testvault123/image.png" {
		t.Fatalf("unexpected key: %s", client.receivedHeadKey)
	}
}

func TestS3HeadObjectNotFound(t *testing.T) {
	t.Parallel()

	client := &mockS3Client{
		headObjectFn: func(ctx context.Context, in *s3.HeadObjectInput) (*s3.HeadObjectOutput, error) {
			return nil, &types.NotFound{}
		},
	}
	s := newDirectS3Store(client)
	meta, err := s.HeadObject("test-vault-123", "missing.png")
	if err != nil {
		t.Fatalf("head object: %v", err)
	}
	if meta != nil {
		t.Fatalf("expected nil metadata, got: %#v", meta)
	}
}

func TestS3DeleteObject(t *testing.T) {
	t.Parallel()

	client := &mockS3Client{}
	s := newDirectS3Store(client)
	if err := s.DeleteObject("test-vault-123", "to-delete.txt"); err != nil {
		t.Fatalf("delete object: %v", err)
	}
	if len(client.receivedDelKeys) != 1 || client.receivedDelKeys[0] != "testvault123/to-delete.txt" {
		t.Fatalf("unexpected delete keys: %#v", client.receivedDelKeys)
	}
}

func TestS3DeleteObjectNotFoundIgnored(t *testing.T) {
	t.Parallel()

	client := &mockS3Client{
		deleteObjectFn: func(ctx context.Context, in *s3.DeleteObjectInput) (*s3.DeleteObjectOutput, error) {
			return nil, &types.NoSuchKey{}
		},
	}
	s := newDirectS3Store(client)
	if err := s.DeleteObject("test-vault-123", "missing.txt"); err != nil {
		t.Fatalf("delete object: %v", err)
	}
}

func TestS3DeleteVaultObjects(t *testing.T) {
	t.Parallel()

	page := 0
	client := &mockS3Client{
		listObjectsV2Fn: func(ctx context.Context, in *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
			page++
			if page == 1 {
				if aws.ToString(in.ContinuationToken) != "" {
					t.Fatalf("first page should not send token")
				}
				return &s3.ListObjectsV2Output{
					Contents: []types.Object{
						{Key: aws.String("testvault123/file1.txt")},
					},
					IsTruncated:           aws.Bool(true),
					NextContinuationToken: aws.String("next-1"),
				}, nil
			}
			if aws.ToString(in.ContinuationToken) != "next-1" {
				t.Fatalf("unexpected token on second page: %q", aws.ToString(in.ContinuationToken))
			}
			return &s3.ListObjectsV2Output{
				Contents: []types.Object{
					{Key: aws.String("testvault123/file2.txt")},
				},
				IsTruncated: aws.Bool(false),
			}, nil
		},
	}
	s := newDirectS3Store(client)
	if err := s.DeleteVaultObjects("test-vault-123"); err != nil {
		t.Fatalf("delete vault objects: %v", err)
	}

	if page != 2 {
		t.Fatalf("expected 2 list pages, got %d", page)
	}
	if len(client.receivedDelKeys) != 2 {
		t.Fatalf("expected 2 deleted keys, got %#v", client.receivedDelKeys)
	}
	if client.receivedDelKeys[0] != "testvault123/file1.txt" || client.receivedDelKeys[1] != "testvault123/file2.txt" {
		t.Fatalf("unexpected deleted keys: %#v", client.receivedDelKeys)
	}
	if len(client.receivedListPref) == 0 || client.receivedListPref[0] != "testvault123/" {
		t.Fatalf("unexpected list prefix: %#v", client.receivedListPref)
	}
}

func TestS3DeleteVaultObjectsEmpty(t *testing.T) {
	t.Parallel()

	client := &mockS3Client{
		listObjectsV2Fn: func(ctx context.Context, in *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{IsTruncated: aws.Bool(false)}, nil
		},
	}
	s := newDirectS3Store(client)
	if err := s.DeleteVaultObjects("test-vault-123"); err != nil {
		t.Fatalf("delete vault objects: %v", err)
	}
	if len(client.receivedDelKeys) != 0 {
		t.Fatalf("expected no deletes, got %#v", client.receivedDelKeys)
	}
}

func TestS3PublicAssetURL(t *testing.T) {
	t.Parallel()

	client := &mockS3Client{}
	s := &S3Store{
		client:    client,
		bucket:    "test-bucket",
		publicURL: "https://s3.example.com/",
	}
	got := s.PublicAssetURL("test-vault-123", "assets/image.png")
	want := "https://s3.example.com/test-bucket/testvault123/assets/image.png"
	if got != want {
		t.Fatalf("unexpected public URL:\nwant: %s\ngot:  %s", want, got)
	}
}

func TestS3PublicAssetURLMissingConfig(t *testing.T) {
	t.Parallel()

	s := &S3Store{
		client:    &mockS3Client{},
		bucket:    "test-bucket",
		publicURL: "",
	}
	if got := s.PublicAssetURL("test-vault-123", "file.txt"); got != "" {
		t.Fatalf("expected empty URL, got: %q", got)
	}
}

type mockAPIError struct {
	code string
	msg  string
}

func (m *mockAPIError) Error() string        { return m.msg }
func (m *mockAPIError) ErrorCode() string    { return m.code }
func (m *mockAPIError) ErrorMessage() string { return m.msg }
func (m *mockAPIError) ErrorFault() smithy.ErrorFault {
	return smithy.FaultClient
}

func TestIsNotFound(t *testing.T) {
	t.Parallel()

	if !isNotFound(&types.NoSuchKey{}) {
		t.Fatal("expected NoSuchKey to be not found")
	}
	if !isNotFound(&types.NotFound{}) {
		t.Fatal("expected NotFound to be not found")
	}
	if !isNotFound(&mockAPIError{code: "404", msg: "not found"}) {
		t.Fatal("expected APIError 404 to be not found")
	}
	if isNotFound(errors.New("boom")) {
		t.Fatal("unexpected not found for generic error")
	}
}
