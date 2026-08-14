import unittest
import os
import cv2
import numpy as np
from python_ocr.server import _clean_english_text, _process_single, _process_batch

class TestPythonOCRServer(unittest.TestCase):
    def test_clean_english_text(self):
        # 1. Punctuation spacing
        self.assertEqual(_clean_english_text("Hello,world"), "Hello, world")
        self.assertEqual(_clean_english_text("Stop.Go!Wait?Yes:No;End"), "Stop. Go! Wait? Yes: No; End")
        
        # 2. Empty string
        self.assertEqual(_clean_english_text(""), "")
        self.assertEqual(_clean_english_text(None), None)
        
        # 3. Normal sentence
        normal = "What are you doing today?"
        self.assertEqual(_clean_english_text(normal), normal)

    def test_process_batch_with_synthetic_images(self):
        test_dir = os.path.abspath("test_pyocr_tmp")
        os.makedirs(test_dir, exist_ok=True)
        img_path = os.path.join(test_dir, "test_crop.jpg")
        
        # Draw a synthetic subtitle bar image with white text on black background
        img = np.zeros((80, 500, 3), dtype=np.uint8)
        cv2.putText(img, "HELLO WORLD", (30, 50), cv2.FONT_HERSHEY_SIMPLEX, 1.2, (255, 255, 255), 2)
        cv2.imwrite(img_path, img)

        try:
            results = _process_batch([img_path], lang="en")
            self.assertEqual(len(results), 1)
            self.assertTrue(results[0].success)
            self.assertGreaterEqual(len(results[0].items), 1)
            found_text = results[0].items[0].text.upper()
            self.assertIn("HELLO", found_text)
        finally:
            if os.path.exists(img_path):
                os.remove(img_path)
            if os.path.exists(test_dir):
                os.rmdir(test_dir)

if __name__ == "__main__":
    unittest.main()
