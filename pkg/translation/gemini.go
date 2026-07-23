package translation

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"translate-video/pkg/config"
)

var (
	geminiQuotaMutex sync.Mutex
	lastGeminiReq    time.Time
	geminiRateWindow time.Duration = 400 * time.Millisecond
	geminiHTTPClient *http.Client  = &http.Client{
		Timeout: 45 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     90 * time.Second,
		},
	}
)

type GeminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *GeminiInlineData `json:"inlineData,omitempty"`
}

type GeminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type GeminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []GeminiPart `json:"parts"`
}

type GeminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

type GeminiSystemInstruction struct {
	Parts []GeminiPart `json:"parts"`
}

type GeminiGenerateRequest struct {
	Contents          []GeminiContent          `json:"contents"`
	SystemInstruction *GeminiSystemInstruction `json:"systemInstruction,omitempty"`
	SafetySettings    []GeminiSafetySetting    `json:"safetySettings,omitempty"`
}

type GeminiGenerateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func WaitForGeminiQuota(ctx context.Context) error {
	geminiQuotaMutex.Lock()
	now := time.Now()
	elapsed := now.Sub(lastGeminiReq)
	var waitDuration time.Duration
	if elapsed < geminiRateWindow {
		waitDuration = geminiRateWindow - elapsed
	}
	lastGeminiReq = now.Add(waitDuration)
	geminiQuotaMutex.Unlock()

	if waitDuration > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDuration):
		}
	}
	return nil
}

func GeminiRetryDelay(retryAfterHeader string, attempt int) time.Duration {
	if retryAfterHeader != "" {
		if seconds, err := strconv.Atoi(retryAfterHeader); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	fallbacks := []time.Duration{15 * time.Second, 30 * time.Second, 60 * time.Second}
	if attempt < len(fallbacks) {
		return fallbacks[attempt]
	}
	return 60 * time.Second
}

func CallGeminiAPIWithContext(ctx context.Context, apiKey string, prompt string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(osEnvKey())
	}
	if apiKey == "" {
		return "", fmt.Errorf("chưa cấu hình Gemini API Key")
	}

	reqBody := GeminiGenerateRequest{
		Contents: []GeminiContent{
			{
				Role:  "user",
				Parts: []GeminiPart{{Text: prompt}},
			},
		},
		SafetySettings: []GeminiSafetySetting{
			{Category: "HARM_CATEGORY_HARASSMENT", Threshold: "BLOCK_NONE"},
			{Category: "HARM_CATEGORY_HATE_SPEECH", Threshold: "BLOCK_NONE"},
			{Category: "HARM_CATEGORY_SEXUALLY_EXPLICIT", Threshold: "BLOCK_NONE"},
			{Category: "HARM_CATEGORY_DANGEROUS_CONTENT", Threshold: "BLOCK_NONE"},
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	models := []string{"gemini-2.5-flash", "gemini-1.5-flash"}
	var lastErr error

	for attempt := 0; attempt < 3; attempt++ {
		for _, model := range models {
			if err := WaitForGeminiQuota(ctx); err != nil {
				return "", err
			}

			url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)
			req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBytes))
			if err != nil {
				return "", err
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := geminiHTTPClient.Do(req)
			if err != nil {
				lastErr = err
				continue
			}

			if resp.StatusCode == 429 {
				retryAfter := resp.Header.Get("Retry-After")
				resp.Body.Close()
				delay := GeminiRetryDelay(retryAfter, attempt)
				select {
				case <-ctx.Done():
					return "", ctx.Err()
				case <-time.After(delay):
					continue
				}
			}

			respBytes, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				lastErr = err
				continue
			}

			if resp.StatusCode != http.StatusOK {
				lastErr = fmt.Errorf("Gemini API %s error (%d): %s", model, resp.StatusCode, string(respBytes))
				continue
			}

			var geminiResp GeminiGenerateResponse
			if err := json.Unmarshal(respBytes, &geminiResp); err != nil {
				lastErr = err
				continue
			}

			if geminiResp.Error != nil {
				lastErr = fmt.Errorf("Gemini Error: %s", geminiResp.Error.Message)
				continue
			}

			if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
				return geminiResp.Candidates[0].Content.Parts[0].Text, nil
			}
		}
	}

	return "", lastErr
}

func CallGeminiAPI(apiKey string, prompt string) (string, error) {
	return CallGeminiAPIWithContext(context.Background(), apiKey, prompt)
}

func CallGeminiVisionAPI(apiKey string, prompt string, imgBytes []byte) (string, error) {
	return CallGeminiVisionAPIWithContext(context.Background(), apiKey, prompt, imgBytes)
}

