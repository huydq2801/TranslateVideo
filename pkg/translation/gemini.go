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
	"unicode"

	"translate-video/pkg/config"
)

var (
	geminiQuotaMutex sync.Mutex
	lastGeminiReq    time.Time
	// geminiRateWindow enforces ~14 RPM sustained to stay safely under the 15 RPM free-tier limit.
	// 4250ms gives ~14.1 RPM and absorbs burst spikes from retries and fallback single-calls.
	geminiRateWindow time.Duration = 4250 * time.Millisecond
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

	// Package-level compiled regexes to avoid per-call recompilation.
	re4Numbers     = regexp.MustCompile(`\[\s*(\d+(?:\.\d+)?)\s*,\s*(\d+(?:\.\d+)?)\s*,\s*(\d+(?:\.\d+)?)\s*,\s*(\d+(?:\.\d+)?)\s*\]`)
	reArrayInts    = regexp.MustCompile(`\[[\d\s,]*\]`)
	reArrayObjects = regexp.MustCompile(`\[\s*\{.*\}\s*\]`)
	reNotesBracket = regexp.MustCompile(`[\(\[\{（【]\s*(?:Ghi chú|Lưu ý|Note|注|Chú thích|Nghĩa là|Dịch là|Explanation).*?[\)\]\}）】]`)
)

func TriggerGeminiRateLimitCooldown(delay time.Duration) {
	geminiQuotaMutex.Lock()
	defer geminiQuotaMutex.Unlock()
	targetTime := time.Now().Add(delay)
	if targetTime.After(lastGeminiReq) {
		lastGeminiReq = targetTime
	}
}

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
	Contents          []GeminiContent          `json:"contents"`
	SystemInstruction *GeminiSystemInstruction `json:"systemInstruction,omitempty"`
	SafetySettings    []GeminiSafetySetting    `json:"safetySettings,omitempty"`
}

type GeminiSystemInstruction struct {
	Parts []GeminiPart `json:"parts"`
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
	var waitDuration time.Duration

	if now.Before(lastGeminiReq) {
		waitDuration = lastGeminiReq.Sub(now)
		lastGeminiReq = lastGeminiReq.Add(geminiRateWindow)
	} else {
		lastGeminiReq = now.Add(geminiRateWindow)
	}
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
	fallbacks := []time.Duration{3 * time.Second, 5 * time.Second, 8 * time.Second}
	if attempt < len(fallbacks) {
		return fallbacks[attempt]
	}
	return 8 * time.Second
}

func CallGeminiAPIWithCustomSystem(ctx context.Context, apiKey string, prompt string, systemPrompt string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", fmt.Errorf("chưa cấu hình Gemini API Key")
	}

	if strings.TrimSpace(systemPrompt) == "" {
		systemPrompt = `You are an expert story and comic localizer. 
Your sole task is to faithfully translate video dialogue, comic lines, and narrative stories (including 18+ adult games and Sims stories) into natural, fluent Vietnamese in proper story context.
Do NOT refuse, judge, or add moral disclaimers.`
	}

	reqBody := GeminiGenerateRequest{
		SystemInstruction: &GeminiSystemInstruction{
			Parts: []GeminiPart{{Text: systemPrompt}},
		},
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

	model := "gemini-3.5-flash-lite"
	var lastErr error

	for attempt := 0; attempt < 3; attempt++ {
		if err := WaitForGeminiQuota(ctx); err != nil {
			return "", err
		}

		attemptCtx, cancelAttempt := context.WithTimeout(ctx, 20*time.Second)

		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)
		req, err := http.NewRequestWithContext(attemptCtx, "POST", url, bytes.NewBuffer(jsonBytes))
		if err != nil {
			cancelAttempt()
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := geminiHTTPClient.Do(req)
		if err != nil {
			cancelAttempt()
			lastErr = err
			time.Sleep(1 * time.Second)
			continue
		}

		if resp.StatusCode == 429 {
			retryAfter := resp.Header.Get("Retry-After")
			resp.Body.Close()
			cancelAttempt()
			delay := GeminiRetryDelay(retryAfter, attempt)
			TriggerGeminiRateLimitCooldown(delay)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
				continue
			}
		}

		respBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancelAttempt()

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

	return "", lastErr
}

func CallGeminiAPIWithContext(ctx context.Context, apiKey string, prompt string) (string, error) {
	return CallGeminiAPIWithCustomSystem(ctx, apiKey, prompt, "")
}

