package uploader

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tuananh165-art/NexusChat/pkg/common"
	"github.com/Tuananh165-art/NexusChat/pkg/config"
	"github.com/gin-gonic/gin"
)

func TestNewHttpServerUsesPublicEndpointForPresignedURLs(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Uploader: &config.UploaderConfig{},
	}
	cfg.Uploader.S3.Endpoint = "http://minio.minio.svc.cluster.local:9000"
	cfg.Uploader.S3.PublicEndpoint = "http://minio.192.168.109.131.nip.io"
	cfg.Uploader.S3.Region = "us-east-1"
	cfg.Uploader.S3.Bucket = "myfilebucket"
	cfg.Uploader.S3.AccessKey = "labminio"
	cfg.Uploader.S3.SecretKey = "lab-minio-secret"
	cfg.Uploader.S3.PresignLifetimeSecond = 3600

	server := NewHttpServer(
		"uploader",
		common.HttpLog{Logger: slog.Default()},
		cfg,
		gin.New(),
		ChannelUploadRateLimiter{},
	)

	presigned, err := server.presigner.PutObject(context.Background(), cfg.Uploader.S3.Bucket, "42/test.png")
	if err != nil {
		t.Fatalf("presign put object: %v", err)
	}
	if got, want := presigned.URL, "http://minio.192.168.109.131.nip.io/myfilebucket/42/test.png"; !strings.HasPrefix(got, want) {
		t.Fatalf("expected presigned URL to start with %q, got %q", want, got)
	}
	if got := server.s3PublicEndpoint; got != cfg.Uploader.S3.PublicEndpoint {
		t.Fatalf("expected public endpoint %q, got %q", cfg.Uploader.S3.PublicEndpoint, got)
	}
}

func TestNewHttpServerFallsBackToInternalEndpointWhenPublicEndpointMissing(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Uploader: &config.UploaderConfig{},
	}
	cfg.Uploader.S3.Endpoint = "http://minio.minio.svc.cluster.local:9000"
	cfg.Uploader.S3.Region = "us-east-1"
	cfg.Uploader.S3.Bucket = "myfilebucket"
	cfg.Uploader.S3.AccessKey = "labminio"
	cfg.Uploader.S3.SecretKey = "lab-minio-secret"
	cfg.Uploader.S3.PresignLifetimeSecond = 3600

	server := NewHttpServer(
		"uploader",
		common.HttpLog{Logger: slog.Default()},
		cfg,
		gin.New(),
		ChannelUploadRateLimiter{},
	)

	presigned, err := server.presigner.PutObject(context.Background(), cfg.Uploader.S3.Bucket, "42/test.png")
	if err != nil {
		t.Fatalf("presign put object: %v", err)
	}
	if got, want := presigned.URL, "http://minio.minio.svc.cluster.local:9000/myfilebucket/42/test.png"; !strings.HasPrefix(got, want) {
		t.Fatalf("expected presigned URL to start with %q, got %q", want, got)
	}
	if got := server.s3PublicEndpoint; got != cfg.Uploader.S3.Endpoint {
		t.Fatalf("expected fallback public endpoint %q, got %q", cfg.Uploader.S3.Endpoint, got)
	}
}

func TestEnsureBucketExistsCreatesMissingBucket(t *testing.T) {
	t.Parallel()

	headCalls := 0
	createCalls := 0
	fakeS3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			headCalls++
			w.WriteHeader(http.StatusNotFound)
		case http.MethodPut:
			createCalls++
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer fakeS3.Close()

	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Uploader: &config.UploaderConfig{}}
	cfg.Uploader.S3.Endpoint = fakeS3.URL
	cfg.Uploader.S3.PublicEndpoint = fakeS3.URL
	cfg.Uploader.S3.Region = "us-east-1"
	cfg.Uploader.S3.Bucket = "myfilebucket"
	cfg.Uploader.S3.AccessKey = "labminio"
	cfg.Uploader.S3.SecretKey = "lab-minio-secret"
	cfg.Uploader.S3.PresignLifetimeSecond = 3600

	server := NewHttpServer(
		"uploader",
		common.HttpLog{Logger: slog.Default()},
		cfg,
		gin.New(),
		ChannelUploadRateLimiter{},
	)

	if err := server.ensureBucketExists(context.Background()); err != nil {
		t.Fatalf("ensure bucket exists: %v", err)
	}
	if headCalls != 1 {
		t.Fatalf("expected 1 head bucket call, got %d", headCalls)
	}
	if createCalls != 1 {
		t.Fatalf("expected 1 create bucket call, got %d", createCalls)
	}
}

func TestEnsureBucketExistsSkipsCreateWhenBucketAlreadyExists(t *testing.T) {
	t.Parallel()

	headCalls := 0
	createCalls := 0
	fakeS3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			headCalls++
			w.WriteHeader(http.StatusOK)
		case http.MethodPut:
			createCalls++
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer fakeS3.Close()

	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Uploader: &config.UploaderConfig{}}
	cfg.Uploader.S3.Endpoint = fakeS3.URL
	cfg.Uploader.S3.PublicEndpoint = fakeS3.URL
	cfg.Uploader.S3.Region = "us-east-1"
	cfg.Uploader.S3.Bucket = "myfilebucket"
	cfg.Uploader.S3.AccessKey = "labminio"
	cfg.Uploader.S3.SecretKey = "lab-minio-secret"
	cfg.Uploader.S3.PresignLifetimeSecond = 3600

	server := NewHttpServer(
		"uploader",
		common.HttpLog{Logger: slog.Default()},
		cfg,
		gin.New(),
		ChannelUploadRateLimiter{},
	)

	if err := server.ensureBucketExists(context.Background()); err != nil {
		t.Fatalf("ensure bucket exists: %v", err)
	}
	if headCalls != 1 {
		t.Fatalf("expected 1 head bucket call, got %d", headCalls)
	}
	if createCalls != 0 {
		t.Fatalf("expected 0 create bucket calls, got %d", createCalls)
	}
}
