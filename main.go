package main

import (
	"embed"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend
var assets embed.FS

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) || err != nil {
		return false
	}
	return !info.IsDir()
}

func startLocalVideoServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/video", func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[LocalVideoServer] Recovered panic while streaming video: %v", rec)
			}
		}()

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Accept-Ranges", "bytes")

		filePath := r.URL.Query().Get("path")
		if filePath == "" {
			http.Error(w, "Vui lòng chọn file video", http.StatusBadRequest)
			return
		}

		cleanPath := filepath.Clean(filePath)
		targetPath := ""
		if fileExists(cleanPath) {
			targetPath = cleanPath
		} else if fileExists(filePath) {
			targetPath = filePath
		}

		if targetPath == "" {
			http.Error(w, "Không tìm thấy file video: "+cleanPath, http.StatusNotFound)
			return
		}

		ext := strings.ToLower(filepath.Ext(targetPath))
		switch ext {
		case ".mp4":
			w.Header().Set("Content-Type", "video/mp4")
		case ".webm":
			w.Header().Set("Content-Type", "video/webm")
		case ".mkv":
			w.Header().Set("Content-Type", "video/x-matroska")
		case ".mov":
			w.Header().Set("Content-Type", "video/quicktime")
		case ".avi":
			w.Header().Set("Content-Type", "video/x-msvideo")
		default:
			w.Header().Set("Content-Type", "application/octet-stream")
		}

		file, err := os.Open(targetPath)
		if err != nil {
			http.Error(w, "Lỗi mở file video: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer file.Close()

		stat, err := file.Stat()
		if err != nil {
			http.Error(w, "Lỗi đọc thông tin file: "+err.Error(), http.StatusInternalServerError)
			return
		}

		http.ServeContent(w, r, stat.Name(), stat.ModTime(), file)
	})

	server := &http.Server{
		Addr:    "127.0.0.1:18989",
		Handler: mux,
	}

	go func() {
		log.Printf("[LocalVideoServer] Listening on http://127.0.0.1:18989...")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[LocalVideoServer] Error: %v", err)
		}
	}()
}

func main() {
	startLocalVideoServer()

	app := NewApp()

	err := wails.Run(&options.App{
		Title:            "Comicify Video Translator",
		Width:            1400,
		Height:           900,
		WindowStartState: options.Maximised,
		MinWidth:         800,
		MinHeight:        600,
		DisableResize:    false,
		Fullscreen:       false,
		Frameless:        false,
		BackgroundColour: &options.RGBA{R: 13, G: 15, B: 23, A: 255},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		log.Fatalf("Lỗi khởi tạo Wails: %v", err)
	}
}
