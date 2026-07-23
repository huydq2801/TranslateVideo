package translation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
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
	Text string `json:"text,omitempty"`
}

type GeminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []GeminiPart `json:"parts"`
}

type GeminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

type GeminiGenerateRequest struct {
	Contents       []GeminiContent       `json:"contents"`
	SafetySettings []GeminiSafetySetting `json:"safetySettings,omitempty"`
}

type GeminiGenerateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
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

func NormalizeOCRText(text string) string {
	trimmed := strings.TrimSpace(text)
	trimmed = strings.Trim(trimmed, ".,...!?~–—:;\"'「」『』《》（）()【】[] ")
	return strings.TrimSpace(trimmed)
}

func IsCJK(s string) bool {
	for _, r := range s {
		if (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3400 && r <= 0x4DBF) || (r >= 0x3000 && r <= 0x303F) {
			return true
		}
	}
	return false
}

func ContainsSignificantCJK(s string) bool {
	cjkCount := 0
	total := 0
	for _, r := range s {
		if r > 32 {
			total++
			if (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3400 && r <= 0x4DBF) {
				cjkCount++
			}
		}
	}
	return total > 0 && float64(cjkCount)/float64(total) > 0.15
}

func LevenshteinSimilarity(a, b string) float64 {
	sa, sb := NormalizeOCRText(a), NormalizeOCRText(b)
	if sa == sb {
		return 1.0
	}
	ra, rb := []rune(sa), []rune(sb)
	if len(ra) == 0 || len(rb) == 0 {
		return 0.0
	}
	if len(ra) < len(rb) {
		ra, rb = rb, ra
	}

	previous := make([]int, len(rb)+1)
	current := make([]int, len(rb)+1)
	for j := range previous {
		previous[j] = j
	}
	for i, charA := range ra {
		current[0] = i + 1
		for j, charB := range rb {
			cost := 0
			if charA != charB {
				cost = 1
			}
			current[j+1] = minInt(previous[j+1]+1, current[j]+1, previous[j]+cost)
		}
		previous, current = current, previous
	}

	return 1.0 - float64(previous[len(rb)])/float64(len(ra))
}

func minInt(values ...int) int {
	min := values[0]
	for _, value := range values[1:] {
		if value < min {
			min = value
		}
	}
	return min
}

func CleanTranslation(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", true
	}

	reBrackets := regexp.MustCompile(`[\(\[\{（【].*?[\)\]\}）】]`)
	raw = reBrackets.ReplaceAllString(raw, "")

	cutMarkers := []string{
		"(Note", "(注", "(Lưu ý", "\n注:", "。 注:", "Lưu ý:", "Chú thích:", "Ghi chú:",
		"本句", "本词", "改为：", "翻译为", "翻译：", "解释为", "意思是", "可译为",
		"->", "—>", "->", "→",
	}
	for _, m := range cutMarkers {
		if idx := strings.Index(raw, m); idx != -1 {
			raw = strings.TrimSpace(raw[:idx])
		}
	}

	prefixes := []string{"Bản dịch:", "Dịch:", "Nghĩa là:", "Tiếng Việt:"}
	for _, p := range prefixes {
		if strings.HasPrefix(raw, p) {
			raw = strings.TrimSpace(strings.TrimPrefix(raw, p))
		}
	}

	if ContainsSignificantCJK(raw) {
		return "⚠️ Dịch thất bại (chưa dịch xong tiếng Trung)", true
	}

	raw = strings.Trim(raw, "\"'` ")
	return strings.TrimSpace(raw), false
}
