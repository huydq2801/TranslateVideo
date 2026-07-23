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

func main() {
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
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/wails-local-video") {
					w.Header().Set("Access-Control-Allow-Origin", "*")
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

					if targetPath != "" {
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
						}
						http.ServeFile(w, r, targetPath)
						return
					}
					http.Error(w, "Không tìm thấy file video: "+cleanPath, http.StatusNotFound)
					return
				}
			}),
		},
		OnStartup: app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		log.Fatalf("Lỗi khởi tạo Wails: %v", err)
	}
}