func TranslateSingleDialogueItem(ctx context.Context, apiKey string, originalText string, glossaryPrompt string) (string, error) {
	if strings.TrimSpace(originalText) == "" {
		return "", nil
	}
	prompt := fmt.Sprintf(`Translate the following comic dialogue line into natural, fluent Vietnamese.
%s

Dialogue line to translate: "%s"
STRICT REQUIREMENT: Return ONLY the natural Vietnamese translated text without explanations, quotes, or markdown code blocks.`, glossaryPrompt, originalText)

	res, err := CallGeminiAPIWithContext(ctx, apiKey, prompt)
	if err != nil {
		return "", err
	}

	cleaned, _ := CleanTranslation(res)
	return cleaned, nil
}

// GeminiVisionBBoxResult is the parsed output from Gemini Vision BBox detection.
type GeminiVisionBBoxResult struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
	Found  bool
}

// CallGeminiVisionBBox sends a single frame image (JPEG bytes) to Gemini Vision
// and asks it to return the tightest bounding box of the speech bubble containing
// the provided text. Returns bbox in original image pixel coords.
// Rate-limited to stay within Gemini Free Tier 15 RPM.
func CallGeminiVisionBBox(ctx context.Context, apiKey string, imageBytes []byte, dialogueText string, imageW, imageH int) (GeminiVisionBBoxResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return GeminiVisionBBoxResult{}, fmt.Errorf("chưa cấu hình Gemini API Key")
	}

	b64 := base64.StdEncoding.EncodeToString(imageBytes)

	prompt := fmt.Sprintf(
		`You are an expert AI Vision model for comic/video spatial grounding. The image contains dialogue text: "%s".
Task: Locate the speech bubble or dialogue frame containing this text.
CRITICAL REQUIREMENT: Return a 2D bounding box [ymin, xmin, ymax, xmax] normalized to 0-1000 scale. Make sure the width [xmin, xmax] is wide enough to cover the dialogue, but keep the vertical height [ymin, ymax] TIGHT around the text and speech bubble without adding excessive top/bottom margin.
Return strictly raw JSON format without explanations:
{"box_2d": [ymin, xmin, ymax, xmax]}`,
		dialogueText,
	)

	type GeminiInlinePart struct {
		Text       string `json:"text,omitempty"`
		InlineData *struct {
			MimeType string `json:"mimeType"`
			Data     string `json:"data"`
		} `json:"inlineData,omitempty"`
	}
	type GeminiVisionContent struct {
		Role  string             `json:"role,omitempty"`
		Parts []GeminiInlinePart `json:"parts"`
	}
	type GeminiVisionRequest struct {
		Contents       []GeminiVisionContent `json:"contents"`
		SafetySettings []GeminiSafetySetting `json:"safetySettings,omitempty"`
	}

	reqBody := GeminiVisionRequest{
		Contents: []GeminiVisionContent{
			{
				Role: "user",
				Parts: []GeminiInlinePart{
					{
						InlineData: &struct {
							MimeType string `json:"mimeType"`
							Data     string `json:"data"`
						}{MimeType: "image/jpeg", Data: b64},
					},
					{Text: prompt},
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
		return GeminiVisionBBoxResult{}, err
	}

	model := "gemini-3.5-flash-lite"
	var lastErr error

	for attempt := 0; attempt < 4; attempt++ {
		if err := WaitForGeminiQuota(ctx); err != nil {
			return GeminiVisionBBoxResult{}, err
		}

		attemptCtx, cancelAttempt := context.WithTimeout(ctx, 12*time.Second)

		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)
		req, err := http.NewRequestWithContext(attemptCtx, "POST", url, bytes.NewBuffer(jsonBytes))
		if err != nil {
			cancelAttempt()
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := geminiHTTPClient.Do(req)
		if err != nil {
			cancelAttempt()
			lastErr = err
			continue
		}

		if resp.StatusCode == 429 {
			resp.Body.Close()
			cancelAttempt()
			// Cooldown rồi retry — KHÔNG return ngay, để vòng lặp thử lại
			cooldown := GeminiRetryDelay(resp.Header.Get("Retry-After"), attempt)
			TriggerGeminiRateLimitCooldown(cooldown)
			lastErr = fmt.Errorf("Gemini Vision model %s bị Rate Limit (429), đang chờ %v", model, cooldown)
			continue
		}

		respBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancelAttempt()

		if err != nil || resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("Gemini Vision model %s error (%d)", model, resp.StatusCode)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		var geminiResp GeminiGenerateResponse
		if err := json.Unmarshal(respBytes, &geminiResp); err != nil || len(geminiResp.Candidates) == 0 {
			lastErr = fmt.Errorf("không parse được kết quả Gemini Vision")
			continue
		}

		rawText := strings.TrimSpace(geminiResp.Candidates[0].Content.Parts[0].Text)

		// Try 1: Regex match for 4 normalized numbers in brackets [ymin, xmin, ymax, xmax]
		if matches := re4Numbers.FindStringSubmatch(rawText); len(matches) == 5 {
			ymin, _ := strconv.ParseFloat(matches[1], 64)
			xmin, _ := strconv.ParseFloat(matches[2], 64)
			ymax, _ := strconv.ParseFloat(matches[3], 64)
			xmax, _ := strconv.ParseFloat(matches[4], 64)

			if ymax > ymin && xmax > xmin {
				x := math.Max(0, (xmin/1000.0)*float64(imageW))
				y := math.Max(0, (ymin/1000.0)*float64(imageH))
				w := math.Max(15, ((xmax-xmin)/1000.0)*float64(imageW))
				h := math.Max(15, ((ymax-ymin)/1000.0)*float64(imageH))

				// Add tight coverage margin (1.5% extra padding) to tightly cover speech bubble edges
				padX := math.Max(2.0, w*0.015)
				padY := math.Max(2.0, h*0.015)

				finalX := math.Max(0, x-padX)
				finalY := math.Max(0, y-padY)
				finalW := math.Min(float64(imageW)-finalX, w+2*padX)
				finalH := math.Min(float64(imageH)-finalY, h+2*padY)

				return GeminiVisionBBoxResult{
					X: finalX, Y: finalY, Width: finalW, Height: finalH, Found: true,
				}, nil
			}
		}

		// Extract JSON object if present
		if idx := strings.Index(rawText, "{"); idx >= 0 {
			rawText = rawText[idx:]
		}
		if idx := strings.LastIndex(rawText, "}"); idx >= 0 {
			rawText = rawText[:idx+1]
		}

		// Try 2: Direct pixel {"x":..., "y":..., "w":..., "h":...}
		var parsedPixel struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
			W float64 `json:"w"`
			H float64 `json:"h"`
		}
		if err := json.Unmarshal([]byte(rawText), &parsedPixel); err == nil && parsedPixel.W > 0 && parsedPixel.H > 0 {
			padX := math.Max(2.0, parsedPixel.W*0.015)
			padY := math.Max(2.0, parsedPixel.H*0.015)
			finalX := math.Max(0, parsedPixel.X-padX)
			finalY := math.Max(0, parsedPixel.Y-padY)
			finalW := math.Min(float64(imageW)-finalX, parsedPixel.W+2*padX)
			finalH := math.Min(float64(imageH)-finalY, parsedPixel.H+2*padY)
			return GeminiVisionBBoxResult{
				X: finalX, Y: finalY,
				Width: finalW, Height: finalH, Found: true,
			}, nil
		}

		lastErr = fmt.Errorf("Gemini Vision trả về JSON không hợp lệ: %s", rawText)
	}

	if lastErr != nil {
		return GeminiVisionBBoxResult{}, lastErr
	}
	return GeminiVisionBBoxResult{}, fmt.Errorf("Gemini Vision: hết số lần thử")
}

var validSingleCJKRunes = map[rune]bool{
	// Interjections & Exclamations
	'啊': true, '哦': true, '嗯': true, '哈': true, '哎': true, '喂': true, '嗨': true, '哼': true,
	'呸': true, '嘘': true, '吧': true, '呢': true, '吗': true, '啦': true, '呀': true, '哇': true,
	'么': true, '诶': true, '呃': true, '哟': true, '喔': true, '嘻': true, '赫': true, '啧': true,
	'嗷': true, '咩': true, '喵': true, '汪': true, '吼': true,
	// Onomatopoeia / Sound Effects
	'嘎': true, '咕': true, '扑': true, '啪': true, '砰': true, '咚': true, '嗖': true, '嗤': true,
	'铿': true, '锵': true, '唰': true, '轰': true, '咔': true, '嗒': true,
	// One-word Commands / Imperatives
	'滚': true, '走': true, '停': true, '救': true, '逃': true, '快': true, '慢': true, '杀': true,
	'斩': true, '破': true, '冲': true, '灭': true, '变': true, '降': true, '跑': true, '打': true,
}

var valid2LetterEnglishWords = map[string]bool{
	"he": true, "we": true, "if": true, "is": true, "no": true, "at": true,
	"to": true, "so": true, "me": true, "my": true, "up": true, "on": true,
	"in": true, "it": true, "do": true, "go": true, "am": true, "an": true,
	"as": true, "by": true, "or": true, "us": true, "hi": true, "ok": true,
	"oh": true, "ah": true, "ha": true, "um": true, "eh": true,
}

func HeuristicIsTrash(text string) bool {
	t := strings.TrimSpace(text)
	if len(t) == 0 {
		return true
	}

	norm := NormalizeOCRText(t)
	if len(norm) == 0 {
		return true
	}

	runes := []rune(norm)
	lower := strings.ToLower(norm)

	// Single character check: standalone 1-rune string
	if len(runes) == 1 {
		r := runes[0]
		if unicode.Is(unicode.Han, r) {
			if !validSingleCJKRunes[r] {
				return true
			}
		} else {
			// Single Latin letters, numbers, or symbols standing alone without context are OCR artifacts
			return true
		}
	}

	// 2-character check: pure ASCII letters/digits/symbols without any CJK character
	if len(runes) == 2 && !IsCJK(norm) {
		if !valid2LetterEnglishWords[lower] {
			return true
		}
	}

	// Check for URL watermarks and CJK platform watermarks
	urlWatermarks := []string{
		"http://", "https://", "www.", ".com", ".cn", ".net", ".org",
		"bilibili", "kuaikan", "iqiyi", "tencent", "douyin", "tiktok",
		"qq.com", "weibo", "关注", "公众号", "扫码", "免费看", "全集",
		"未删减", "更新", "汉化", "嵌字", "责编", "策划", "监制", "美工",
		"出品", "禁止转载", "严禁转载", "独家", "首发",
	}
	for _, kw := range urlWatermarks {
		if strings.Contains(lower, kw) {
			return true
		}
	}

	// Flag scan-group watermarks ONLY when the text is short (≤10 runes)
	if len(runes) <= 10 {
		shortWatermarks := []string{"scan", "translate", "group", "discord", "mangadex", "manhua", "manga", "page", "chapter", "ch.", "p."}
		for _, kw := range shortWatermarks {
			if strings.Contains(lower, kw) {
				return true
			}
		}
	}

	hasLetterOrHan := false
	for _, r := range runes {
		if unicode.IsLetter(r) || unicode.Is(unicode.Han, r) {
			hasLetterOrHan = true
			break
		}
	}
	if !hasLetterOrHan {
		return true
	}

	return false
}

func FilterTrashDialogues(ctx context.Context, apiKey string, items []config.DialogueItem, onProgress func(curr, total int, status string)) ([]config.DialogueItem, error) {
	if len(items) == 0 {
		return items, nil
	}

	if onProgress != nil {
		onProgress(10, 100, "🧹 [1/2] Đang lọc ký tự rác / watermark theo quy tắc nội cục...")
	}

	// Step 1: Local Heuristic Filter (Instant)
	for i := range items {
		if HeuristicIsTrash(items[i].Original) {
			items[i].IsTrash = true
		}
	}

	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		if onProgress != nil {
			onProgress(100, 100, "✅ Hoàn tất lọc rác thoại theo quy tắc nội cục!")
		}
		return items, nil
	}

	// Step 2: AI Gemini Filter — batch 40 items at a time to avoid token overflow
	type DialogueSample struct {
		ID   int    `json:"id"`
		Text string `json:"text"`
	}

	var activeSamples []DialogueSample
	for _, it := range items {
		if !it.IsTrash && it.Original != "" {
			activeSamples = append(activeSamples, DialogueSample{ID: it.ID, Text: it.Original})
		}
	}

	if len(activeSamples) == 0 {
		if onProgress != nil {
			onProgress(100, 100, "✅ Hoàn tất lọc rác thoại!")
		}
		return items, nil
	}

	sysPrompt := `You are an expert AI assistant analyzing OCR items extracted from comic/manga videos.
Your primary job is to filter out garbage, single random letters/numbers, isolated non-exclamation character fragments, website watermarks, and OCR artifacts, returning a JSON array of trash item IDs.`

	const batchSize = 40
	total := len(activeSamples)

	for batchStart := 0; batchStart < total; batchStart += batchSize {
		select {
		case <-ctx.Done():
			return items, ctx.Err()
		default:
		}

		batchEnd := batchStart + batchSize
		if batchEnd > total {
			batchEnd = total
		}
		batch := activeSamples[batchStart:batchEnd]

		processed := batchEnd
		if onProgress != nil {
			onProgress(10+processed*80/total, 100,
				fmt.Sprintf("🤖 [2/2] Đang gửi Gemini AI phân tích %d/%d thoại & phát hiện rác...", processed, total))
		}

		jsonBytes, _ := json.Marshal(batch)

		prompt := fmt.Sprintf(`Below is a list of OCR dialogue items recognized from a comic/manga video:
%s

Task: Identify ALL TRASH / NOISE / OCR ARTIFACT items that should be removed/hidden.

MUST FLAG AS TRASH (Include their IDs in the returned JSON array):
1. Single standalone letters, numbers, or random ASCII characters (e.g., "a", "b", "c", "x", "y", "z", "A", "1", "2", "a1", "x2", "p.3", "v1").
2. Single isolated Chinese/Japanese/Korean characters or broken sentence fragments that are NOT valid comic sound effects or exclamations (e.g., isolated words like "一", "了", "的", "在", "是", "个", "之", "与", "也", "就", "中", "到", "以", "看", "你", "他").
3. OCR garbled characters, misrecognized icon symbols, or random punctuation with no context.
4. Website watermarks, domains, platform names, or scan group credits (e.g., bilibili, kuaikan, discord, mangadex, scan group).
5. Chapter markers, page numbers, or timestamps (e.g. "P.3", "第1页", "01:23").

KEEP THESE (DO NOT FLAG AS TRASH):
- Natural comic exclamations or sound effects (e.g., "啊", "哦", "嗯", "哈", "哇", "哼", "哎", "滚!", "快!").
- Coherent, meaningful dialogue sentences or character names.

Return strictly a raw JSON Array of integer IDs of trash items, e.g. [1, 5, 12]. If none found, return: []
Do NOT include explanations or markdown formatting.`, string(jsonBytes))

		rawResp, err := CallGeminiAPIWithCustomSystem(ctx, apiKey, prompt, sysPrompt)
		if err != nil {
			// Non-fatal: skip this batch, keep going
			continue
		}

		rawResp = strings.TrimSpace(rawResp)
		if idx := strings.Index(rawResp, "```"); idx != -1 {
			rawResp = rawResp[idx+3:]
			if strings.HasPrefix(rawResp, "json") {
				rawResp = rawResp[4:]
			}
			if endIdx := strings.Index(rawResp, "```"); endIdx != -1 {
				rawResp = rawResp[:endIdx]
			}
		}
		rawResp = strings.TrimSpace(rawResp)
		if match := reArrayInts.FindString(rawResp); match != "" {
			rawResp = match
		}

		var trashIDs []int
		if errJson := json.Unmarshal([]byte(rawResp), &trashIDs); errJson == nil && len(trashIDs) > 0 {
			trashMap := make(map[int]bool)
			for _, id := range trashIDs {
				trashMap[id] = true
			}
			for i := range items {
				if trashMap[items[i].ID] {
					items[i].IsTrash = true
				}
			}
		}
	}

	if onProgress != nil {
		onProgress(100, 100, "✅ Hoàn tất lọc rác thoại!")
	}
	return items, nil
}

