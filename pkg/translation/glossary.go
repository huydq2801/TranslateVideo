package translation

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"translate-video/pkg/config"
)

type GlossaryEntry struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

func SuggestGlossary(ctx context.Context, apiKey string, dialogues []config.DialogueItem) ([]GlossaryEntry, error) {
	if len(dialogues) == 0 {
		return nil, fmt.Errorf("chưa có dữ liệu thoại để tạo từ điển")
	}

	samples := make([]string, 0, len(dialogues))
	seen := make(map[string]bool)

	for _, d := range dialogues {
		norm := NormalizeOCRText(d.Original)
		if norm == "" || seen[norm] {
			continue
		}
		seen[norm] = true
		samples = append(samples, d.Original)
		if len(samples) >= 30 {
			break
		}
	}

	if len(samples) == 0 {
		return nil, fmt.Errorf("không tìm thấy mẫu thoại hợp lệ")
	}

	prompt := fmt.Sprintf(`Bạn là chuyên gia dịch thuật truyện tranh/video.
Dưới đây là các câu thoại gốc xuất hiện trong video:
%s

Nhiệm vụ:
Hãy tìm các tên riêng (nhân vật, danh xưng, địa danh) xuất hiện trong các thoại trên và đề xuất cách dịch chuẩn tiếng Việt tự nhiên nhất.
Trả về duy nhất JSON Array, tối đa 12 phần tử, theo dạng:
[
  {"source": "từ gốc trong OCR", "target": "cách gọi tiếng Việt"}
]
Không thêm bất kỳ lời giải thích hay markdown codeblock nào.`, strings.Join(samples, "\n"))

	rawResp, err := CallGeminiAPIWithContext(ctx, apiKey, prompt)
	if err != nil {
		return nil, err
	}

	return validateGlossarySuggestions(rawResp, samples), nil
}

func validateGlossarySuggestions(rawJSON string, samples []string) []GlossaryEntry {
	rawJSON = strings.TrimSpace(rawJSON)
	reArray := regexp.MustCompile(`\[\s*\{.*\}\s*\]`)
	if match := reArray.FindString(rawJSON); match != "" {
		rawJSON = match
	}

	var parsed []GlossaryEntry
	if err := json.Unmarshal([]byte(rawJSON), &parsed); err != nil {
		return nil
	}

	fullText := strings.Join(samples, " ")
	var valid []GlossaryEntry
	seen := make(map[string]bool)

	for _, entry := range parsed {
		src := strings.TrimSpace(entry.Source)
		tgt := strings.TrimSpace(entry.Target)
		if src == "" || tgt == "" || seen[src] {
			continue
		}
		if strings.Contains(fullText, src) {
			seen[src] = true
			valid = append(valid, GlossaryEntry{Source: src, Target: tgt})
		}
	}

	return valid
}

func GlossaryPrompt(entries []GlossaryEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\nBẢNG TỪ ĐIỂN TÊN RIÊNG & BẮT BUỘC TUÂN THỦ:\n")
	for _, e := range entries {
		if e.Source != "" && e.Target != "" {
			sb.WriteString(fmt.Sprintf("- \"%s\" => \"%s\"\n", e.Source, e.Target))
		}
	}
	return sb.String()
}

func ParseGlossary(raw string) []GlossaryEntry {
	lines := strings.Split(raw, "\n")
	var entries []GlossaryEntry
	seen := make(map[string]bool)

	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		parts := strings.Split(l, "=>")
		if len(parts) == 2 {
			src := strings.TrimSpace(parts[0])
			tgt := strings.TrimSpace(parts[1])
			if src != "" && tgt != "" && !seen[src] {
				seen[src] = true
				entries = append(entries, GlossaryEntry{Source: src, Target: tgt})
			}
		}
	}

	return entries
}
