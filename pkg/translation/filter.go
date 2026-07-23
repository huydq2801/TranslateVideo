package translation

import (
	"regexp"
	"strings"
	"unicode"
)

func NormalizeText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	punctCutset := " \t\r\n.!?,:-;\"'()[]{}~…‽"
	text = strings.Trim(text, punctCutset)

	var sb strings.Builder
	inSpace := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			if !inSpace {
				sb.WriteRune(' ')
				inSpace = true
			}
		} else {
			sb.WriteRune(r)
			inSpace = false
		}
	}
	return strings.Trim(sb.String(), punctCutset)
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

func TextSimilarityRatio(a, b string) float64 {
	sa, sb := NormalizeText(a), NormalizeText(b)
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

func IsSimilarText(a, b string) bool {
	return TextSimilarityRatio(a, b) >= 0.80
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

	if len(raw) > 250 {
		if idx := strings.IndexAny(raw, ".\n"); idx > 0 {
			raw = strings.TrimSpace(raw[:idx+1])
		}
	}

	refusalMarkers := []string{
		"i cannot", "i am sorry", "i'm sorry", "cannot fulfill",
		"tôi không thể", "xin lỗi", "tôi là mô hình", "không thể dịch",
	}
	lowerRaw := strings.ToLower(raw)
	for _, m := range refusalMarkers {
		if strings.Contains(lowerRaw, m) {
			return "⚠️ Dịch thất bại (nội dung bị chặn, cần sửa tay)", true
		}
	}

	if ContainsSignificantCJK(raw) {
		return "⚠️ Dịch thất bại (chưa dịch xong tiếng Trung)", true
	}

	raw = strings.Trim(raw, "\"'` ")
	return strings.TrimSpace(raw), false
}