func AnalyzeStoryContextAndGlossary(ctx context.Context, apiKey string, items []config.DialogueItem, onProgress func(curr, total int, status string)) (config.TranslationContextConfig, error) {
	var result config.TranslationContextConfig
	if len(items) == 0 {
		return result, fmt.Errorf("chưa có danh sách thoại để phân tích")
	}

	if onProgress != nil {
		onProgress(30, 100, "🧠 Đang gửi Gemini AI đọc kịch bản & phân tích ngữ cảnh...")
	}

	var sb strings.Builder
	active := 0
	for _, it := range items {
		if it.Original != "" && !it.IsTrash {
			sb.WriteString(fmt.Sprintf("%d. %s\n", it.ID, it.Original))
			active++
			if active >= 50 {
				break
			}
		}
	}

	prompt := fmt.Sprintf(`You are a professional comic localization editor.
Below are consecutive dialogue lines extracted from a video:
%s

Tasks:
1. Analyze the overall story context to determine main characters, genre, and their proper Vietnamese pronouns/relationship terms (e.g. Male Lead -> Female Lead: Anh / Em; Enemies: Ta / Ngươi; Friends: Tớ / Cậu).
2. Propose character pronoun pairs and a Vietnamese translation glossary for character names/terms.
3. Formulate a clear, direct Vietnamese pronoun rule (pronounRules) for maintaining consistency throughout translation.

Return strictly a raw JSON Object in this format:
{
  "presetStyle": "ngon_tinh",
  "pronounRules": "Chỉ định rõ xưng hô từng cặp nhân vật...",
  "characterPairs": [
    {"source": "Nam chính -> Nữ chính", "target": "Anh - Em"}
  ],
  "glossaryEntries": [
    {"source": "tên_tiếng_trung", "target": "Tên_Tiếng_Việt"}
  ]
}
Return ONLY raw JSON without explanations or markdown fence blocks.`, sb.String())

	rawResp, err := CallGeminiAPIWithContext(ctx, apiKey, prompt)
	if err != nil {
		return result, err
	}

	rawResp = strings.TrimSpace(rawResp)

	// Try parsing JSON Object
	if startIdx := strings.Index(rawResp, "{"); startIdx != -1 {
		if endIdx := strings.LastIndex(rawResp, "}"); endIdx != -1 && endIdx > startIdx {
			jsonStr := rawResp[startIdx : endIdx+1]
			if errJson := json.Unmarshal([]byte(jsonStr), &result); errJson == nil && (len(result.CharacterPairs) > 0 || len(result.GlossaryEntries) > 0 || result.PronounRules != "") {
				return result, nil
			}
		}
	}

	// Fallback parsing JSON Array if Gemini returns old array format
	if match := reArrayObjects.FindString(rawResp); match != "" {
		var entries []config.GlossaryEntry
		if errArr := json.Unmarshal([]byte(match), &entries); errArr == nil {
			result.GlossaryEntries = entries
		}
	}

	if onProgress != nil {
		onProgress(100, 100, "✅ Hoàn tất phân tích kịch bản!")
	}
	return result, nil
}

