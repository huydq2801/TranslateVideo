package translation

import "testing"

func TestCleanTranslation(t *testing.T) {
	translation, failed := CleanTranslation("Bản dịch: Xin chào! (Ghi chú: câu chào)")
	if failed || translation != "Xin chào!" {
		t.Fatalf("CleanTranslation() = %q, %v; want %q, false", translation, failed, "Xin chào!")
	}
}

func TestCleanTranslationRejectsRemainingChinese(t *testing.T) {
	_, failed := CleanTranslation("Đây là 中文")
	if !failed {
		t.Fatal("CleanTranslation() should flag untranslated CJK text")
	}
}

func TestLevenshteinSimilarity(t *testing.T) {
	sim1 := LevenshteinSimilarity(".Tôi", "Tôi.")
	if sim1 != 1.0 {
		t.Errorf("Expected 1.0 similarity for normalized match, got %f", sim1)
	}

	sim2 := LevenshteinSimilarity("Tôi đi học", "Tôi đi hộc")
	if sim2 < 0.75 {
		t.Errorf("Expected >= 0.75 similarity for typo, got %f", sim2)
	}

	sim3 := LevenshteinSimilarity("Tôi đi học", "Cơm gà chiên")
	if sim3 >= 0.75 {
		t.Errorf("Expected < 0.75 similarity for different text, got %f", sim3)
	}
}

func TestIsCJK(t *testing.T) {
	if !IsCJK("你好") {
		t.Fatal("IsCJK should return true for Chinese text")
	}
	if IsCJK("Hello World") {
		t.Fatal("IsCJK should return false for English text")
	}
}
