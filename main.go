package main

import (
	"embed"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

//go:embed views/index.html
var indexHTML string

//go:embed views/progress.html
var progressHTML string

//go:embed views/*
var static embed.FS

type FileProgress struct {
	Filename string
	Size     int64
	Progress int64
	Message  string
	Done     bool
	DoneTime time.Time
}

func main() {

	var file_progress sync.Map

	http.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, indexHTML)
	})

	http.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
			return
		}
		for {
			var data string = ""
			file_progress.Range(func(k, v interface{}) bool {
				filename := k.(string)
				progress := v.(FileProgress)
				p := float64(progress.Progress) / float64(progress.Size) * 100
				s := fmt.Sprintf("%s %.2f%%", filename, p)
				s = strings.ReplaceAll(progressHTML, "{text}", s)
				s = strings.ReplaceAll(s, "33", fmt.Sprintf("%.0f", p))
				data += s
				if progress.Done && time.Since(progress.DoneTime) > 2*time.Second {
					file_progress.Delete(filename)
				}
				return true
			})
			if len(data) == 0 {
				data = "No uploads in progress"
			}

			lines := strings.Split(data, "\n")
			for _, line := range lines {
				_, err := fmt.Fprintf(w, "data: %s\n", line)
				if err != nil {
					return
				}
			}
			fmt.Fprint(w, "\n")
			flusher.Flush()
			//fmt.Println(time.Now())
			time.Sleep(time.Second)
		}
	})

	http.HandleFunc("POST /upload", func(w http.ResponseWriter, r *http.Request) {
		in_file, file_header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Failed to get file from form", http.StatusBadRequest)
			return
		}
		defer in_file.Close()
		out_file, err := os.Create("./" + file_header.Filename)
		if err != nil {
			fmt.Println("Error creating output file:", err)
			http.Error(w, "Failed to create output file", http.StatusInternalServerError)
			return
		}
		defer out_file.Close()

		progress := FileProgress{
			Filename: file_header.Filename,
			Size:     file_header.Size,
			Progress: 0,
			Message:  "",
			Done:     false,
			DoneTime: time.Now(),
		}
		file_progress.Store(progress.Filename, progress)
		buffer := make([]byte, 1024*10) // 10KB buffer
		for {
			// uncomment for slow mode
			//time.Sleep(time.Millisecond)
			n, err := in_file.Read(buffer)
			if n > 0 {
				progress.Progress += int64(n)
				progress.Message = "Uploading"
				n, err = out_file.Write(buffer[:n])
				if err != nil {
					fmt.Println("Error writing to file:", err)
					progress.Message = err.Error()
					progress.Done = true
					progress.DoneTime = time.Now()
					break
				}
			}
			if err != nil {
				progress.Message = err.Error()
				progress.Done = true
				progress.DoneTime = time.Now()
				break
			}
			if n == 0 {
				progress.Message = "Upload complete"
				progress.Done = true
				progress.DoneTime = time.Now()
				break
			}
			file_progress.Store(progress.Filename, progress)
		}
		// progress.Message = "Done"
		// progress.Done = true
		// progress.DoneTime = time.Now()
		file_progress.Store(progress.Filename, progress)
	})

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		panic(err)
	}
}