func BulkReplacePronouns(dialogues []config.DialogueItem, replacements []config.GlossaryEntry) []config.DialogueItem {
	if len(dialogues) == 0 || len(replacements) == 0 {
		return dialogues
	}
	for i := range dialogues {
		if dialogues[i].Translated == "" {
			continue
		}
		for _, r := range replacements {
			src := strings.TrimSpace(r.Source)
			tgt := strings.TrimSpace(r.Target)
			if src == "" || tgt == "" {
				continue
			}
			dialogues[i].Translated = strings.ReplaceAll(dialogues[i].Translated, src, tgt)
		}
	}
	return dialogues
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

	// Remove explicit note cut markers first
	cutMarkers := []string{
		"(Note", "(注", "(Lưu ý", "\n注:", "。 注:", "Lưu ý:", "Chú thích:", "Ghi chú:",
		"本句", "本词", "改为：", "翻译为", "翻译：", "解释为", "意思是", "可译为",
		"->", "—>", "→",
	}
	for _, m := range cutMarkers {
		if idx := strings.Index(raw, m); idx != -1 {
			raw = strings.TrimSpace(raw[:idx])
		}
	}

	// Clean specific explanation/note brackets like (Ghi chú...), [Note...] while preserving actual parenthesized dialogue
	raw = reNotesBracket.ReplaceAllString(raw, "")

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
	if strings.TrimSpace(raw) == "" {
		return "", true
	}
	return strings.TrimSpace(raw), false
}

