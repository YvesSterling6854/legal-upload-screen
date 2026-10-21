package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"example.com/legal-upload-screen/internal/legalflow"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	infrai := legalflow.NewInfraiClient(key, nil)
	pipeline := legalflow.NewPipeline(infrai, infrai)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /submissions", submissionHandler(pipeline))
	log.Println("legal upload screen listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func submissionHandler(pipeline *legalflow.Pipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(12 << 20); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart request"})
			return
		}
		file, header, err := r.FormFile("image")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "image is required"})
			return
		}
		defer file.Close()
		image, err := io.ReadAll(io.LimitReader(file, 10<<20))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not read image"})
			return
		}
		mediaType := header.Header.Get("Content-Type")
		if mediaType == "" {
			mediaType = http.DetectContentType(image)
		}
		decision, err := pipeline.Screen(r.Context(), legalflow.Submission{
			MatterID: strings.TrimSpace(r.FormValue("matter_id")), Stage: legalflow.Stage(r.FormValue("stage")),
			Caption: strings.TrimSpace(r.FormValue("caption")), Filename: header.Filename, MediaType: mediaType, Image: image,
		})
		if err != nil {
			writePipelineError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, decision)
	}
}

func writePipelineError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	var apiErr *legalflow.InfraiError
	if errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 {
		status = apiErr.Status
	} else if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "stage must") {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
