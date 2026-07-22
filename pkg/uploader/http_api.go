package uploader

import (
	"context"
	b64 "encoding/base64"
	"io"
	"net/http"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gin-gonic/gin"
	"github.com/Tuananh165-art/NexusChat/pkg/common"
)

// @Summary Upload files (deprecated)
// @Description Upload files to S3 bucket (deprecated; use presigned urls instead)
// @Tags uploader
// @Accept mpfd
// @param files formData []file true "files to upload" collectionFormat(multi)
// @Produce json
// @param Authorization header string true "channel authorization"
// @Success 201 {object} UploadedFilesPresenter
// @Failure 400 {object} common.ErrResponse
// @Failure 401 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /uploader/upload/files [post]
func (r *HttpServer) UploadFiles(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	if err := c.Request.ParseMultipartForm(r.maxMemory); err != nil {
		r.logger.Error("error parsing multipart form into memory: " + err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		r.logger.Error("parse multipart form error: " + err.Error())
		response(c, http.StatusBadRequest, ErrReceiveFile)
		return
	}
	fileHeaders := form.File["files"]

	var uploadedFiles []UploadedFilePresenter

	for _, fileHeader := range fileHeaders {
		f, err := fileHeader.Open()
		if err != nil {
			r.logger.Error("error opening multipart file header: " + err.Error())
			response(c, http.StatusBadRequest, ErrOpenFile)
			return
		}

		extension := filepath.Ext(fileHeader.Filename)
		newFileName := newObjectKey(channelID, extension)
		if err := r.putFileToS3(c.Request.Context(), r.s3Bucket, newFileName, f); err != nil {
			r.logger.Error("error putting file to S3: " + err.Error())
			response(c, http.StatusInternalServerError, ErrUploadFile)
			return
		}
		uploadedFiles = append(uploadedFiles, UploadedFilePresenter{
			Name: fileHeader.Filename,
			Url:  joinStrs(r.s3PublicEndpoint, "/", r.s3Bucket, "/", newFileName),
		})
	}

	c.JSON(http.StatusCreated, &UploadedFilesPresenter{
		UploadedFiles: uploadedFiles,
	})
}

func (r *HttpServer) putFileToS3(ctx context.Context, bucket, fileName string, f io.Reader) error {
	_, err := r.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(fileName),
		ACL:    types.ObjectCannedACLPublicRead,
		Body:   f,
	})
	if err != nil {
		return err
	}
	return nil
}

// @Summary Proxy upload file
// @Description Upload file through uploader service (bypasses presigned URL DNS issues)
// @Tags uploader
// @Accept mpfd
// @param file formData file true "file to upload"
// @param ext query string true "file extension"
// @Produce json
// @param Authorization header string true "channel authorization"
// @Success 200 {object} PresignedUpload
// @Failure 400 {object} common.ErrResponse
// @Failure 401 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /uploader/upload/proxy [post]
func (r *HttpServer) ProxyUpload(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}

	ext := c.Query("ext")
	objectKey := newObjectKey(channelID, common.Join(".", ext))

	if err := c.Request.ParseMultipartForm(r.maxMemory); err != nil {
		r.logger.Error("error parsing multipart form: " + err.Error())
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}

	file, _, err := c.Request.FormFile("file")
	if err != nil {
		r.logger.Error("error getting file from form: " + err.Error())
		response(c, http.StatusBadRequest, ErrReceiveFile)
		return
	}
	defer file.Close()

	if err := r.putFileToS3(c.Request.Context(), r.s3Bucket, objectKey, file); err != nil {
		r.logger.Error("error uploading file to S3: " + err.Error())
		response(c, http.StatusInternalServerError, ErrUploadFile)
		return
	}

	c.JSON(http.StatusOK, &PresignedUpload{
		ObjectKey: objectKey,
		Url:       "",
	})
}

