package s3

import (
	"context"
	"os"
	"testing"
)

func TestIntegrationMinIO(t *testing.T) {
	if os.Getenv("MINIO_INTEGRATION") != "1" {
		t.Skip("set MINIO_INTEGRATION=1 to run")
	}
	c := Default()
	if c == nil {
		t.Fatal("s3 client not configured")
	}
	ctx := context.Background()
	if err := c.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	objs, err := c.ListObjects(ctx, c.Config().Bucket, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	t.Logf("listed %d objects", len(objs))
}
