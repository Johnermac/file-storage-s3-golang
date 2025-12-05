package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"os"
	"os/exec"

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

	aspectRatio, err := getVideoAspectRatio(createFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not get video aspect ratio", err)
		return
	}
	
	// generate random 32 bytes file path
	key := make([]byte, 32)
	rand.Read(key)

	// encode to base64
	filePath := base64.RawURLEncoding.EncodeToString(key)	

	videoFile := fmt.Sprintf("%v/%v.%s", aspectRatio, filePath, "mp4")
	videoURL := fmt.Sprintf("https://%v.s3.%v.amazonaws.com/%v", cfg.s3Bucket, cfg.s3Region, videoFile)



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

func getVideoAspectRatio(filePath string) (string, error) {
	type FFProbeOutput struct {
			Streams []struct {
					Width  int `json:"width"`
					Height int `json:"height"`
			} `json:"streams"`
	}

	cmd := exec.Command(
			"ffprobe",
			"-v", "error",
			"-print_format", "json",
			"-show_streams",
			filePath,
	)
	
	var out bytes.Buffer
	cmd.Stdout = &out	
	if err := cmd.Run(); err != nil {
		return "", err
	}
	
	var probe FFProbeOutput
	if err := json.Unmarshal(out.Bytes(), &probe); err != nil  {
		return "", err
	}

	if len(probe.Streams) == 0 {
		return "", fmt.Errorf("no streams found in video")
	}

	width := probe.Streams[0].Width
	height := probe.Streams[0].Height

	if width == 0 || height == 0 {
		return "", fmt.Errorf("invalid video dimensions")
	}

	 // "portrait" / "landscape" / "other"
	aspectRatio := detectAspectRatio(width, height)	

	switch aspectRatio {
	case "16:9":
		aspectRatio = "landscape"
	case "9:16":
		aspectRatio = "portrait"
	default:
		aspectRatio = "other"
	}
	
  return aspectRatio, nil  

}

func detectAspectRatio(width, height int) string {
			const tolerance = 0.03 // 3%

			w := float64(width)
			h := float64(height)

			// Compare ratios
			ratio := w / h

			ratio16x9 := 16.0 / 9.0
			ratio9x16 := 9.0 / 16.0

			if isApprox(ratio, ratio16x9, tolerance) {
					return "16:9"
			}

			if isApprox(ratio, ratio9x16, tolerance) {
					return "9:16"
			}

			return "other"
	}

func isApprox(a, b, tolerance float64) bool {
		diff := math.Abs(a - b)
		return diff/b <= tolerance
}