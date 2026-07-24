// Package storage provides the only object-storage boundary used by VisionAI.
package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type ObjectInfo struct {
	Key  string
	Size int64
}

type Provider interface {
	EnsureBucket(context.Context) error
	Put(context.Context, string, io.Reader, int64, string) error
	Compose(context.Context, string, []string) error
	Get(context.Context, string) (io.ReadCloser, ObjectInfo, error)
	Stat(context.Context, string) (ObjectInfo, error)
	Delete(context.Context, string) error
	List(context.Context, string) ([]ObjectInfo, error)
	PresignedGet(context.Context, string, time.Duration) (*url.URL, error)
	URI(string) string
}

type MinIO struct {
	client        *minio.Client
	presignClient *minio.Client
	bucket        string
}

func NewMinIO(cfg config.Config) (*MinIO, error) {
	client, err := minio.New(cfg.S3Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.S3AccessKey, cfg.S3SecretKey, ""),
		Secure: cfg.S3Secure,
		Region: "us-east-1",
	})
	if err != nil {
		return nil, fmt.Errorf("create S3 client: %w", err)
	}
	presignClient := client
	if cfg.S3PublicEndpoint != "" && cfg.S3PublicEndpoint != cfg.S3Endpoint {
		presignClient, err = minio.New(cfg.S3PublicEndpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(cfg.S3AccessKey, cfg.S3SecretKey, ""),
			Secure: cfg.S3Secure,
			Region: "us-east-1",
		})
		if err != nil {
			return nil, fmt.Errorf("create public S3 client: %w", err)
		}
	}
	return &MinIO{client: client, presignClient: presignClient, bucket: cfg.S3Bucket}, nil
}

func (m *MinIO) EnsureBucket(ctx context.Context) error {
	exists, err := m.client.BucketExists(ctx, m.bucket)
	if err != nil {
		return fmt.Errorf("check S3 bucket: %w", err)
	}
	if !exists {
		if err = m.client.MakeBucket(ctx, m.bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("create S3 bucket: %w", err)
		}
	}
	return nil
}

func (m *MinIO) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	_, err := m.client.PutObject(ctx, m.bucket, key, body, size, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("put S3 object: %w", err)
	}
	return nil
}

func (m *MinIO) Compose(ctx context.Context, destination string, sources []string) error {
	src := make([]minio.CopySrcOptions, 0, len(sources))
	for _, key := range sources {
		src = append(src, minio.CopySrcOptions{Bucket: m.bucket, Object: key})
	}
	_, err := m.client.ComposeObject(ctx, minio.CopyDestOptions{Bucket: m.bucket, Object: destination}, src...)
	if err != nil {
		return fmt.Errorf("compose S3 object: %w", err)
	}
	return nil
}

func (m *MinIO) Get(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error) {
	object, err := m.client.GetObject(ctx, m.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, ObjectInfo{}, fmt.Errorf("get S3 object: %w", err)
	}
	stat, err := object.Stat()
	if err != nil {
		_ = object.Close()
		return nil, ObjectInfo{}, fmt.Errorf("stat S3 object: %w", err)
	}
	return object, ObjectInfo{Key: key, Size: stat.Size}, nil
}

func (m *MinIO) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	stat, err := m.client.StatObject(ctx, m.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("stat S3 object: %w", err)
	}
	return ObjectInfo{Key: key, Size: stat.Size}, nil
}

func (m *MinIO) Delete(ctx context.Context, key string) error {
	if err := m.client.RemoveObject(ctx, m.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete S3 object: %w", err)
	}
	return nil
}

func (m *MinIO) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	rows := make([]ObjectInfo, 0)
	for object := range m.client.ListObjects(ctx, m.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if object.Err != nil {
			return nil, fmt.Errorf("list S3 objects: %w", object.Err)
		}
		rows = append(rows, ObjectInfo{Key: object.Key, Size: object.Size})
	}
	return rows, nil
}

func (m *MinIO) PresignedGet(ctx context.Context, key string, expiry time.Duration) (*url.URL, error) {
	return m.presignClient.PresignedGetObject(ctx, m.bucket, key, expiry, nil)
}

func (m *MinIO) URI(key string) string { return "s3://" + m.bucket + "/" + key }
