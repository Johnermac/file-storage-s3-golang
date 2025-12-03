package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

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

	if mediaType != "image/jpeg" && mediaType != "image/png" {
		respondWithError(w, http.StatusUnauthorized, "Unsupported media type. Only JPEG and PNG are allowed.", err)
		return 
	}

	//fmt.Println("media type:", mediaType)
	
	// 1 - Encoding data to base64
	//encodedString := base64.StdEncoding.EncodeToString(data)
	//thumbnailURL := fmt.Sprintf("data:%s;base64,%s", mediaType, encodedString)
	
	// 2 - Saving file to disk and providing URL
	//thumbFile := fmt.Sprintf("%v.%s", videoID.String(), mediaType[len("image/"):])
	//fileURL := filepath.Join(cfg.assetsRoot, thumbFile)	
	//thumbnailURL := fmt.Sprintf("http://localhost:%v/assets/%v", cfg.port, thumbFile)

	
	// 3 - Saving file to random characters to avoid caching issues
	
	// generate random 32 bytes file path
	key := make([]byte, 32)
	rand.Read(key)

	// encode to base64
	filePath := base64.RawURLEncoding.EncodeToString(key)	
	thumbFile := fmt.Sprintf("%v.%s", filePath, mediaType[len("image/"):])
	fileURL := filepath.Join(cfg.assetsRoot, thumbFile)	
	thumbnailURL := fmt.Sprintf("http://localhost:%v/assets/%v", cfg.port, thumbFile)
	
	createFile, err := os.Create(fileURL)
	if err != nil {
		log.Fatal(err)
	}
	
	defer createFile.Close()	

	if _, err := io.Copy(createFile, file); err != nil {
		log.Fatal(err)
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