// @Summary Get presigned upload url
// @Description Get presigned url for uploading a file to S3
// @Tags uploader
// @Produce json
// @Param ext query string true "file extension"
// @param Authorization header string true "channel authorization"
// @Success 200 {object} PresignedUpload
// @Failure 400 {object} common.ErrResponse
// @Failure 401 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /uploader/upload/presigned [get]
func (r *HttpServer) GetPresignedUpload(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	var req GetPresignedUploadRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	objectKey := newObjectKey(channelID, common.Join(".", req.Extension))
	res, err := r.presigner.PutObject(c.Request.Context(), r.s3Bucket, objectKey)
	if err != nil {
		r.logger.Error("get presigned upload url failed: " + err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}

	c.JSON(http.StatusOK, &PresignedUpload{
		ObjectKey: objectKey,
		Url:       res.URL,
	})
}

// @Summary Get presigned download url
// @Description Get presigned url for downloading a file from S3
// @Tags uploader
// @Produce json
// @Param okb64 query string true "base64-encoded object key"
// @param Authorization header string true "channel authorization"
// @Success 200 {object} PresignedDownload
// @Failure 400 {object} common.ErrResponse
// @Failure 401 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /uploader/download/presigned [get]
func (r *HttpServer) GetPresignedDownload(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	var req GetPresignedDownloadRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	objectKeyByte, err := b64.URLEncoding.DecodeString(req.ObjectKeyBase64)
	if err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	objectKey := byteSlice2String(objectKeyByte)
	targetChannelID, err := getChannelIDFromObjectKey(objectKey)
	if err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	if channelID != targetChannelID {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}

	res, err := r.presigner.GetObject(c.Request.Context(), r.s3Bucket, objectKey)
	if err != nil {
		r.logger.Error("get presigned download url failed: " + err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}

	c.JSON(http.StatusOK, &PresignedDownload{res.URL})
}

// @Summary Proxy download file
// @Description Proxy download a file from S3 through the uploader service
// @Tags uploader
// @Produce octet-stream
// @Param okb64 query string true "base64-encoded object key"
// @param Authorization header string true "channel authorization"
// @Success 200 {file} file
// @Failure 400 {object} common.ErrResponse
// @Failure 401 {object} common.ErrResponse
// @Failure 404 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /uploader/download/file [get]
func (r *HttpServer) ProxyDownload(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	var req GetPresignedDownloadRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	objectKeyByte, err := b64.URLEncoding.DecodeString(req.ObjectKeyBase64)
	if err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	objectKey := byteSlice2String(objectKeyByte)
	targetChannelID, err := getChannelIDFromObjectKey(objectKey)
	if err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	if channelID != targetChannelID {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}

	result, err := r.s3Client.GetObject(c.Request.Context(), &s3.GetObjectInput{
		Bucket: aws.String(r.s3Bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		r.logger.Error("proxy download failed: " + err.Error())
		response(c, http.StatusNotFound, common.ErrServer)
		return
	}
	defer result.Body.Close()

	// Detect content type from object metadata
	contentType := "application/octet-stream"
	if result.ContentType != nil {
		contentType = *result.ContentType
	}
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "public, max-age=3600")
	c.Status(http.StatusOK)
	io.Copy(c.Writer, result.Body)
}

// @Summary Initiate chunk upload
// @Description Initiate a multipart upload and return the upload ID
// @Tags uploader
// @Produce json
// @Param ext query string true "file extension"
// @Param filename query string true "original filename"
// @Param total_parts query int true "total number of parts"
// @param Authorization header string true "channel authorization"
// @Success 200 {object} ChunkUploadInitResponse
// @Failure 400 {object} common.ErrResponse
// @Failure 401 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /uploader/upload/chunk/init [post]
func (r *HttpServer) InitChunkUpload(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	ext := c.Query("ext")
	objectKey := newObjectKey(channelID, common.Join(".", ext))
	result, err := r.s3Client.CreateMultipartUpload(c.Request.Context(), &s3.CreateMultipartUploadInput{
		Bucket: aws.String(r.s3Bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		r.logger.Error("init multipart upload failed: " + err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	c.JSON(http.StatusOK, &ChunkUploadInitResponse{
		UploadID:  *result.UploadId,
		ObjectKey: objectKey,
	})
}

// @Summary Get presigned URL for chunk
// @Description Get presigned URL for uploading a specific chunk part
// @Tags uploader
// @Produce json
// @Param object_key query string true "object key"
// @Param upload_id query string true "upload id"
// @Param part_number query int true "part number (1-based)"
// @param Authorization header string true "channel authorization"
// @Success 200 {object} ChunkPresignedResponse
// @Failure 400 {object} common.ErrResponse
// @Failure 401 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /uploader/upload/chunk/presign [get]
func (r *HttpServer) GetChunkPresignedUrl(c *gin.Context) {
	_, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	objectKey := c.Query("object_key")
	uploadID := c.Query("upload_id")
	partNumStr := c.Query("part_number")
	partNumber := 0
	for _, ch := range partNumStr {
		partNumber = partNumber*10 + int(ch-'0')
	}
	if partNumber < 1 {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	request, err := r.presigner.UploadPart(c.Request.Context(), r.s3Bucket, objectKey, uploadID, int32(partNumber))
	if err != nil {
		r.logger.Error("presign upload part failed: " + err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	c.JSON(http.StatusOK, &ChunkPresignedResponse{
		Url: request.URL,
	})
}

// @Summary Complete chunk upload
// @Description Complete a multipart upload
// @Tags uploader
// @Produce json
// @Param object_key query string true "object key"
// @Param upload_id query string true "upload id"
// @param Authorization header string true "channel authorization"
// @Success 200 {object} common.SuccessMessage
// @Failure 400 {object} common.ErrResponse
// @Failure 401 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /uploader/upload/chunk/complete [post]
func (r *HttpServer) CompleteChunkUpload(c *gin.Context) {
	_, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	objectKey := c.Query("object_key")
	uploadID := c.Query("upload_id")
	partsResult, err := r.s3Client.ListParts(c.Request.Context(), &s3.ListPartsInput{
		Bucket:   aws.String(r.s3Bucket),
		Key:      aws.String(objectKey),
		UploadId: aws.String(uploadID),
	})
	if err != nil {
		r.logger.Error("list parts failed: " + err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	var completedParts []types.CompletedPart
	for _, part := range partsResult.Parts {
		completedParts = append(completedParts, types.CompletedPart{
			ETag:       part.ETag,
			PartNumber: part.PartNumber,
		})
	}
	_, err = r.s3Client.CompleteMultipartUpload(c.Request.Context(), &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(r.s3Bucket),
		Key:      aws.String(objectKey),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		r.logger.Error("complete multipart upload failed: " + err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	c.JSON(http.StatusOK, common.SuccessMessage{Message: "ok"})
}

// @Summary Abort chunk upload
// @Description Abort a multipart upload
// @Tags uploader
// @Produce json
// @Param object_key query string true "object key"
// @Param upload_id query string true "upload id"
// @param Authorization header string true "channel authorization"
// @Success 200 {object} common.SuccessMessage
// @Failure 400 {object} common.ErrResponse
// @Failure 401 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /uploader/upload/chunk/abort [delete]
func (r *HttpServer) AbortChunkUpload(c *gin.Context) {
	_, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	objectKey := c.Query("object_key")
	uploadID := c.Query("upload_id")
	_, err := r.s3Client.AbortMultipartUpload(c.Request.Context(), &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(r.s3Bucket),
		Key:      aws.String(objectKey),
		UploadId: aws.String(uploadID),
	})
	if err != nil {
		r.logger.Error("abort multipart upload failed: " + err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	c.JSON(http.StatusOK, common.SuccessMessage{Message: "ok"})
}
