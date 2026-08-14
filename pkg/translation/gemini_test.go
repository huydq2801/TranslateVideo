package translation

import (
	"testing"
)

func TestHeuristicIsTrash(t *testing.T) {
	trashCases := []string{
		"b", "c", "x", "y", "z", "B", "1", "2", "9",
		"a.", " (b) ", "x!", "【b】", "1.", "a1", "x2", "p.3", "v1", "12",
		"你", "是", "看", "等", "他", "我", "买", "卖", "一", "在", "的",
		"bilibili", "http://test.com", "scan", "ch.1",
	}

	validCases := []string{
		"啊", "哦", "嗯", "哈", "哇", "哼", "哎", "滚!", "快!", "走", "救命",
		"你好", "你是谁？", "Anh yêu em", "Comic title text",
		"He", "we", "if", "is", "no", "at", "to", "so", "John wondered if I would accompany him",
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
