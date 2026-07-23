package config

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type ScanCache struct {
	VideoPath string         `json:"videoPath"`
	Config    ScanConfig     `json:"config"`
	Completed int            `json:"completed"`
	Total     int            `json:"total"`
	Dialogues []DialogueItem `json:"dialogues"`
}

func GetCachePath(videoPath string) string {
	info, err := os.Stat(videoPath)
	if err != nil {
		return filepath.Join(os.TempDir(), "scan_cache_default.json")
	}
	rawStr := fmt.Sprintf("%s_%d_%d", videoPath, info.Size(), info.ModTime().Unix())
	hash := fmt.Sprintf("%x", md5.Sum([]byte(rawStr)))
	return filepath.Join(os.TempDir(), fmt.Sprintf("scan_cache_%s.json", hash))
}

func SaveCache(cachePath string, config ScanConfig, dialogues []DialogueItem, completed, total int) {
	cache := ScanCache{
		VideoPath: config.VideoPath,
		Config:    config,
		Completed: completed,
		Total:     total,
		Dialogues: dialogues,
	}
	bytes, err := json.Marshal(cache)
	if err == nil {
		_ = os.WriteFile(cachePath, bytes, 0644)
	}
}

func LoadCache(cachePath string) (*ScanCache, error) {
	bytes, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}
	var cache ScanCache
	if err := json.Unmarshal(bytes, &cache); err != nil {
		return nil, err
	}
	return &cache, nil
}
