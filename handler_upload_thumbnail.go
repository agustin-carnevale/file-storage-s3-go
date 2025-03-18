package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/agustin-carnevale/file-storage-s3-go/internal/auth"
	"github.com/agustin-carnevale/file-storage-s3-go/internal/database"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory = 10 << 20
	r.ParseMultipartForm(maxMemory)

	// "thumbnail" should match the HTML form input name
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	mediaType := header.Header.Get("Content-Type")
	imageBytes, err := io.ReadAll(file)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to read image", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid video id", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You don't have access to this resource", err)
		return
	}

	// Add to global map
	// videoThumbnails[videoID] = thumbnail{
	// 	data:      imageBytes,
	// 	mediaType: mediaType,
	// }

	// thumbnailUrl := fmt.Sprintf("http://localhost:%s/api/thumbnails/%s", cfg.port, videoID)

	imageBase64 := base64.StdEncoding.EncodeToString(imageBytes)
	thumbnailUrl := fmt.Sprintf("data:%s;base64,%s", mediaType, imageBase64)

	updatedVideo := database.Video{
		ID:           video.ID,
		CreatedAt:    video.CreatedAt,
		UpdatedAt:    time.Now(),
		ThumbnailURL: &thumbnailUrl,
		VideoURL:     video.VideoURL,
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
