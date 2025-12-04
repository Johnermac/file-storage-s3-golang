package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
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

	fmt.Println("[!] uploading video", videoID, "by user", userID)	
	
	const maxSize = 1 << 30 // 1 GB
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
    http.Error(w, "could not get the video", http.StatusBadRequest)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	// -----------------------------------	

	file, header, err := r.FormFile("video"); 
	if err != nil {
    http.Error(w, "could not read file from form", http.StatusBadRequest)
		return
	}
	defer file.Close()

	mediaType := header.Header.Get("Content-Type")

	mediaCheck, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unsupported media type. Only MP4 is allowed.", err)
		return 
	}	
	if mediaCheck != "video/mp4" {
		respondWithError(w, http.StatusUnauthorized, "file is not an MP4", nil)
		return 
	}	

	// 3 - Saving file to random characters to avoid caching issues
	
	// generate random 32 bytes file path
	key := make([]byte, 32)
	rand.Read(key)

	// encode to base64
	filePath := base64.RawURLEncoding.EncodeToString(key)	
	videoFile := fmt.Sprintf("%v.%s", filePath, "mp4")
	//fileURL := filepath.Join(cfg.assetsRoot, videoFile)	
	videoURL := fmt.Sprintf("https://%v.s3.%v.amazonaws.com/%v", cfg.s3Bucket, cfg.s3Region, videoFile)
	
	createFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not create temp file", err)
		return
	}
	
	defer os.Remove(createFile.Name())	
	defer createFile.Close()

	if _, err := io.Copy(createFile, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not save the video file", err)
		return
	}	

	createFile.Seek(0, io.SeekStart)		

	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &videoFile,
		Body:        createFile,
		ContentType: &mediaCheck,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not upload the video to S3", err)
		return
	}

	video.VideoURL = &videoURL
	fmt.Println("video URL:", videoURL)

	
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