func CallGeminiVisionAPIWithContext(ctx context.Context, apiKey string, prompt string, imgBytes []byte) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(osEnvKey())
	}
	if apiKey == "" {
		return "", fmt.Errorf("chưa cấu hình Gemini API Key")
	}

	b64Img := base64.StdEncoding.EncodeToString(imgBytes)

	reqBody := GeminiGenerateRequest{
		Contents: []GeminiContent{
			{
				Role: "user",
				Parts: []GeminiPart{
					{Text: prompt},
					{InlineData: &GeminiInlineData{MimeType: "image/jpeg", Data: b64Img}},
				},
			},
		},
		SafetySettings: []GeminiSafetySetting{
			{Category: "HARM_CATEGORY_HARASSMENT", Threshold: "BLOCK_NONE"},
			{Category: "HARM_CATEGORY_HATE_SPEECH", Threshold: "BLOCK_NONE"},
			{Category: "HARM_CATEGORY_SEXUALLY_EXPLICIT", Threshold: "BLOCK_NONE"},
			{Category: "HARM_CATEGORY_DANGEROUS_CONTENT", Threshold: "BLOCK_NONE"},
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	models := []string{"gemini-2.5-flash", "gemini-1.5-flash"}
	var lastErr error

	for _, model := range models {
		if err := WaitForGeminiQuota(ctx); err != nil {
			return "", err
		}
		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBytes))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := geminiHTTPClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		respBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode == http.StatusOK {
			var geminiResp GeminiGenerateResponse
			if err := json.Unmarshal(respBytes, &geminiResp); err == nil && len(geminiResp.Candidates) > 0 {
				if len(geminiResp.Candidates[0].Content.Parts) > 0 {
					return geminiResp.Candidates[0].Content.Parts[0].Text, nil
				}
			}
		} else {
			lastErr = fmt.Errorf("Vision API HTTP %d: %s", resp.StatusCode, string(respBytes))
		}
	}

	return "", lastErr
}

func CallGeminiVisionBatchSnapAPI(apiKey string, items []config.DialogueItem, imgBytes []byte, imgW, imgH float64) (map[int]config.BBox, error) {
	if len(items) == 0 {
		return map[int]config.BBox{}, nil
	}

	type ItemPrompt struct {
		ID   int    `json:"id"`
		Text string `json:"text"`
	}
	promptItems := make([]ItemPrompt, len(items))
	for i, d := range items {
		promptItems[i] = ItemPrompt{ID: d.ID, Text: d.Original}
	}

	itemsJSON, _ := json.Marshal(promptItems)

	prompt := fmt.Sprintf(`Bạn là mô hình thị giác máy tính AI chuyên phân tích khung hình video truyện tranh.
Dưới đây là khung hình chứa danh sách %d câu thoại:
%s

Nhiệm vụ:
Hãy xác định chính xác vị trí khung bóng thoại (speech bubble) chứa từng câu thoại trên ảnh.
Trả về kết quả dưới dạng JSON Array:
[
  {"id": 1, "box_2d": [ymin, xmin, ymax, xmax]},
  {"id": 2, "box_2d": [ymin, xmin, ymax, xmax]}
]

Trong đó box_2d là tọa độ tỉ lệ 0-1000.
CHỈ trả về duy nhất chuỗi JSON Array. KHÔNG thêm bất kỳ lời giải thích hay markdown codeblock nào.`, len(items), string(itemsJSON))

	respStr, err := CallGeminiVisionAPI(apiKey, prompt, imgBytes)
	if err != nil {
		return nil, err
	}

	rawContent := strings.TrimSpace(respStr)

	type AIBatchItem struct {
		ID    int   `json:"id"`
		Box2D []int `json:"box_2d"`
	}

	var outputs []AIBatchItem
	if err := json.Unmarshal([]byte(rawContent), &outputs); err != nil {
		reArray := regexp.MustCompile(`\[\s*\{.*\}\s*\]`)
		match := reArray.FindString(rawContent)
		if match != "" {
			_ = json.Unmarshal([]byte(match), &outputs)
		}
	}

	resMap := make(map[int]config.BBox)
	if len(outputs) > 0 {
		itemDimMap := make(map[int]config.DialogueItem)
		for _, it := range items {
			itemDimMap[it.ID] = it
		}

		for _, out := range outputs {
			if len(out.Box2D) == 4 {
				if _, ok := itemDimMap[out.ID]; ok {
					ymin, xmin, ymax, xmax := out.Box2D[0], out.Box2D[1], out.Box2D[2], out.Box2D[3]
					if ymax > ymin && xmax > xmin {
						realX := float64(xmin) * imgW / 1000.0
						realY := float64(ymin) * imgH / 1000.0
						realW := float64(xmax-xmin) * imgW / 1000.0
						realH := float64(ymax-ymin) * imgH / 1000.0

						padX := math.Max(4.0, realW*0.02)
						padY := math.Max(4.0, realH*0.02)

						resMap[out.ID] = config.BBox{
							X:      math.Max(0, realX-padX),
							Y:      math.Max(0, realY-padY),
							Width:  realW + 2*padX,
							Height: realH + 2*padY,
						}
					}
				}
			}
		}
	}
	return resMap, nil
}

func osEnvKey() string {
	return ""
}
