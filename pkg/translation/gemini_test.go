package translation

import (
	"testing"
	"translate-video/pkg/config"
)

func TestHeuristicIsTrash(t *testing.T) {
	trashCases := []string{
		"b", "c", "x", "y", "z", "B", "1", "2", "9",
		"a.", " (b) ", "x!", "【b】", "1.", "a1", "x2", "p.3", "v1", "12",
		"你", "是", "看", "等", "他", "我", "买", "卖", "一", "在", "的",
		"bilibili", "http://test.com", "https://discord.gg/abc", "scan", "ch.1", "p.12",
		"29:30", "00:15", "01:23:45", "12:00.5", "\"29:30\"",
	}

	validCases := []string{
		"啊", "哦", "嗯", "哈", "哇", "哼", "哎", "滚!", "快!", "走", "救命",
		"你好", "你是谁？", "Anh yêu em", "Comic title text",
		"He", "we", "if", "is", "no", "at", "to", "so", "John wondered if I would accompany him",
		"What are you doing here?", "I am so addict to this.",
	}

	for _, tc := range trashCases {
		if !HeuristicIsTrash(tc) {
			t.Errorf("Expected '%s' to be recognized as TRASH, but HeuristicIsTrash returned false", tc)
		}
	}

	for _, vc := range validCases {
		if HeuristicIsTrash(vc) {
			t.Errorf("Expected '%s' to be recognized as VALID, but HeuristicIsTrash returned true", vc)
		}
	}
}

func TestNormalizeOCRText(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"  Hello World!  ", "Hello World"},
		{"「你好」...", "你好"},
		{"【Notice】: Important!!", "Notice】: Important"},
		{"   ", ""},
		{"— Cẩn thận đấy! —", "Cẩn thận đấy"},
	}

	for _, c := range cases {
		actual := NormalizeOCRText(c.input)
		if actual != c.expected {
			t.Errorf("NormalizeOCRText(%q) = %q; expected %q", c.input, actual, c.expected)
		}
	}
}

func TestIsCJK(t *testing.T) {
	if !IsCJK("你好世界") {
		t.Errorf("Expected '你好世界' to be detected as CJK")
	}
	if !IsCJK("Hello 世界") {
		t.Errorf("Expected 'Hello 世界' to contain CJK")
	}
	if IsCJK("Hello World") {
		t.Errorf("Expected 'Hello World' not to be CJK")
	}
	if IsCJK("12345 !?#") {
		t.Errorf("Expected ASCII symbols not to be CJK")
	}
}

func TestContainsSignificantCJK(t *testing.T) {
	if !ContainsSignificantCJK("这是一段测试中文文本") {
		t.Errorf("Expected Chinese text to have significant CJK")
	}
	if ContainsSignificantCJK("This is pure English with no Chinese") {
		t.Errorf("Expected pure English not to have significant CJK")
	}
}

func TestLevenshteinSimilarity(t *testing.T) {
	// Exact matches
	if sim := LevenshteinSimilarity("Hello", "Hello"); sim != 1.0 {
		t.Errorf("Expected exact match to be 1.0, got %f", sim)
	}

	// High similarity
	simHigh := LevenshteinSimilarity("What are you doing", "What are you doin")
	if simHigh < 0.85 {
		t.Errorf("Expected high similarity > 0.85, got %f", simHigh)
	}

	// Completely different
	simLow := LevenshteinSimilarity("Apple", "Elephant")
	if simLow > 0.40 {
		t.Errorf("Expected low similarity < 0.40, got %f", simLow)
	}
}

func TestCleanTranslation(t *testing.T) {
	cases := []struct {
		input          string
		expectedClean  string
		expectedFailed bool
	}{
		{"Bản dịch: Xin chào bạn", "Xin chào bạn", false},
		{"Dịch: \"Tôi ổn mà!\"", "Tôi ổn mà!", false},
		{"Em đi đâu đấy? (Lưu ý: em là cách xưng hô thân mật)", "Em đi đâu đấy?", false},
		{"Anh yêu em. (Note: romantic line)", "Anh yêu em.", false},
		{"这是一段没有翻译完的中文", "⚠️ Dịch thất bại (chưa dịch xong tiếng Trung)", true},
		{"   ", "", true},
	}

	for _, c := range cases {
		clean, failed := CleanTranslation(c.input)
		if clean != c.expectedClean || failed != c.expectedFailed {
			t.Errorf("CleanTranslation(%q) = (%q, %v); expected (%q, %v)", c.input, clean, failed, c.expectedClean, c.expectedFailed)
		}
	}
}

func TestBulkReplacePronouns(t *testing.T) {
	dialogues := []config.DialogueItem{
		{ID: 1, Translated: "Anh thích em từ lâu rồi."},
		{ID: 2, Translated: "Em cũng thích anh."},
		{ID: 3, Translated: ""},
	}

	replacements := []config.GlossaryEntry{
		{Source: "Anh", Target: "Tớ"},
		{Source: "em", Target: "cậu"},
		{Source: "Em", Target: "Tớ"},
		{Source: "anh", Target: "cậu"},
	}

	result := BulkReplacePronouns(dialogues, replacements)

	if result[0].Translated != "Tớ thích cậu từ lâu rồi." {
		t.Errorf("Dialogue 1 replacement failed: %q", result[0].Translated)
	}
	if result[1].Translated != "Tớ cũng thích cậu." {
		t.Errorf("Dialogue 2 replacement failed: %q", result[1].Translated)
	}
}
