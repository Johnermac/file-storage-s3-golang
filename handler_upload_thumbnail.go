package main

import (
	"fmt"
	"io"
	"net/http"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
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

	if err := r.ParseMultipartForm(maxMemory); err != nil {
		http.Error(w, "could not parse multipart form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("thumbnail"); 
	if err != nil {
    http.Error(w, "could not read file from form", http.StatusBadRequest)
		return
	}
	defer file.Close()

	mediaType := header.Header.Get("Content-Type")

	data, err := io.ReadAll(file)
	if err != nil {
    http.Error(w, "could not read file", http.StatusBadRequest)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
    http.Error(w, "could not get the video", http.StatusBadRequest)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	videoThumbnails[video.ID] = thumbnail{
		data: data,
		mediaType: mediaType,
	}

	
	thumbnailURL := fmt.Sprintf("http://localhost:%s/api/thumbnails/%s", cfg.port, videoID)
	
	video.ThumbnailURL = &thumbnailURL
	fmt.Println("thumb URL:", thumbnailURL)
	
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not update the video", err)
		return
	}

	video, err = cfg.db.GetVideo(videoID)
	if err != nil {
    http.Error(w, "could not get the video", http.StatusBadRequest)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
