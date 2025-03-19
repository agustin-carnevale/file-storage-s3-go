package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"time"

	"github.com/agustin-carnevale/file-storage-s3-go/internal/auth"
	"github.com/agustin-carnevale/file-storage-s3-go/internal/database"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {

	// videoId param to uuid
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	// Get Token
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	// Get user
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	// Get video from DB
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid video id", err)
		return
	}
	// Verify belongs to same user
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You don't have access to this resource", err)
		return
	}

	fmt.Println("uploading video file for video", videoID, "by user", userID)

	// Get multipart form
	const maxMemory = 10 << 30
	r.ParseMultipartForm(maxMemory)

	// "video" should match the HTML form input name
	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	// Media Type
	contentType := header.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || !(mediaType == "video/mp4") {
		respondWithError(w, http.StatusBadRequest, "Invalid media type", err)
		return
	}

	// Save video to temp file on filesystem
	tmpVideoFile, err := os.CreateTemp("", "tubely-upload-temp.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error creating file", err)
		return
	}
	defer os.Remove(tmpVideoFile.Name())
	defer tmpVideoFile.Close()

	// Copy Contents from request file to tmp file on disk
	_, err = io.Copy(tmpVideoFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error copying file", err)
		return
	}

	// After copy, reset file to start
	tmpVideoFile.Seek(0, io.SeekStart)

	// generate random key for filename
	randomBytes := make([]byte, 32)
	_, err = rand.Read(randomBytes)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error generating filename", err)
		return
	}

	// Check for the video aspect ratio
	aspectRatio, err := getVideoAspectRatio(tmpVideoFile.Name())
	if err != nil {
		fmt.Println("Error getting aspect ratio of the video")
	}
	filePrefix := "other/"
	if aspectRatio == "16:9" {
		filePrefix = "landscape/"
	} else if aspectRatio == "9:16" {
		filePrefix = "portrait/"
	}

	videoFilename := filePrefix + base64.RawURLEncoding.EncodeToString(randomBytes) + ".mp4"

	// Read the file into a byte slice
	videoBytes, err := io.ReadAll(tmpVideoFile)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error with video temp file", err)
		return
	}

	_, err = cfg.s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(videoFilename),
		Body:        bytes.NewReader(videoBytes),
		ContentType: aws.String(mediaType),
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to upload file to s3", err)
		return
	}

	videoUrl := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, videoFilename)

	fmt.Println("VIDEO URL:", videoUrl)

	updatedVideo := database.Video{
		ID:           video.ID,
		CreatedAt:    video.CreatedAt,
		UpdatedAt:    time.Now(),
		ThumbnailURL: video.ThumbnailURL,
		VideoURL:     &videoUrl,
		CreateVideoParams: database.CreateVideoParams{
			Title:       video.Title,
			Description: video.Description,
			UserID:      userID,
		},
	}

	err = cfg.db.UpdateVideo(updatedVideo)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error updating video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, updatedVideo)
}