type BatchTranslationItem struct {
	ID       int    `json:"id"`
	Original string `json:"original"`
}

func TranslateDialoguesBatch(ctx context.Context, apiKey string, dialogues []config.DialogueItem, glossaryPrompt string, onProgress func(curr, total int, status string)) ([]config.DialogueItem, error) {
	if len(dialogues) == 0 {
		return dialogues, nil
	}

	var activeIndices []int
	for i, d := range dialogues {
		if d.Original != "" && !d.IsTrash {
			activeIndices = append(activeIndices, i)
		}
	}

	totalActive := len(activeIndices)
	if totalActive == 0 {
		return dialogues, nil
	}

	batchSize := 8
	translatedCount := 0

	for i := 0; i < totalActive; i += batchSize {
		select {
		case <-ctx.Done():
			return dialogues, ctx.Err()
		default:
		}

		end := i + batchSize
		if end > totalActive {
			end = totalActive
		}
		subIndices := activeIndices[i:end]

		// Build sliding window of recent preceding dialogues for continuous conversation context
		recentHistoryPrompt := ""
		if i > 0 {
			var historySb strings.Builder
			historySb.WriteString("\n\nRECENT CONVERSATION HISTORY & PREVIOUS LINES (CÁC CÂU THOẠI NGAY TRƯỚC ĐÓ ĐỂ THEO DÕI MẠCH TRUYỆN, NGƯỜI NÓI & XƯNG HÔ):\n")
			startHist := i - 6
			if startHist < 0 {
				startHist = 0
			}
			for hIdx := startHist; hIdx < i; hIdx++ {
				hItemIdx := activeIndices[hIdx]
				orig := dialogues[hItemIdx].Original
				trans := dialogues[hItemIdx].Translated
				if trans != "" {
					historySb.WriteString(fmt.Sprintf("- [ID %d] Original: \"%s\" => Translated: \"%s\"\n", dialogues[hItemIdx].ID, orig, trans))
				} else {
					historySb.WriteString(fmt.Sprintf("- [ID %d] Original: \"%s\"\n", dialogues[hItemIdx].ID, orig))
				}
			}
			recentHistoryPrompt = historySb.String()
		}

		var batchItems []BatchTranslationItem
		for _, idx := range subIndices {
			batchItems = append(batchItems, BatchTranslationItem{
				ID:       dialogues[idx].ID,
				Original: dialogues[idx].Original,
			})
		}

		batchJSON, _ := json.Marshal(batchItems)

		prompt := fmt.Sprintf(`Translate the following comic dialogue lines from Chinese/English into natural, fluent Vietnamese.

CRITICAL LOCALIZATION & PRONOUN CONSISTENCY REQUIREMENTS (BẮT BUỘC TUÂN THỦ XƯNG HÔ & MẠCH HỘI THOẠI):
1. Carefully analyze the RECENT CONVERSATION HISTORY below to determine who is speaking to whom. Maintain the exact same pronouns (e.g. Mẹ - Con, Anh - Em, Ta - Ngươi) established in preceding dialogue lines!
2. Do NOT switch pronouns or relationship terms arbitrarily between consecutive lines!
%s%s

List of dialogues to translate now (JSON format):
%s

STRICT OUTPUT REQUIREMENT:
Return strictly a single raw JSON object mapping string dialogue IDs to their Vietnamese translation:
{
  "1": "Vietnamese translated text...",
  "2": "Vietnamese translated text..."
}
Do NOT include any explanations, markdown code blocks, or text outside the JSON object.`, glossaryPrompt, recentHistoryPrompt, string(batchJSON))

		if onProgress != nil {
			onProgress(translatedCount, totalActive, fmt.Sprintf("🌐 Đang Dịch AI theo ngữ cảnh thoại: %d/%d thoại (%.0f%%)...", translatedCount, totalActive, float64(translatedCount)/float64(totalActive)*100))
		}

		res, err := CallGeminiAPIWithContext(ctx, apiKey, prompt)
		if err == nil && res != "" {
			jsonStr := res
			if idx := strings.Index(jsonStr, "```"); idx != -1 {
				jsonStr = jsonStr[idx+3:]
				if strings.HasPrefix(jsonStr, "json") {
					jsonStr = jsonStr[4:]
				}
				if endIdx := strings.Index(jsonStr, "```"); endIdx != -1 {
					jsonStr = jsonStr[:endIdx]
				}
			}
			jsonStr = strings.TrimSpace(jsonStr)
			if startIdx := strings.Index(jsonStr, "{"); startIdx != -1 {
				if endIdx := strings.LastIndex(jsonStr, "}"); endIdx != -1 && endIdx > startIdx {
					jsonStr = jsonStr[startIdx : endIdx+1]
				}
			}

			var resultMap map[string]string
			if errMap := json.Unmarshal([]byte(jsonStr), &resultMap); errMap == nil && len(resultMap) > 0 {
				for _, idx := range subIndices {
					idStr := strconv.Itoa(dialogues[idx].ID)
					if transText, ok := resultMap[idStr]; ok && strings.TrimSpace(transText) != "" {
						cleaned, failed := CleanTranslation(transText)
						dialogues[idx].Translated = cleaned
						dialogues[idx].TranslationFailed = failed
					} else {
						singlePrompt := fmt.Sprintf(`Translate the following dialogue line into natural Vietnamese, maintaining exact pronoun and context consistency with previous lines.
%s%s

Dialogue line to translate: "%s"
Return ONLY the Vietnamese translated text without explanations.`, glossaryPrompt, recentHistoryPrompt, dialogues[idx].Original)
						singleRes, singleErr := CallGeminiAPIWithContext(ctx, apiKey, singlePrompt)
						if singleErr == nil {
							cleaned, failed := CleanTranslation(singleRes)
							dialogues[idx].Translated = cleaned
							dialogues[idx].TranslationFailed = failed
						}
					}
				}
			} else {
				for _, idx := range subIndices {
					singlePrompt := fmt.Sprintf(`Translate the following dialogue line into natural Vietnamese, maintaining exact pronoun and context consistency with previous lines.
%s%s

Dialogue line to translate: "%s"
Return ONLY the Vietnamese translated text without explanations.`, glossaryPrompt, recentHistoryPrompt, dialogues[idx].Original)
					singleRes, singleErr := CallGeminiAPIWithContext(ctx, apiKey, singlePrompt)
					if singleErr == nil {
						cleaned, failed := CleanTranslation(singleRes)
						dialogues[idx].Translated = cleaned
						dialogues[idx].TranslationFailed = failed
					}
				}
			}
		}

		translatedCount += len(subIndices)
	}

	return dialogues, nil
}

