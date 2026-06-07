package s3

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	s3api "github.com/aws/aws-sdk-go-v2/service/s3"
)

// Config holds MinIO/S3 connection settings from environment variables.
type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
}

func ConfigFromEnv() Config {
	bucket := os.Getenv("MINIO_BUCKET")
	if bucket == "" {
		bucket = "music"
	}
	region := os.Getenv("MINIO_REGION")
	if region == "" {
		region = "us-east-1"
	}
	return Config{
		Endpoint:  strings.TrimSpace(os.Getenv("MINIO_ENDPOINT")),
		AccessKey: strings.TrimSpace(os.Getenv("MINIO_ACCESS_KEY")),
		SecretKey: strings.TrimSpace(os.Getenv("MINIO_SECRET_KEY")),
		Bucket:    bucket,
		Region:    region,
	}
}

func (c Config) Enabled() bool {
	return c.Endpoint != "" && c.AccessKey != "" && c.SecretKey != ""
}

func (c Config) MusicURI() string {
	return "s3://" + c.Bucket + "/"
}

type Client struct {
	cfg    Config
	client *s3api.Client
}

var (
	defaultClient *Client
	defaultOnce   sync.Once
)

// Default returns a process-wide client built from MINIO_* env vars, or nil if unset.
func Default() *Client {
	defaultOnce.Do(func() {
		cfg := ConfigFromEnv()
		if !cfg.Enabled() {
			return
		}
		c, err := NewClient(cfg)
		if err == nil {
			defaultClient = c
		}
	})
	return defaultClient
}

func NewClient(cfg Config) (*Client, error) {
	if !cfg.Enabled() {
		return nil, fmt.Errorf("s3 not configured")
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
	)
	if err != nil {
		return nil, err
	}
	client := s3api.NewFromConfig(awsCfg, func(o *s3api.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = true
	})
	return &Client{cfg: cfg, client: client}, nil
}

func (c *Client) Config() Config { return c.cfg }

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.client.HeadBucket(ctx, &s3api.HeadBucketInput{Bucket: aws.String(c.cfg.Bucket)})
	return err
}

type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
}

func (c *Client) ListObjects(ctx context.Context, bucket, prefix string) ([]ObjectInfo, error) {
	var out []ObjectInfo
	paginator := s3api.NewListObjectsV2Paginator(c.client, &s3api.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			if obj.Key == nil {
				continue
			}
			key := *obj.Key
			if strings.HasSuffix(key, "/") {
				continue
			}
			info := ObjectInfo{Key: key}
			if obj.Size != nil {
				info.Size = *obj.Size
			}
			if obj.LastModified != nil {
				info.LastModified = *obj.LastModified
			}
			out = append(out, info)
		}
	}
	return out, nil
}

func (c *Client) HeadObject(ctx context.Context, bucket, key string) (ObjectInfo, error) {
	resp, err := c.client.HeadObject(ctx, &s3api.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return ObjectInfo{}, err
	}
	info := ObjectInfo{Key: key}
	if resp.ContentLength != nil {
		info.Size = *resp.ContentLength
	}
	if resp.LastModified != nil {
		info.LastModified = *resp.LastModified
	}
	return info, nil
}

// ObjectStream is the body and metadata from GetObject.
type ObjectStream struct {
	Body          io.ReadCloser
	ContentLength int64
	ContentType   string
	ContentRange  string
	Partial       bool
}

func (c *Client) GetObject(ctx context.Context, bucket, key string, rangeHeader string) (*ObjectStream, error) {
	if !ValidKey(key) {
		return nil, fmt.Errorf("invalid s3 key")
	}
	in := &s3api.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}
	if rangeHeader != "" {
		in.Range = aws.String(rangeHeader)
	}
	resp, err := c.client.GetObject(ctx, in)
	if err != nil {
		return nil, err
	}
	out := &ObjectStream{Body: resp.Body}
	if resp.ContentLength != nil {
		out.ContentLength = *resp.ContentLength
	}
	if resp.ContentType != nil {
		out.ContentType = *resp.ContentType
	}
	if resp.ContentRange != nil {
		out.ContentRange = *resp.ContentRange
		out.Partial = true
	}
	return out, nil
}

func (c *Client) DownloadToFile(ctx context.Context, bucket, key, dest string) error {
	if !ValidKey(key) {
		return fmt.Errorf("invalid s3 key")
	}
	resp, err := c.client.GetObject(ctx, &s3api.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// LoadAWSConfig is kept for tests; production uses explicit static credentials.
func LoadAWSConfig(cfg Config) (aws.Config, error) {
	return awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
	)
}
