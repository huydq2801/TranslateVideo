package ocr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

var HTTPClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	},
}

func CallPythonOCRWithContext(ctx context.Context, img image.Image, lang string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, img, &jpeg.Options{Quality: 95}); err != nil {
		return "", err
	}

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "bubble.jpg")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, buf); err != nil {
		return "", err
	}
	writer.WriteField("lang", lang)
	writer.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1:8000/ocr", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Success bool   `json:"success"`
		Text    string `json:"text"`
		Error   string `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if !result.Success {
		return "", fmt.Errorf("OCR error: %s", result.Error)
	}

	return strings.TrimSpace(result.Text), nil
}

func CallPythonOCR(img image.Image, lang string) (string, error) {
	return CallPythonOCRWithContext(context.Background(), img, lang)
}

func CallPythonOCRBatchWithContext(ctx context.Context, imgs []image.Image, lang string) ([]string, error) {
	if len(imgs) == 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if len(imgs) == 1 {
		res, err := CallPythonOCRWithContext(ctx, imgs[0], lang)
		if err != nil {
			return nil, err
		}
		return []string{res}, nil
	}

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	for i, img := range imgs {
		buf := new(bytes.Buffer)
		if err := jpeg.Encode(buf, img, &jpeg.Options{Quality: 95}); err != nil {
			return nil, err
		}
		part, err := writer.CreateFormFile("files", fmt.Sprintf("bubble_%d.jpg", i))
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(part, buf); err != nil {
			return nil, err
		}
	}
	writer.WriteField("lang", lang)
	writer.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1:8000/ocr_batch", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := HTTPClient.Do(req)
	if err != nil {
		results := make([]string, len(imgs))
		for i, img := range imgs {
			txt, _ := CallPythonOCRWithContext(ctx, img, lang)
			results[i] = txt
		}
		return results, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		results := make([]string, len(imgs))
		for i, img := range imgs {
			txt, _ := CallPythonOCRWithContext(ctx, img, lang)
			results[i] = txt
		}
		return results, nil
	}

	var batchResp struct {
		Results []struct {
			Success bool   `json:"success"`
			Text    string `json:"text"`
			Error   string `json:"error"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&batchResp); err != nil {
		results := make([]string, len(imgs))
		for i, img := range imgs {
			txt, _ := CallPythonOCRWithContext(ctx, img, lang)
			results[i] = txt
		}
		return results, nil
	}

	results := make([]string, len(imgs))
	for i, item := range batchResp.Results {
		if i < len(results) && item.Success {
			results[i] = strings.TrimSpace(item.Text)
		}
	}
	return results, nil
}

func CallPythonOCRBatch(imgs []image.Image, lang string) ([]string, error) {
	return CallPythonOCRBatchWithContext(context.Background(), imgs, lang)
}

func CallPythonOCRFullFrameBatch(ctx context.Context, paths []string, lang string) (map[string][]OCRFullFrameItem, error) {
	if len(paths) == 0 {
		return map[string][]OCRFullFrameItem{}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	reqBody := struct {
		Paths []string `json:"paths"`
		Lang  string   `json:"lang"`
	}{
		Paths: paths,
		Lang:  lang,
	}

	buf, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1:8000/ocr_full_frame_batch", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OCR FullFrame HTTP status: %d", resp.StatusCode)
	}

	var batchResp OCRFullFrameBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&batchResp); err != nil {
		return nil, err
	}

	resultMap := make(map[string][]OCRFullFrameItem)
	for _, r := range batchResp.Results {
		if r.Success {
			resultMap[r.Path] = r.Items
		}
	}
	return resultMap, nil
}
