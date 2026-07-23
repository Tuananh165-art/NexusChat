package uploader

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	"log/slog"

	"github.com/Tuananh165-art/NexusChat/pkg/common"
	"github.com/Tuananh165-art/NexusChat/pkg/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	metrics "github.com/slok/go-http-metrics/metrics/prometheus"
	prommiddleware "github.com/slok/go-http-metrics/middleware"
	ginmiddleware "github.com/slok/go-http-metrics/middleware/gin"

	doc "github.com/Tuananh165-art/NexusChat/docs/uploader"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

type ChannelUploadRateLimiter struct {
	*common.RateLimiter
}

func NewChannelUploadRateLimiter(rc redis.UniversalClient, config *config.Config) ChannelUploadRateLimiter {
	return ChannelUploadRateLimiter{
		common.NewRateLimiter(
			rc,
			config.Uploader.RateLimit.ChannelUpload.Rps,
			config.Uploader.RateLimit.ChannelUpload.Burst,
			time.Duration(config.Redis.ExpirationHour)*time.Hour,
		),
	}
}

type HttpServer struct {
	name                     string
	logger                   common.HttpLog
	svr                      *gin.Engine
	s3PublicEndpoint         string
	s3Bucket                 string
	maxMemory                int64
	s3Client                 *s3.Client
	uploader                 *manager.Uploader
	presigner                *Presigner
	httpPort                 string
	httpServer               *http.Server
	channelUploadRateLimiter ChannelUploadRateLimiter
	serveSwag                bool
}

func NewGinServer(name string, logger common.HttpLog, config *config.Config) *gin.Engine {
	initJWT(config)
	svr := gin.New()
	svr.Use(gin.Recovery())
	svr.Use(common.CorsMiddleware())
	svr.Use(common.LoggingMiddleware(logger))
	svr.Use(common.LimitBodySize(config.Uploader.Http.Server.MaxBodyByte))

	mdlw := prommiddleware.New(prommiddleware.Config{
		Recorder: metrics.NewRecorder(metrics.Config{
			Prefix: name,
		}),
	})
	svr.Use(ginmiddleware.Handler("", mdlw))
	return svr
}

func initJWT(config *config.Config) {
	common.JwtSecret = config.Chat.JWT.Secret
	common.JwtExpirationSecond = config.Chat.JWT.ExpirationSecond
}

func NewHttpServer(name string, logger common.HttpLog, config *config.Config, svr *gin.Engine, channelUploadRateLimiter ChannelUploadRateLimiter) *HttpServer {
	s3Endpoint := config.Uploader.S3.Endpoint
	s3PublicEndpoint := config.Uploader.S3.PublicEndpoint
	if s3PublicEndpoint == "" {
		s3PublicEndpoint = s3Endpoint
	}
	s3Bucket := config.Uploader.S3.Bucket
	creds := credentials.NewStaticCredentialsProvider(config.Uploader.S3.AccessKey, config.Uploader.S3.SecretKey, "")
	awsConfig := aws.Config{
		Credentials:      creds,
		Region:           config.Uploader.S3.Region,
		RetryMaxAttempts: 3,
	}
	s3Client := s3.NewFromConfig(awsConfig, func(o *s3.Options) {
		o.UsePathStyle = true
		o.BaseEndpoint = aws.String(s3Endpoint)
	})
	presignClient := s3.NewPresignClient(s3.NewFromConfig(awsConfig, func(o *s3.Options) {
		o.UsePathStyle = true
		o.BaseEndpoint = aws.String(s3PublicEndpoint)
	}))

	return &HttpServer{
		name:                     name,
		logger:                   logger,
		svr:                      svr,
		s3PublicEndpoint:         s3PublicEndpoint,
		s3Bucket:                 s3Bucket,
		maxMemory:                config.Uploader.Http.Server.MaxMemoryByte,
		s3Client:                 s3Client,
		uploader:                 manager.NewUploader(s3Client),
		presigner:                &Presigner{presignClient, config.Uploader.S3.PresignLifetimeSecond},
		httpPort:                 config.Uploader.Http.Server.Port,
		channelUploadRateLimiter: channelUploadRateLimiter,
		serveSwag:                config.Uploader.Http.Server.Swag,
	}
}

func (r *HttpServer) ChannelUploadRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
		if !ok {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		allow, err := r.channelUploadRateLimiter.Allow(c.Request.Context(), strconv.FormatUint(channelID, 10))
		if err != nil {
			r.logger.Error(err.Error())
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		if !allow {
			c.AbortWithStatus(http.StatusTooManyRequests)
			return
		}
		c.Next()
	}
}

// @title           Uploader Service Swagger API
// @version         2.0
// @description     Uploader service API

// @contact.name   Ming Hsu
// @contact.email  Tuananh165-art@gmail.com

// @BasePath  /api
func (r *HttpServer) RegisterRoutes() {
	uploaderGroup := r.svr.Group("/api/uploader")
	{
		uploadGroup := uploaderGroup.Group("/upload")
		uploadGroup.Use(common.JWTForwardAuth())
		uploadGroup.Use(r.ChannelUploadRateLimit())
		{
			uploadGroup.POST("/files", r.UploadFiles)
			uploadGroup.GET("/presigned", r.GetPresignedUpload)
			uploadGroup.POST("/proxy", r.ProxyUpload)
			chunkGroup := uploadGroup.Group("/chunk")
			{
				chunkGroup.POST("/init", r.InitChunkUpload)
				chunkGroup.GET("/presign", r.GetChunkPresignedUrl)
				chunkGroup.POST("/complete", r.CompleteChunkUpload)
				chunkGroup.DELETE("/abort", r.AbortChunkUpload)
			}
		}
		downloadGroup := uploaderGroup.Group("/download")
		downloadGroup.Use(common.JWTForwardAuth())
		{
			downloadGroup.GET("/presigned", r.GetPresignedDownload)
			downloadGroup.GET("/file", r.ProxyDownload)
		}
	}
	if r.serveSwag {
		uploaderGroup.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.InstanceName(doc.SwaggerInfouploader.InfoInstanceName)))
	}
}

func (r *HttpServer) ensureBucketExists(ctx context.Context) error {
	_, err := r.s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(r.s3Bucket),
	})
	if err == nil {
		return nil
	}
	if !isMissingBucketError(err) {
		return err
	}
	_, err = r.s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(r.s3Bucket),
	})
	if err != nil && !isBucketAlreadyOwnedError(err) {
		return err
	}
	return nil
}

func isMissingBucketError(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.ErrorCode() == "NoSuchBucket" || apiErr.ErrorCode() == "NotFound"
}

func isBucketAlreadyOwnedError(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.ErrorCode() == "BucketAlreadyOwnedByYou" || apiErr.ErrorCode() == "BucketAlreadyExists"
}

func (r *HttpServer) Run() {
	if err := r.ensureBucketExists(context.Background()); err != nil {
		r.logger.Error("ensure S3 bucket exists failed: " + err.Error())
		os.Exit(1)
	}
	go func() {
		addr := ":" + r.httpPort
		r.httpServer = &http.Server{
			Addr:    addr,
			Handler: common.NewOtelHttpHandler(r.svr, r.name+"_http"),
		}
		r.logger.Info("http server listening", slog.String("addr", addr))
		err := r.httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			r.logger.Error(err.Error())
			os.Exit(1)
		}
	}()
}
func (r *HttpServer) GracefulStop(ctx context.Context) error {
	return r.httpServer.Shutdown(ctx)
}

func response(c *gin.Context, httpCode int, err error) {
	message := err.Error()
	c.JSON(httpCode, common.ErrResponse{
		Message: message,
	})
}
