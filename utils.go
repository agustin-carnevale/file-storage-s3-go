package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"time"

	"github.com/agustin-carnevale/file-storage-s3-go/internal/database"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func getVideoAspectRatio(filePath string) (string, error) {

	ffprobe := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)

	var output bytes.Buffer
	ffprobe.Stdout = &output

	err := ffprobe.Run()
	if err != nil {
		fmt.Println("Error running ffprobe:", err)
		return "", err
	}

	// Define a struct to parse only the relevant fields
	type Stream struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}
	type FFProbeOutput struct {
		Streams []Stream `json:"streams"`
	}

	// Parse JSON output
	var result FFProbeOutput
	err = json.Unmarshal(output.Bytes(), &result)
	if err != nil {
		fmt.Println("Error parsing JSON:", err)
		return "", err
	}

	if len(result.Streams) == 0 {
		fmt.Println("Invalid ffprobe output", err)
		return "", err
	}

	width := result.Streams[0].Width
	height := result.Streams[0].Height

	aspectRatio := determineAspectRatio(width, height)

	return aspectRatio, nil
}

func determineAspectRatio(width, height int) string {
	ratio := float64(width) / float64(height)

	const tolerance = 0.05 // allow small floating-point deviations

	if math.Abs(ratio-(16.0/9.0)) < tolerance {
		return "16:9"
	} else if math.Abs(ratio-(9.0/16.0)) < tolerance {
		return "9:16"
	}
	return "other"
}

func processVideoForFastStart(filePath string) (string, error) {

	outputPath := filePath + ".processing"

	ffmpeg := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputPath)

	err := ffmpeg.Run()
	if err != nil {
		return "", err
	}

	return outputPath, nil
}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {

	s3PresignedClient := s3.NewPresignClient(s3Client)
	presignedReq, err := s3PresignedClient.PresignGetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	}, s3.WithPresignExpires(expireTime))

	if err != nil {
		return "", err
	}

	return presignedReq.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {

	if video.VideoURL == nil {
		return video, nil
	}

	urlParts := strings.Split(*video.VideoURL, ",")
	if len(urlParts) < 2 {
		return database.Video{}, errors.New("invalid videoURL")
	}

	bucket := urlParts[0]
	key := urlParts[1]
	expiresIn := 15 * time.Minute

	presignedUrl, err := generatePresignedURL(cfg.s3Client, bucket, key, expiresIn)
	if err != nil {
		return database.Video{}, err
	}

	video.VideoURL = &presignedUrl

	return video, nil

	// return database.Video{
	// 	ID:           video.ID,
	// 	CreatedAt:    video.CreatedAt,
	// 	UpdatedAt:    video.UpdatedAt,
	// 	ThumbnailURL: video.ThumbnailURL,
	// 	VideoURL:     &presignedUrl,
	// 	CreateVideoParams: database.CreateVideoParams{
	// 		Title:       video.Title,
	// 		Description: video.Description,
	// 		UserID:      video.UserID,
	// 	},
	// }, nil
}