type ScriptPolishItem struct {
	ID       int    `json:"id"`
	Original string `json:"original"`
	Draft    string `json:"draft_vietnamese"`
}

func RefineFullScriptCoherence(ctx context.Context, apiKey string, dialogues []config.DialogueItem, glossaryPrompt string, onProgress func(curr, total int, status string)) ([]config.DialogueItem, error) {
	if len(dialogues) == 0 {
		return dialogues, nil
	}

	var activeIndices []int
	for i, d := range dialogues {
		if d.Original != "" && d.Translated != "" && !d.IsTrash {
			activeIndices = append(activeIndices, i)
		}
	}

	totalActive := len(activeIndices)
	if totalActive == 0 {
		return dialogues, nil
	}

	batchSize := 12
	polishedCount := 0

	for i := 0; i < totalActive; i += batchSize {
		select {
		case <-ctx.Done():
			return dialogues, ctx.Err()
		default:
		}

		end := i + batchSize
		if end > totalActive {
			end = totalActive
		}
		subIndices := activeIndices[i:end]

		prevContextSnippet := ""
		if i > 0 {
			var sbPrev strings.Builder
			sbPrev.WriteString("\n\nMẠCH HỘI THOẠI VỪA DIỄN RA TRƯỚC ĐÓ (DÙNG ĐỂ THEO DÕI NỐI TIẾP VĂN PHONG & XƯNG HÔ):\n")
			startPrev := i - 4
			if startPrev < 0 {
				startPrev = 0
			}
			for pIdx := startPrev; pIdx < i; pIdx++ {
				idx := activeIndices[pIdx]
				sbPrev.WriteString(fmt.Sprintf("- [ID %d] Original: \"%s\" => Draft: \"%s\"\n", dialogues[idx].ID, dialogues[idx].Original, dialogues[idx].Translated))
			}
			prevContextSnippet = sbPrev.String()
		}

		var itemsToPolish []ScriptPolishItem
		for _, idx := range subIndices {
			itemsToPolish = append(itemsToPolish, ScriptPolishItem{
				ID:       dialogues[idx].ID,
				Original: dialogues[idx].Original,
				Draft:    dialogues[idx].Translated,
			})
		}

		itemsJSON, _ := json.Marshal(itemsToPolish)

		prompt := fmt.Sprintf(`You are a senior comic localization editor and proofreader.
Your task is to RE-READ the following draft Vietnamese translation of a comic sequence and POLISH/RE-TRANSLATE it to ensure maximum naturalness, emotional resonance, and seamless flow.

CRITICAL EDITORIAL REQUIREMENTS (BẮT BUỘC TRAU CHUỐT MẠCH HỘI THOẠI):
1. Read through the full continuous exchange. Re-phrase any awkward, stiff, or machine-translated sentences into fluent, natural Vietnamese dialogue fitting a published comic.
2. Ensure character pronouns and speech tone (Anh - Em, Tớ - Cậu, Ta - Ngươi, Mẹ - Con, v.v.) are 100%% CONSISTENT and SEAMLESS across consecutive speaking turns.
3. Preserve original comic tone, emotions, and parenthesized thought bubbles.
%s%s

Draft dialogues to review & polish (JSON format):
%s

STRICT OUTPUT REQUIREMENT:
Return strictly a single raw JSON object mapping string dialogue IDs to their polished Vietnamese translation:
{
  "1": "Vietnamese polished text...",
  "2": "Vietnamese polished text..."
}
Do NOT include any explanations, markdown code blocks, or text outside the JSON object.`, glossaryPrompt, prevContextSnippet, string(itemsJSON))

		if onProgress != nil {
			onProgress(polishedCount, totalActive, fmt.Sprintf("✨ AI Đang đọc lại toàn bộ kịch bản & trau chuốt mượt mà: %d/%d thoại (%.0f%%)...", polishedCount, totalActive, float64(polishedCount)/float64(totalActive)*100))
		}

		res, err := CallGeminiAPIWithContext(ctx, apiKey, prompt)
		if err == nil && res != "" {
			jsonStr := res
			if idx := strings.Index(jsonStr, "```"); idx != -1 {
				jsonStr = jsonStr[idx+3:]
				if strings.HasPrefix(jsonStr, "json") {
					jsonStr = jsonStr[4:]
				}
				if endIdx := strings.Index(jsonStr, "```"); endIdx != -1 {
					jsonStr = jsonStr[:endIdx]
				}
			}
			jsonStr = strings.TrimSpace(jsonStr)
			if startIdx := strings.Index(jsonStr, "{"); startIdx != -1 {
				if endIdx := strings.LastIndex(jsonStr, "}"); endIdx != -1 && endIdx > startIdx {
					jsonStr = jsonStr[startIdx : endIdx+1]
				}
			}

			var resultMap map[string]string
			if errMap := json.Unmarshal([]byte(jsonStr), &resultMap); errMap == nil && len(resultMap) > 0 {
				for _, idx := range subIndices {
					idStr := strconv.Itoa(dialogues[idx].ID)
					if polishedText, ok := resultMap[idStr]; ok && strings.TrimSpace(polishedText) != "" {
						cleaned, failed := CleanTranslation(polishedText)
						if !failed && cleaned != "" {
							dialogues[idx].Translated = cleaned
						}
					}
				}
			}
		}

		polishedCount += len(subIndices)
	}

	return dialogues, nil
}
