// Package objectstore wraps an S3-compatible client (real AWS S3 in prod,
// MinIO in dev) behind a small Put/Get/Delete/Exists API. Banner pass-through
// in Phase 2 builds on this; same interface stays valid if the prod backend
// later swaps to Cloudflare R2 or a provider-native bucket.
package objectstore

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Config struct {
	Endpoint     string // empty = AWS S3 default; e.g. "http://localhost:9000" for MinIO
	Region       string
	Bucket       string
	AccessKey    string
	SecretKey    string
	UsePathStyle bool // true for MinIO; false for AWS S3 virtual-hosted style
}

type Client struct {
	s3     *s3.Client
	bucket string
}

func New(ctx context.Context, cfg Config) (*Client, error) {
	loadOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}
	if cfg.AccessKey != "" {
		loadOpts = append(loadOpts,
			awsconfig.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
			),
		)
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	clientOpts := []func(*s3.Options){}
	if cfg.Endpoint != "" {
		ep := cfg.Endpoint
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(ep)
		})
	}
	if cfg.UsePathStyle {
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	return &Client{
		s3:     s3.NewFromConfig(awsCfg, clientOpts...),
		bucket: cfg.Bucket,
	}, nil
}

func (c *Client) Put(ctx context.Context, key string, body io.Reader, contentType string) error {
	in := &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
		Body:   body,
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	if _, err := c.s3.PutObject(ctx, in); err != nil {
		return fmt.Errorf("put %s: %w", key, err)
	}
	return nil
}

// Get returns the object body and its Content-Type. Caller must close the body.
func (c *Client) Get(ctx context.Context, key string) (io.ReadCloser, string, error) {
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, "", fmt.Errorf("get %s: %w", key, err)
	}
	contentType := ""
	if out.ContentType != nil {
		contentType = *out.ContentType
	}
	return out.Body, contentType, nil
}

func (c *Client) Delete(ctx context.Context, key string) error {
	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("delete %s: %w", key, err)
	}
	return nil
}

// Exists returns whether the key resolves to an object. Errors that aren't
// NoSuchKey/404 are surfaced (network, permissions, etc.).
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	_, err := c.s3.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, nil
	}
	var nsk *types.NoSuchKey
	var nf *types.NotFound
	if errors.As(err, &nsk) || errors.As(err, &nf) {
		return false, nil
	}
	// Some implementations surface 404 only via API-level error.
	return false, fmt.Errorf("head %s: %w", key, err)
}
