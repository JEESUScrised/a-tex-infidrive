package blobstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Config — параметры S3-совместимого API (в т.ч. Cloudflare R2).
type S3Config struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	ForcePathStyle  bool
	Prefix          string // необязательный префикс ключей
}

// S3 — приватное объектное хранилище через AWS SDK v2.
type S3 struct {
	client   *s3.Client
	uploader *manager.Uploader
	bucket   string
	prefix   string
	name     string
}

func NewS3(cfg S3Config) (*S3, error) {
	if cfg.Bucket == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, ErrNotConfigured
	}
	region := cfg.Region
	if region == "" {
		region = "auto"
	}
	resolver := aws.EndpointResolverWithOptionsFunc(func(service, _ string, _ ...interface{}) (aws.Endpoint, error) {
		if cfg.Endpoint == "" {
			return aws.Endpoint{}, &aws.EndpointNotFoundError{}
		}
		return aws.Endpoint{
			URL:               cfg.Endpoint,
			HostnameImmutable: true,
			SigningRegion:     region,
		}, nil
	})
	awsCfg := aws.Config{
		Region:                      region,
		Credentials:                 credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		EndpointResolverWithOptions: resolver,
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.ForcePathStyle
	})
	return &S3{
		client:   client,
		uploader: manager.NewUploader(client),
		bucket:   cfg.Bucket,
		prefix:   strings.Trim(cfg.Prefix, "/"),
		name:     "s3",
	}, nil
}

func (s *S3) Name() string { return s.name }

func (s *S3) key(k string) string {
	k = strings.TrimPrefix(k, "/")
	if s.prefix == "" {
		return k
	}
	return s.prefix + "/" + k
}

func (s *S3) Available(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

func (s *S3) Put(ctx context.Context, key string, r io.Reader, opts PutOptions) (ObjectMeta, error) {
	ct := opts.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	// Без ACL: bucket должен быть private по умолчанию (R2/S3).
	out, err := s.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(s.key(key)),
		Body:        r,
		ContentType: aws.String(ct),
	})
	if err != nil {
		return ObjectMeta{}, err
	}
	etag := ""
	if out.ETag != nil {
		etag = strings.Trim(*out.ETag, `"`)
	}
	size := opts.Size
	if size <= 0 {
		if h, err := s.Head(ctx, key); err == nil {
			size = h.Size
		}
	}
	return ObjectMeta{Key: key, Size: size, ContentType: ct, ETag: etag, LastModified: time.Now().UTC()}, nil
}

func (s *S3) Get(ctx context.Context, key string) (io.ReadCloser, ObjectMeta, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(key)),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, ObjectMeta{}, ErrNotFound
		}
		return nil, ObjectMeta{}, err
	}
	meta := ObjectMeta{Key: key, ContentType: aws.ToString(out.ContentType)}
	if out.ContentLength != nil {
		meta.Size = *out.ContentLength
	}
	if out.ETag != nil {
		meta.ETag = strings.Trim(*out.ETag, `"`)
	}
	if out.LastModified != nil {
		meta.LastModified = *out.LastModified
	}
	return out.Body, meta, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(key)),
	})
	return err
}

func (s *S3) Head(ctx context.Context, key string) (ObjectMeta, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(key)),
	})
	if err != nil {
		if isNotFound(err) {
			return ObjectMeta{}, ErrNotFound
		}
		return ObjectMeta{}, err
	}
	meta := ObjectMeta{Key: key, ContentType: aws.ToString(out.ContentType)}
	if out.ContentLength != nil {
		meta.Size = *out.ContentLength
	}
	if out.ETag != nil {
		meta.ETag = strings.Trim(*out.ETag, `"`)
	}
	if out.LastModified != nil {
		meta.LastModified = *out.LastModified
	}
	return meta, nil
}

func (s *S3) PresignPut(ctx context.Context, key, contentType string, expiry time.Duration) (string, error) {
	if expiry <= 0 {
		expiry = 15 * time.Minute
	}
	ps := s3.NewPresignClient(s.client)
	in := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(key)),
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	out, err := ps.PresignPutObject(ctx, in, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", err
	}
	return out.URL, nil
}

func (s *S3) PresignGet(ctx context.Context, key string, expiry time.Duration) (string, error) {
	if expiry <= 0 {
		expiry = 15 * time.Minute
	}
	ps := s3.NewPresignClient(s.client)
	out, err := ps.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(key)),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", err
	}
	return out.URL, nil
}

func isNotFound(err error) bool {
	var nsk *types.NotFound
	var nsk2 *types.NoSuchKey
	return errors.As(err, &nsk) || errors.As(err, &nsk2) || strings.Contains(strings.ToLower(err.Error()), "not found") || strings.Contains(err.Error(), "404")
}
