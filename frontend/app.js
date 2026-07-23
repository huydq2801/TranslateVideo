// Comicify Video Translator - Client Side Application Core

document.addEventListener('DOMContentLoaded', () => {
    // ── DOM References ────────────────────────────────────────────────────────
    const video = document.getElementById('video-player');
    const canvas = document.getElementById('bubble-canvas');
    const ctx = canvas.getContext('2d');
    const btnSelectVideo = document.getElementById('btn-select-video');
    const btnScanFull = document.getElementById('btn-scan-full');
    const btnBatchTranslate = document.getElementById('btn-batch-translate');
    const btnExportVideo = document.getElementById('btn-export-video');
    const btnClear = document.getElementById('btn-clear');
    const btnClearStrip = document.getElementById('btn-clear-strip');
    const btnRetryFailed = document.getElementById('btn-retry-failed');
    const comicStrip = document.getElementById('comic-strip');
    
    // Inputs & Settings
    const scanIntervalInput = document.getElementById('scan-interval');
    const scanLanguageSelect = document.getElementById('scan-language');
    const bubbleThresholdInput = document.getElementById('bubble-threshold');
    const thresholdValSpan = document.getElementById('threshold-val');
    const inputGeminiKey = document.getElementById('input-gemini-key');
    const translationGlossary = document.getElementById('translation-glossary');

    // Health Dashboard Elements
    const badgeOCR = document.getElementById('badge-ocr');
    const badgeGemini = document.getElementById('badge-gemini');
    const ocrStatusText = document.getElementById('ocr-status-text');
    const geminiStatusText = document.getElementById('gemini-status-text');

    // Progress Overlay
    const scanProgressOverlay = document.getElementById('scan-progress-overlay');
    const progressModalTitle = document.getElementById('progress-modal-title');
    const scanProgressBar = document.getElementById('scan-progress-bar');
    const scanProgressStatus = document.getElementById('scan-progress-status');
    const scanStatTime = document.getElementById('scan-stat-time');
    const scanStatFound = document.getElementById('scan-stat-found');
    const btnCancelScan = document.getElementById('btn-cancel-scan');
    const loader = document.getElementById('loader');
    const loaderText = document.getElementById('loader-text');

    let currentVideoPath = '';
    let detectedBubbles = [];
    let isProcessing = false;
    let selectedDialogueId = null;
    let dragState = null;
    let appMode = 'preview'; // 'preview' (Final Video Preview) or 'edit' (Interactive BBox Editor)

    const btnModePreview = document.getElementById('btn-mode-preview');
    const btnModeEdit = document.getElementById('btn-mode-edit');

    if (btnModePreview && btnModeEdit) {
        btnModePreview.addEventListener('click', () => {
            appMode = 'preview';
            btnModePreview.classList.add('active');
            btnModeEdit.classList.remove('active');
            drawCanvasOverlay();
        });

        btnModeEdit.addEventListener('click', () => {
            appMode = 'edit';
            btnModeEdit.classList.add('active');
            btnModePreview.classList.remove('active');
            drawCanvasOverlay();
        });
    }

    // ResizeObserver to keep overlay canvas matched to video element dimensions dynamically
    const videoContainer = document.getElementById('video-container');
    if (videoContainer) {
        const resizeObserver = new ResizeObserver(() => {
            canvas.width = video.clientWidth || videoContainer.clientWidth;
            canvas.height = video.clientHeight || videoContainer.clientHeight;
            drawCanvasOverlay();
        });
        resizeObserver.observe(videoContainer);

        videoContainer.addEventListener('mousemove', (event) => {
            if (dragState || appMode === 'preview') return;
            const layout = previewLayout();
            if (!layout) return;
            const rect = canvas.getBoundingClientRect();
            const pointX = (event.clientX - rect.left) * (canvas.width / rect.width);
            const pointY = (event.clientY - rect.top) * (canvas.height / rect.height);
            const hit = getHitInfo(pointX, pointY, layout);
            if (hit) {
                canvas.style.pointerEvents = 'auto';
                canvas.style.cursor = hit.cursor;
            } else {
                canvas.style.pointerEvents = 'none';
                canvas.style.cursor = 'default';
            }
        });
    }

    function selectedDialogue() {
        return detectedBubbles.find(d => d.id === selectedDialogueId) || null;
    }

    function previewLayout() {
        const canvasWidth = canvas.width;
        const canvasHeight = canvas.height;
        if (!video.videoWidth || !video.videoHeight || !canvasWidth || !canvasHeight) return null;

        const scale = Math.min(canvasWidth / video.videoWidth, canvasHeight / video.videoHeight);
        const width = video.videoWidth * scale;
        const height = video.videoHeight * scale;
        return { scale, left: (canvasWidth - width) / 2, top: (canvasHeight - height) / 2 };
    }

    function getDialogueDuration(d, index) {
        if (!detectedBubbles || detectedBubbles.length === 0) return 2.0;

        if (d.endTimestamp && d.endTimestamp > d.timestamp) {
            let dur = d.endTimestamp - d.timestamp;
            if (index < detectedBubbles.length - 1) {
                const nextTs = detectedBubbles[index + 1].timestamp;
                if (d.timestamp + dur >= nextTs) return Math.max(0.1, nextTs - d.timestamp - 0.05);
            }
            return dur;
        }

        if (d.duration && d.duration > 0) {
            let dur = d.duration;
            if (index < detectedBubbles.length - 1) {
                const nextTs = detectedBubbles[index + 1].timestamp;
                if (d.timestamp + dur >= nextTs) return Math.max(0.1, nextTs - d.timestamp - 0.05);
            }
            return dur;
        }

        if (index < detectedBubbles.length - 1) {
            const nextTs = detectedBubbles[index + 1].timestamp;
            if (nextTs > d.timestamp) {
                return Math.max(0.1, nextTs - d.timestamp - 0.05);
            }
        }
        return 2.0;
    }

    function getActiveDialogues() {
        if (!detectedBubbles || detectedBubbles.length === 0) return [];
        const currentTime = video.currentTime || 0;
        const activeMap = new Map();

        detectedBubbles.forEach((d, idx) => {
            const dur = getDialogueDuration(d, idx);
            if (currentTime >= (d.timestamp - 0.1) && currentTime <= (d.timestamp + dur)) {
                activeMap.set(d.id, d);
            }
        });

        const selected = selectedDialogue();
        if (selected && !activeMap.has(selected.id) && appMode === 'edit') {
            if (video.paused || Math.abs(currentTime - selected.timestamp) < 3.0) {
                activeMap.set(selected.id, selected);
            }
        }

        return Array.from(activeMap.values());
    }

    function drawFinalSpeechBubble(d, x, y, w, h) {
        const text = d.translated || d.original;
        if (!text) return;

        ctx.save();
        const radius = Math.min(12, Math.min(w, h) / 4);

        // 1. Draw borderless white rounded rectangle speech bubble
        ctx.beginPath();
        if (ctx.roundRect) {
            ctx.roundRect(x, y, w, h, radius);
        } else {
            ctx.rect(x, y, w, h);
        }
        ctx.fillStyle = '#ffffff';
        ctx.fill();

        // 3. Draw wrapped centered black text
        const pad = 8;
        const maxW = w - pad * 2;
        const maxH = h - pad * 2;

        if (maxW > 10 && maxH > 10) {
            let fontSize = Math.min(22, Math.max(11, Math.floor(h / 3)));
            ctx.font = `bold ${fontSize}px "Inter", "Segoe UI", sans-serif`;
            ctx.fillStyle = '#000000';
            ctx.textAlign = 'center';
            ctx.textBaseline = 'top';

            const words = text.split(/\s+/);
            let lines = [];
            let currentLine = '';

            for (let word of words) {
                let testLine = currentLine ? currentLine + ' ' + word : word;
                if (ctx.measureText(testLine).width > maxW && currentLine) {
                    lines.push(currentLine);
                    currentLine = word;
                } else {
                    currentLine = testLine;
                }
            }
            if (currentLine) lines.push(currentLine);

            let lineHeight = fontSize * 1.3;
            if (lines.length * lineHeight > maxH && fontSize > 9) {
                fontSize = Math.max(9, Math.floor(maxH / (lines.length * 1.3)));
                lineHeight = fontSize * 1.3;
                ctx.font = `bold ${fontSize}px "Inter", "Segoe UI", sans-serif`;
            }

            const totalH = lines.length * lineHeight;
            let startY = y + (h - totalH) / 2;

            lines.forEach(line => {
                ctx.fillText(line, x + w / 2, startY);
                startY += lineHeight;
            });
        }
        ctx.restore();
    }

    function drawCanvasOverlay() {
        ctx.clearRect(0, 0, canvas.width, canvas.height);
        const layout = previewLayout();
        if (!layout) {
            canvas.classList.remove('is-editing');
            return;
        }

        const activeList = getActiveDialogues();
        canvas.classList.toggle('is-editing', activeList.length > 0 && appMode === 'edit');
        if (activeList.length === 0) return;

        // Mode 1: Final Video Preview Mode
        if (appMode === 'preview') {
            canvas.style.pointerEvents = 'none';
            canvas.style.cursor = 'default';
            activeList.forEach(d => {
                const { bbox } = d;
                const x = layout.left + bbox.x * layout.scale;
                const y = layout.top + bbox.y * layout.scale;
                const width = bbox.width * layout.scale;
                const height = bbox.height * layout.scale;

                drawFinalSpeechBubble(d, x, y, width, height);
            });
            return;
        }

        // Mode 2: Interactive BBox Editor Mode
        activeList.sort((a, b) => (a.id === selectedDialogueId ? 1 : 0) - (b.id === selectedDialogueId ? 1 : 0));

        activeList.forEach(d => {
            const isSelected = (d.id === selectedDialogueId);
            const { bbox } = d;
            const x = layout.left + bbox.x * layout.scale;
            const y = layout.top + bbox.y * layout.scale;
            const width = bbox.width * layout.scale;
            const height = bbox.height * layout.scale;

            if (isSelected) {
                // Golden highlight theme for selected box
                ctx.fillStyle = 'rgba(250, 204, 21, 0.2)';
                ctx.strokeStyle = '#facc15';
                ctx.lineWidth = 2.5;
                ctx.setLineDash([]);
                ctx.fillRect(x, y, width, height);
                ctx.strokeRect(x, y, width, height);

                // Draw 8 resize handles
                drawHandles(x, y, width, height);

                // Editor badge label
                const labelText = ` Thoại #${d.id} [${Math.round(bbox.width)}x${Math.round(bbox.height)}]`;
                ctx.fillStyle = '#facc15';
                ctx.fillRect(x, Math.max(0, y - 22), 170, 20);
                ctx.fillStyle = '#0f0c1b';
                ctx.font = 'bold 12px Inter, sans-serif';
                ctx.fillText(labelText, x + 4, Math.max(14, y - 8));
            } else {
                // Cyan dashed line theme for active boxes during playback
                ctx.fillStyle = 'rgba(56, 189, 248, 0.15)';
                ctx.strokeStyle = '#38bdf8';
                ctx.lineWidth = 2;
                ctx.setLineDash([4, 4]);
                ctx.fillRect(x, y, width, height);
                ctx.strokeRect(x, y, width, height);
                ctx.setLineDash([]);

                // Small badge tag
                const labelText = ` Thoại #${d.id}`;
                ctx.fillStyle = '#38bdf8';
                ctx.fillRect(x, Math.max(0, y - 20), 80, 18);
                ctx.fillStyle = '#0f0c1b';
                ctx.font = 'bold 11px Inter, sans-serif';
                ctx.fillText(labelText, x + 4, Math.max(13, y - 6));
            }
        });
    }

    function drawHandles(x, y, w, h) {
        const handleSize = 8;
        const half = handleSize / 2;
        const positions = [
            { x: x, y: y },
            { x: x + w / 2, y: y },
            { x: x + w, y: y },
            { x: x + w, y: y + h / 2 },
            { x: x + w, y: y + h },
            { x: x + w / 2, y: y + h },
            { x: x, y: y + h },
            { x: x, y: y + h / 2 }
        ];

        ctx.fillStyle = '#ffffff';
        ctx.strokeStyle = '#0f0c1b';
        ctx.lineWidth = 1.5;
        ctx.setLineDash([]);
        positions.forEach(pos => {
            ctx.fillRect(pos.x - half, pos.y - half, handleSize, handleSize);
            ctx.strokeRect(pos.x - half, pos.y - half, handleSize, handleSize);
        });
    }

    function getHitInfo(pointX, pointY, layout) {
        if (appMode === 'preview') return null;
        const activeList = getActiveDialogues();
        if (!layout || activeList.length === 0) return null;

        const handleRadius = 10;

        // Prioritize selected dialogue first
        activeList.sort((a, b) => (a.id === selectedDialogueId ? -1 : 1));

        for (const d of activeList) {
            const { bbox } = d;
            const x = layout.left + bbox.x * layout.scale;
            const y = layout.top + bbox.y * layout.scale;
            const w = bbox.width * layout.scale;
            const h = bbox.height * layout.scale;

            if (d.id === selectedDialogueId) {
                // Check toggle text badge button
                const toggleBadgeX = x + 175;
                const toggleBadgeY = Math.max(0, y - 22);
                if (pointX >= toggleBadgeX && pointX <= toggleBadgeX + 85 && pointY >= toggleBadgeY && pointY <= toggleBadgeY + 22) {
                    return { dialogue: d, handleType: 'toggleText', cursor: 'pointer' };
                }

                const handles = [
                    { type: 'nw', x: x, y: y, cursor: 'nwse-resize' },
                    { type: 'n', x: x + w / 2, y: y, cursor: 'ns-resize' },
                    { type: 'ne', x: x + w, y: y, cursor: 'nesw-resize' },
                    { type: 'e', x: x + w, y: y + h / 2, cursor: 'ew-resize' },
                    { type: 'se', x: x + w, y: y + h, cursor: 'nwse-resize' },
                    { type: 's', x: x + w / 2, y: y + h, cursor: 'ns-resize' },
                    { type: 'sw', x: x, y: y + h, cursor: 'nesw-resize' },
                    { type: 'w', x: x, y: y + h / 2, cursor: 'ew-resize' }
                ];

                for (const handle of handles) {
                    if (Math.abs(pointX - handle.x) <= handleRadius && Math.abs(pointY - handle.y) <= handleRadius) {
                        return { dialogue: d, handleType: handle.type, cursor: handle.cursor };
                    }
                }
            }

            if (pointX >= x && pointX <= x + w && pointY >= y && pointY <= y + h) {
                return { dialogue: d, handleType: 'move', cursor: 'move' };
            }
        }
        return null;
    }

    function selectDialogue(dialogue) {
        if (!dialogue) {
            selectedDialogueId = null;
            drawCanvasOverlay();
            return;
        }

        // Always sync latest edited time values from DOM input elements before seeking
        const card = comicStrip.querySelector(`.comic-card[data-dialogue-id="${dialogue.id}"]`);
        if (card) {
            const timeStartIn = card.querySelector('.time-start');
            const timeEndIn = card.querySelector('.time-end');
            if (timeStartIn && timeEndIn) {
                const parsedStart = parseTimeInput(timeStartIn.value);
                const parsedEnd = parseTimeInput(timeEndIn.value);
                if (parsedStart !== null && parsedStart >= 0) {
                    dialogue.timestamp = parsedStart;
                }
                if (parsedEnd !== null && parsedEnd > dialogue.timestamp) {
                    dialogue.endTimestamp = parsedEnd;
                    dialogue.duration = Math.max(0.1, parsedEnd - dialogue.timestamp);
                }
            }
        }

        selectedDialogueId = dialogue.id;
        video.pause();
        video.currentTime = Math.max(0, dialogue.timestamp);
        drawCanvasOverlay();

        const cards = comicStrip.querySelectorAll('.comic-card');
        cards.forEach(c => {
            if (c.dataset.dialogueId == selectedDialogueId) {
                c.classList.add('selected-card');
            } else {
                c.classList.remove('selected-card');
            }
        });
    }

    video.addEventListener('loadedmetadata', drawCanvasOverlay);
    video.addEventListener('timeupdate', drawCanvasOverlay);
    video.addEventListener('play', drawCanvasOverlay);
    video.addEventListener('pause', drawCanvasOverlay);
    video.addEventListener('seeking', drawCanvasOverlay);
    video.addEventListener('seeked', drawCanvasOverlay);
    video.addEventListener('resize', drawCanvasOverlay);

    canvas.addEventListener('pointerdown', event => {
        const layout = previewLayout();
        if (!layout) return;
        const rect = canvas.getBoundingClientRect();
        const pointX = (event.clientX - rect.left) * (canvas.width / rect.width);
        const pointY = (event.clientY - rect.top) * (canvas.height / rect.height);

        const hit = getHitInfo(pointX, pointY, layout);
        if (!hit) return;

        event.preventDefault();
        const { dialogue, handleType } = hit;

        if (handleType === 'toggleText') {
            showPreviewText = !showPreviewText;
            updatePreviewTextBtnUI();
            drawCanvasOverlay();
            return;
        }

        if (dialogue.id !== selectedDialogueId) {
            selectDialogue(dialogue);
        }

        dragState = {
            dialogue,
            handleType,
            startX: pointX,
            startY: pointY,
            origX: dialogue.bbox.x,
            origY: dialogue.bbox.y,
            origW: dialogue.bbox.width,
            origH: dialogue.bbox.height
        };

        canvas.setPointerCapture(event.pointerId);
    });

    canvas.addEventListener('pointermove', event => {
        const layout = previewLayout();
        if (!layout) return;
        const rect = canvas.getBoundingClientRect();
        const pointX = (event.clientX - rect.left) * (canvas.width / rect.width);
        const pointY = (event.clientY - rect.top) * (canvas.height / rect.height);

        if (!dragState) {
            const hit = getHitInfo(pointX, pointY, layout);
            if (hit) {
                canvas.style.cursor = hit.cursor;
                canvas.style.pointerEvents = 'auto';
            } else {
                canvas.style.cursor = 'default';
                const activeList = getActiveDialogues();
                if (activeList.length === 0) {
                    canvas.style.pointerEvents = 'none';
                }
            }
            return;
        }

        const { dialogue, handleType, startX, startY, origX, origY, origW, origH } = dragState;
        const dx = (pointX - startX) / layout.scale;
        const dy = (pointY - startY) / layout.scale;
        const minSize = 10;
        const vw = video.videoWidth || 1920;
        const vh = video.videoHeight || 1080;

        let newX = origX;
        let newY = origY;
        let newW = origW;
        let newH = origH;

        if (handleType === 'move') {
            newX = Math.max(0, Math.min(vw - origW, origX + dx));
            newY = Math.max(0, Math.min(vh - origH, origY + dy));
        } else {
            if (handleType.includes('e')) {
                newW = Math.max(minSize, Math.min(vw - origX, origW + dx));
            }
            if (handleType.includes('s')) {
                newH = Math.max(minSize, Math.min(vh - origY, origH + dy));
            }
            if (handleType.includes('w')) {
                const maxDx = origW - minSize;
                const actualDx = Math.min(maxDx, Math.max(-origX, dx));
                newX = origX + actualDx;
                newW = origW - actualDx;
            }
            if (handleType.includes('n')) {
                const maxDy = origH - minSize;
                const actualDy = Math.min(maxDy, Math.max(-origY, dy));
                newY = origY + actualDy;
                newH = origH - actualDy;
            }
        }

        dialogue.bbox.x = Math.round(newX);
        dialogue.bbox.y = Math.round(newY);
        dialogue.bbox.width = Math.round(newW);
        dialogue.bbox.height = Math.round(newH);

        drawCanvasOverlay();
    });

    canvas.addEventListener('pointerup', event => {
        if (dragState) {
            dragState = null;
        }
        if (canvas.hasPointerCapture && canvas.hasPointerCapture(event.pointerId)) {
            canvas.releasePointerCapture(event.pointerId);
        }
    });

    inputGeminiKey.addEventListener('input', e => {
        checkHealth();
    });

    // Glossary entries are not credentials, so keeping them locally saves the
    // user from re-entering character names for every video.
    translationGlossary.value = localStorage.getItem('comicify_translation_glossary') || '';
    translationGlossary.addEventListener('input', e => {
        localStorage.setItem('comicify_translation_glossary', e.target.value);
    });

    function setProcessing(active) {
        isProcessing = active;
        [btnSelectVideo, btnScanFull, btnBatchTranslate, btnExportVideo, btnRetryFailed].forEach(button => {
            if (button) button.disabled = active;
        });
    }

    // ── Threshold Slider Sync ──────────────────────────────────────────────────
    bubbleThresholdInput.addEventListener('input', e => {
        thresholdValSpan.textContent = e.target.value;
    });

    // ── Health Check Poller ───────────────────────────────────────────────────
    async function checkHealth() {
        if (window.go && window.go.main && window.go.main.App) {
            try {
                const apiKey = inputGeminiKey.value.trim();
                const health = await window.go.main.App.CheckSystemHealth(apiKey);
                
                // OCR Badge
                const ocrDot = badgeOCR.querySelector('.dot');
                if (health.ocrOnline) {
                    ocrDot.className = 'dot online';
                    ocrStatusText.textContent = `Online (${health.ocrEngine})`;
                } else {
                    ocrDot.className = 'dot offline';
                    ocrStatusText.textContent = 'Chưa bật Sidecar';
                }

                // Gemini Badge
                const geminiDot = badgeGemini.querySelector('.dot');
                if (health.hasApiKey) {
                    geminiDot.className = 'dot online';
                    geminiStatusText.textContent = 'API Key Sẵn Sàng';
                } else {
                    geminiDot.className = 'dot warning';
                    geminiStatusText.textContent = 'Thiếu API Key';
                }
            } catch (err) {
                console.error('Lỗi kiểm tra hệ thống:', err);
            }
        }
    }

    setInterval(checkHealth, 5000);
    checkHealth();

    // ── Select Video File ─────────────────────────────────────────────────────
    btnSelectVideo.addEventListener('click', async () => {
        if (window.go && window.go.main && window.go.main.App) {
            try {
                const filePath = await window.go.main.App.SelectVideoFile();
                if (filePath) {
                    currentVideoPath = filePath;
                    video.src = '/wails-local-video?path=' + encodeURIComponent(filePath);
                    video.load();
                    detectedBubbles = [];
                    selectedDialogueId = null;
                    drawCanvasOverlay();
                    renderEmptyStrip();
                }
            } catch (err) {
                alert('Không thể chọn file: ' + (err.message || err));
            }
        }
    });

    // ── 1. Fast Scan Frame & OCR ──────────────────────────────────────────────
    btnScanFull.addEventListener('click', async () => {
        if (isProcessing) return;
        if (!currentVideoPath) {
            alert('Vui lòng chọn video trước!');
            return;
        }

        if (detectedBubbles.length > 0) {
            const confirmed = confirm('⚠️ Bạn có chắc chắn muốn Quét lại Frame & OCR không?\n\nHành động này sẽ làm mới và ghi đè danh sách thoại đã có!');
            if (!confirmed) return;
        }

        const config = {
            videoPath: currentVideoPath,
            interval: parseFloat(scanIntervalInput.value) || 1.0,
            threshold: parseInt(bubbleThresholdInput.value) || 240,
            sourceLang: scanLanguageSelect.value,
            geminiKey: inputGeminiKey.value.trim()
        };

        progressModalTitle.textContent = 'Đang Quét Frame & Nhận Diện OCR Local...';
        scanProgressBar.style.width = '0%';
        scanProgressOverlay.classList.remove('hidden');
        scanProgressStatus.textContent = 'Khởi tạo pipeline local...';
        detectedBubbles = [];

        setProcessing(true);

        try {
            const results = await window.go.main.App.StartFullPipelineScan(config);
            scanProgressOverlay.classList.add('hidden');
            if (results && results.length > 0) {
                detectedBubbles = results;
                renderComicStrip(results);
                alert(`✅ Quét OCR xong! Tìm thấy ${results.length} thoại. Bấm "2. Dịch Ngữ Cảnh Batch (Gemini AI)" để dịch toàn bộ câu chuyện.`);
            } else {
                alert('Không tìm thấy bong bóng thoại nào trên video.');
            }
        } catch (err) {
            scanProgressOverlay.classList.add('hidden');
            const errMsg = err.message || String(err);
            if (errMsg.includes('dừng bởi người dùng')) {
                alert('⏹️ Đã dừng tiến trình quét theo yêu cầu.');
            } else {
                alert('Lỗi quét pipeline: ' + errMsg);
            }
        } finally {
            setProcessing(false);
            if (window.go && window.go.main && window.go.main.App) {
                try { window.go.main.App.ClearSystemMemory(); } catch (e) {}
            }
        }
    });

    if (btnClear) {
        btnClear.addEventListener('click', async () => {
            if (!confirm('Bạn có chắc chắn muốn xóa toàn bộ danh sách thoại và giải phóng RAM không?')) return;
            detectedBubbles = [];
            selectedDialogueId = null;
            drawCanvasOverlay();
            renderEmptyStrip();
            if (window.go && window.go.main && window.go.main.App) {
                try { await window.go.main.App.ClearSystemMemory(); } catch (e) {}
            }
            alert('✅ Đã xóa toàn bộ overlay và giải phóng RAM hệ thống!');
        });
    }

    if (btnClearStrip) {
        btnClearStrip.addEventListener('click', async () => {
            if (!confirm('Bạn có chắc chắn muốn xóa tất cả thoại?')) return;
            detectedBubbles = [];
            selectedDialogueId = null;
            drawCanvasOverlay();
            renderEmptyStrip();
            if (window.go && window.go.main && window.go.main.App) {
                try { await window.go.main.App.ClearSystemMemory(); } catch (e) {}
            }
        });
    }

    // ── 2. Batch Context Translation via Gemini ───────────────────────────────
    if (btnBatchTranslate) {
        btnBatchTranslate.addEventListener('click', async () => {
            if (isProcessing) return;
            if (detectedBubbles.length === 0) {
                alert('Chưa có bong bóng thoại nào! Bấm "1. Quét Frame & OCR" trước.');
                return;
            }

            const apiKey = inputGeminiKey.value.trim();
            if (!apiKey) {
                alert('Vui lòng dán Gemini API Key ở ô trên góc phải!');
                return;
            }

            const confirmed = confirm(`⚠️ Bạn có chắc chắn muốn Dịch Ngữ Cảnh cho toàn bộ ${detectedBubbles.length} câu thoại bằng Gemini AI không?\n\nViệc này sẽ thay thế bản dịch hiện tại của tất cả các thoại.`);
            if (!confirmed) return;

            progressModalTitle.textContent = 'Đang Dịch Ngữ Cảnh Bằng Gemini AI...';
            scanProgressBar.style.width = '0%';
            scanStatTime.textContent = `0 / ${detectedBubbles.length} câu`;
            scanStatFound.innerHTML = `<i class="fa-solid fa-wand-magic-sparkles"></i> Gemini AI`;
            scanProgressStatus.textContent = `Gửi ${detectedBubbles.length} câu thoại tới Gemini AI...`;
            scanProgressOverlay.classList.remove('hidden');
            setProcessing(true);

            try {
                const updated = await window.go.main.App.BatchTranslateDialogues(
                    detectedBubbles,
                    apiKey,
                    translationGlossary.value
                );
                scanProgressOverlay.classList.add('hidden');
                if (updated && updated.length > 0) {
                    detectedBubbles = updated;
                    renderComicStrip(updated);
                    alert(`🎉 Hoàn tất dịch toàn bộ ${updated.length} câu thoại bằng Gemini AI!`);
                }
            } catch (err) {
                scanProgressOverlay.classList.add('hidden');
                alert('Lỗi dịch Gemini AI:\n' + (err.message || err));
            } finally {
                setProcessing(false);
            }
        });
    }

    // ── Retry Failed Single Lines via Gemini ──────────────────────────────────
    if (btnRetryFailed) {
        btnRetryFailed.addEventListener('click', async () => {
            if (isProcessing) return;
            const failedItems = detectedBubbles.filter(d => d.translationFailed);
            if (failedItems.length === 0) {
                alert('Không có câu thoại nào bị lỗi dịch!');
                return;
            }

            const apiKey = inputGeminiKey.value.trim();
            if (!apiKey) {
                alert('Vui lòng nhập Gemini API Key trước khi dịch lại.');
                return;
            }
            showLoader(`Đang thử dịch lại ${failedItems.length} câu thoại lỗi với Gemini AI...`);
            setProcessing(true);

            try {
                for (let d of failedItems) {
                    try {
                        const res = await window.go.main.App.TranslateSingleText(
                            d.original,
                            apiKey,
                            translationGlossary.value
                        );
                        d.translated = res.translated || d.original;
                        d.translationFailed = res.translationFailed;
                    } catch (err) {
                        console.error('Lỗi dịch lại:', err);
                    }
                }
            } finally {
                hideLoader();
                setProcessing(false);
            }
            renderComicStrip(detectedBubbles);
            alert(`✅ Đã dịch lại xong ${failedItems.length} câu thoại!`);
        });
    }

    // ── Wails Event Listeners ────────────────────────────────────────────────
    if (window.runtime && window.runtime.EventsOn) {
        window.runtime.EventsOn('scan-progress', (progress) => {
            if (progress.totalFrames > 0) {
                const pct = Math.round((progress.currentFrame / progress.totalFrames) * 100);
                scanProgressBar.style.width = pct + '%';
                scanStatTime.textContent = `${progress.currentFrame} / ${progress.totalFrames} frame (${pct}%)`;
            }
            scanStatFound.innerHTML = `<i class="fa-solid fa-comment"></i> ${progress.foundCount} bong bóng`;
            scanProgressStatus.textContent = progress.status || 'Đang xử lý...';

            if (progress.newDialogues && progress.newDialogues.length > 0) {
                progress.newDialogues.forEach(d => {
                    detectedBubbles.push(d);
                    addComicCard(d, true);
                });
            }
        });

        window.runtime.EventsOn('batch-translate-progress', (data) => {
            const pct = data.percent || 0;
            scanProgressBar.style.width = pct + '%';
            if (data.total > 0) {
                scanStatTime.textContent = `${data.current} / ${data.total} câu (${pct}%)`;
            }
            scanProgressStatus.textContent = data.status || 'Đang dịch bằng Gemini AI...';
        });

        window.runtime.EventsOn('scan-complete', (progress) => {
            scanProgressOverlay.classList.add('hidden');
        });
    }

    const btnScanFrameAI = document.getElementById('btn-scan-frame-ai');

    if (btnScanFrameAI) {
        btnScanFrameAI.addEventListener('click', async () => {
            if (isProcessing) return;
            if (!currentVideoPath) {
                alert('Vui lòng chọn video trước!');
                return;
            }

            const currentTs = video.currentTime || 0;
            showLoader(`Đang bóc tách chữ & dịch AI tại thời điểm ${formatTime(currentTs)}...`);
            setProcessing(true);

            try {
                const config = {
                    videoPath: currentVideoPath,
                    timestamp: currentTs,
                    threshold: parseInt(bubbleThresholdInput.value) || 240,
                    sourceLang: scanLanguageSelect.value,
                    geminiKey: inputGeminiKey.value.trim(),
                    glossary: translationGlossary.value
                };

                const newItems = await window.go.main.App.ScanSingleFrameWithAI(config);
                hideLoader();

                if (newItems && newItems.length > 0) {
                    let maxId = detectedBubbles.length > 0 ? Math.max(...detectedBubbles.map(b => b.id)) : 0;
                    newItems.forEach(item => {
                        maxId++;
                        item.id = maxId;
                        detectedBubbles.push(item);
                    });

                    detectedBubbles.sort((a, b) => a.timestamp - b.timestamp);
                    renderComicStrip(detectedBubbles, true);
                    selectDialogue(newItems[0]);
                    alert(`✨ AI đã bóc tách chữ & dịch thành công ${newItems.length} thoại tại ${formatTime(currentTs)}!`);
                } else {
                    const vW = video.videoWidth || 1920;
                    const vH = video.videoHeight || 1080;
                    const boxW = Math.round(vW * 0.35);
                    const boxH = Math.round(vH * 0.15);
                    const boxX = Math.round((vW - boxW) / 2);
                    const boxY = Math.round(vH * 0.7);

                    const newId = (detectedBubbles.length > 0 ? Math.max(...detectedBubbles.map(b => b.id)) : 0) + 1;
                    const manualBubble = {
                        id: newId,
                        frameIndex: Math.floor(currentTs),
                        timestamp: currentTs,
                        duration: 2.5,
                        endTimestamp: currentTs + 2.5,
                        original: 'Chưa đọc chữ. Kéo khung và bấm "AI Đọc & Dịch"',
                        translated: 'Chưa đọc chữ. Kéo khung và bấm "AI Đọc & Dịch"',
                        translationFailed: false,
                        bbox: { x: boxX, y: boxY, width: boxW, height: boxH }
                    };

                    detectedBubbles.push(manualBubble);
                    detectedBubbles.sort((a, b) => a.timestamp - b.timestamp);

                    appMode = 'edit';
                    if (btnModeEdit) {
                        btnModeEdit.classList.add('active');
                        if (btnModePreview) btnModePreview.classList.remove('active');
                    }

                    renderComicStrip(detectedBubbles, true);
                    selectDialogue(manualBubble);

                    alert(`ℹ️ AI chưa nhận diện được viền thoại tự động trên frame này.\n\nĐã tạo 1 khung thoại tại ${formatTime(currentTs)}. Bạn hãy điều chỉnh vị trí khung rồi bấm "AI Đọc & Dịch" trên thẻ thoại!`);
                }
            } catch (err) {
                hideLoader();
                alert('Lỗi quét frame bằng AI: ' + (err.message || err));
            } finally {
                setProcessing(false);
            }
        });
    }

    const btnAddManual = document.getElementById('btn-add-manual');
    if (btnAddManual) {
        btnAddManual.addEventListener('click', () => {
            if (!currentVideoPath) {
                alert('Vui lòng chọn video trước!');
                return;
            }

            const currentTs = video.currentTime || 0;
            const newId = (detectedBubbles.length > 0 ? Math.max(...detectedBubbles.map(b => b.id)) : 0) + 1;
            
            const vW = video.videoWidth || 1920;
            const vH = video.videoHeight || 1080;
            const boxW = Math.round(vW * 0.35);
            const boxH = Math.round(vH * 0.15);
            const boxX = Math.round((vW - boxW) / 2);
            const boxY = Math.round(vH * 0.7);

            const newBubble = {
                id: newId,
                frameIndex: Math.floor(currentTs),
                timestamp: currentTs,
                duration: 2.5,
                endTimestamp: currentTs + 2.5,
                original: 'Nhập nội dung thoại...',
                translated: 'Nhập nội dung thoại...',
                translationFailed: false,
                bbox: { x: boxX, y: boxY, width: boxW, height: boxH }
            };

            detectedBubbles.push(newBubble);
            detectedBubbles.sort((a, b) => a.timestamp - b.timestamp);

            appMode = 'edit';
            if (btnModeEdit) {
                btnModeEdit.classList.add('active');
                if (btnModePreview) btnModePreview.classList.remove('active');
            }

            renderComicStrip(detectedBubbles, true);
            selectDialogue(newBubble);
        });
    }

    // ── Export Video ──────────────────────────────────────────────────────────
    btnExportVideo.addEventListener('click', async () => {
        if (isProcessing) return;
        if (detectedBubbles.length === 0) {
            alert('Chưa có bong bóng thoại nào! Hãy thực hiện quét & dịch trước.');
            return;
        }

        try {
            // Construct default filename recommendation based on input video name
            let baseName = 'video_translated.mp4';
            if (currentVideoPath) {
                const parts = currentVideoPath.split(/[\\/]/);
                const file = parts[parts.length - 1];
                const dot = file.lastIndexOf('.');
                if (dot > 0) {
                    baseName = file.substring(0, dot) + '_Vietsub.mp4';
                } else {
                    baseName = file + '_Vietsub.mp4';
                }
            }

            // 1. Open native Windows Save File Dialog for custom directory & filename
            const targetPath = await window.go.main.App.SelectSaveLocation(baseName);
            if (!targetPath) return; // User cancelled save dialog

            showLoader(`Đang render ${detectedBubbles.length} khung thoại và ghép vào video bằng FFmpeg GPU...`);
            setProcessing(true);

            // 2. Export video to chosen path
            const finalPath = await window.go.main.App.ExportVideo(currentVideoPath, JSON.stringify(detectedBubbles), targetPath);
            hideLoader();
            alert(`✅ Xuất video thành công!\nFile đã lưu tại:\n${finalPath}`);
        } catch (err) {
            hideLoader();
            alert('Lỗi xuất video: ' + (err.message || err));
        } finally {
            setProcessing(false);
        }
    });

    // ── Render Helpers ────────────────────────────────────────────────────────
    function renderEmptyStrip() {
        comicStrip.innerHTML = `
            <div class="empty-state">
                <i class="fa-solid fa-photo-film"></i>
                <p>Chưa có bong bóng thoại nào. Bấm <strong>"Chọn Video"</strong> và <strong>"Quét Frame & OCR"</strong> để bắt đầu!</p>
            </div>
        `;
    }

    function renderComicStrip(items, preserveScroll = false) {
        const savedScroll = comicStrip.scrollTop;
        comicStrip.innerHTML = '';
        if (!items || items.length === 0) {
            renderEmptyStrip();
            return;
        }
        items.forEach(d => addComicCard(d, false));
        if (preserveScroll) {
            comicStrip.scrollTop = savedScroll;
        }
    }

    function parseTimeInput(str) {
        if (!str) return null;
        str = str.trim();
        if (str.includes(':')) {
            const parts = str.split(':');
            if (parts.length === 2) {
                const m = parseFloat(parts[0]);
                const s = parseFloat(parts[1]);
                if (!isNaN(m) && !isNaN(s)) return m * 60 + s;
            } else if (parts.length === 3) {
                const h = parseFloat(parts[0]);
                const m = parseFloat(parts[1]);
                const s = parseFloat(parts[2]);
                if (!isNaN(h) && !isNaN(m) && !isNaN(s)) return h * 3600 + m * 60 + s;
            }
        } else {
            const val = parseFloat(str);
            if (!isNaN(val)) return val;
        }
        return null;
    }

    function addComicCard(d, autoScroll = false) {
        const empty = comicStrip.querySelector('.empty-state');
        if (empty) comicStrip.innerHTML = '';

        const card = document.createElement('div');
        card.dataset.dialogueId = d.id;
        const isFailed = d.translationFailed;
        card.className = `comic-card ${isFailed ? 'failed-card' : ''} ${d.id === selectedDialogueId ? 'selected-card' : ''}`;
        
        const idx = detectedBubbles.findIndex(item => item.id === d.id);
        const dur = getDialogueDuration(d, idx >= 0 ? idx : 0);
        const endTs = d.timestamp + dur;

        card.innerHTML = `
            <div class="comic-card-text">
                <div class="orig-text">
                    <span class="time-editor">
                        <i class="fa-solid fa-clock"></i> 
                        <span class="time-adjust-group" title="Thời gian BẮT ĐẦU: Nhấp nút -/+ để chỉnh ±10ms hoặc tự nhập">
                            <button type="button" class="time-adjust-btn time-dec-start" title="Giảm 10 mili-giây (-10ms)">-</button>
                            <input type="text" class="time-in time-start" value="${formatTime(d.timestamp)}" title="Sửa thời gian BẮT ĐẦU (MM:SS hoặc giây)">
                            <button type="button" class="time-adjust-btn time-inc-start" title="Tăng 10 mili-giây (+10ms)">+</button>
                        </span>
                        <span class="time-arrow">➔</span> 
                        <span class="time-adjust-group" title="Thời gian KẾT THÚC: Nhấp nút -/+ để chỉnh ±10ms hoặc tự nhập">
                            <button type="button" class="time-adjust-btn time-dec-end" title="Giảm 10 mili-giây (-10ms)">-</button>
                            <input type="text" class="time-in time-end" value="${formatTime(endTs)}" title="Sửa thời gian KẾT THÚC (MM:SS hoặc giây)">
                            <button type="button" class="time-adjust-btn time-inc-end" title="Tăng 10 mili-giây (+10ms)">+</button>
                        </span>
                    </span>
                    <span style="font-weight: 700; color: #facc15;">#${d.id || ''}</span> ${escapeHtml(d.original)}
                </div>
                <div class="trans-text ${isFailed ? 'trans-failed' : ''}">
                    <i class="fa-solid ${isFailed ? 'fa-triangle-exclamation' : 'fa-comment-dots'}"></i> 
                    <span class="text-content" contenteditable="true" spellcheck="false" title="Click trực tiếp để chỉnh sửa bản dịch">${escapeHtml(d.translated || d.original)}</span>
                </div>
            </div>
            <div class="comic-card-actions">
                <button class="btn btn-xs btn-primary btn-ai-ocr" title="Tự động bóc tách chữ & dịch lại khung thoại này bằng AI"><i class="fa-solid fa-wand-magic-sparkles"></i> AI Đọc & Dịch</button>
                <button class="btn btn-xs btn-secondary btn-preview" title="Xem frame và kéo hộp thoại để đổi vị trí"><i class="fa-solid fa-eye"></i> Xem khung</button>
                <button class="btn btn-xs btn-danger btn-skip" title="Loại khỏi dịch và video xuất"><i class="fa-solid fa-ban"></i> Bỏ qua</button>
            </div>
        `;

        const attachTimeListeners = (cardElem) => {
            const timeStartIn = cardElem.querySelector('.time-start');
            const timeEndIn = cardElem.querySelector('.time-end');
            if (!timeStartIn || !timeEndIn) return;

            const updateTimeRange = () => {
                const parsedStart = parseTimeInput(timeStartIn.value);
                const parsedEnd = parseTimeInput(timeEndIn.value);

                let changed = false;
                if (parsedStart !== null && parsedStart >= 0) {
                    d.timestamp = parsedStart;
                    changed = true;
                }
                if (parsedEnd !== null && parsedEnd > d.timestamp) {
                    d.endTimestamp = parsedEnd;
                    d.duration = Math.max(0.1, parsedEnd - d.timestamp);
                    changed = true;
                }

                if (changed) {
                    detectedBubbles.sort((a, b) => a.timestamp - b.timestamp);
                    drawCanvasOverlay();
                }
            };

            const decStart = cardElem.querySelector('.time-dec-start');
            const incStart = cardElem.querySelector('.time-inc-start');
            const decEnd = cardElem.querySelector('.time-dec-end');
            const incEnd = cardElem.querySelector('.time-inc-end');

            if (decStart && incStart) {
                decStart.addEventListener('mousedown', (e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    d.timestamp = Math.round(Math.max(0, d.timestamp - 0.10) * 100) / 100;
                    timeStartIn.value = formatTime(d.timestamp);
                    updateTimeRange();
                });
                incStart.addEventListener('mousedown', (e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    d.timestamp = Math.round((d.timestamp + 0.10) * 100) / 100;
                    timeStartIn.value = formatTime(d.timestamp);
                    updateTimeRange();
                });
            }

            if (decEnd && incEnd) {
                decEnd.addEventListener('mousedown', (e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    const currentEnd = d.endTimestamp || (d.timestamp + dur);
                    d.endTimestamp = Math.round(Math.max(d.timestamp + 0.1, currentEnd - 0.10) * 100) / 100;
                    d.duration = Math.round((d.endTimestamp - d.timestamp) * 100) / 100;
                    timeEndIn.value = formatTime(d.endTimestamp);
                    updateTimeRange();
                });
                incEnd.addEventListener('mousedown', (e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    const currentEnd = d.endTimestamp || (d.timestamp + dur);
                    d.endTimestamp = Math.round((currentEnd + 0.10) * 100) / 100;
                    d.duration = Math.round((d.endTimestamp - d.timestamp) * 100) / 100;
                    timeEndIn.value = formatTime(d.endTimestamp);
                    updateTimeRange();
                });
            }

            ['input', 'change', 'blur'].forEach(evtType => {
                timeStartIn.addEventListener(evtType, updateTimeRange);
                timeEndIn.addEventListener(evtType, updateTimeRange);
            });

            timeStartIn.addEventListener('keydown', e => {
                if (e.key === 'Enter') {
                    updateTimeRange();
                    selectDialogue(d);
                    timeStartIn.blur();
                }
            });
            timeEndIn.addEventListener('keydown', e => {
                if (e.key === 'Enter') {
                    updateTimeRange();
                    selectDialogue(d);
                    timeEndIn.blur();
                }
            });
        };

        attachTimeListeners(card);

        const btnAiOcr = card.querySelector('.btn-ai-ocr');
        if (btnAiOcr) {
            btnAiOcr.addEventListener('click', async () => {
                if (isProcessing) return;
                if (!currentVideoPath) {
                    alert('Vui lòng chọn video trước!');
                    return;
                }

                showLoader(`AI đang đọc chữ & dịch khung thoại #${d.id}...`);
                setProcessing(true);

                try {
                    const res = await window.go.main.App.ScanCropWithAI(
                        currentVideoPath,
                        d.timestamp,
                        d.bbox,
                        scanLanguageSelect.value,
                        inputGeminiKey.value.trim(),
                        translationGlossary.value
                    );
                    hideLoader();

                    if (res && res.original) {
                        d.original = res.original;
                        d.translated = res.translated || res.original;
                        d.translationFailed = res.translationFailed;

                        const origDiv = card.querySelector('.orig-text');
                        if (origDiv) {
                            origDiv.innerHTML = `
                                <span class="time-editor" title="Nhấp vào từng con số để sửa thời gian xuất hiện (MM:SS hoặc số giây)">
                                    <i class="fa-solid fa-clock"></i> 
                                    [<input type="text" class="time-in time-start" value="${formatTime(d.timestamp)}" title="Sửa thời gian BẮT ĐẦU (MM:SS hoặc giây)"> ➔ 
                                    <input type="text" class="time-in time-end" value="${formatTime(d.timestamp + (d.duration || 2.5))}" title="Sửa thời gian KẾT THÚC (MM:SS hoặc giây)">]
                                </span>
                                <span style="font-weight: 700; color: #facc15;">#${d.id || ''}</span> ${escapeHtml(d.original)}
                            `;
                            attachTimeListeners(card);
                        }

                        const textSpan = card.querySelector('.text-content');
                        if (textSpan) {
                            textSpan.innerText = d.translated;
                        }

                        drawCanvasOverlay();
                        alert(`✅ AI đã đọc & dịch thành công thoại #${d.id}:\n\nGốc: ${d.original}\nDịch: ${d.translated}`);
                    }
                } catch (err) {
                    hideLoader();
                    alert('Lỗi AI đọc khung thoại: ' + (err.message || err));
                } finally {
                    setProcessing(false);
                }
            });
        }

        card.querySelector('.btn-preview').addEventListener('click', () => selectDialogue(d));
        card.querySelector('.btn-skip').addEventListener('click', () => {
            if (!confirm(`Bỏ qua thoại #${d.id} khỏi bản dịch và video xuất?`)) return;
            const savedScroll = comicStrip.scrollTop;
            detectedBubbles = detectedBubbles.filter(item => item.id !== d.id);
            if (selectedDialogueId === d.id) {
                selectedDialogueId = null;
                drawCanvasOverlay();
            }
            if (detectedBubbles.length === 0) renderEmptyStrip();
            else renderComicStrip(detectedBubbles, true);
        });

        // Support inline direct editing of translated text
        const textSpan = card.querySelector('.text-content');
        if (textSpan) {
            const saveEditedText = () => {
                const newText = textSpan.innerText.trim();
                d.translated = newText;
                d.translationFailed = false;
                card.classList.remove('failed-card');
                const icon = card.querySelector('.trans-text i');
                if (icon) icon.className = 'fa-solid fa-comment-dots';
                drawCanvasOverlay();
            };

            textSpan.addEventListener('input', saveEditedText);
            textSpan.addEventListener('blur', saveEditedText);
        }

        comicStrip.appendChild(card);
        if (autoScroll) {
            comicStrip.scrollTop = comicStrip.scrollHeight;
        }
    }

    function formatTime(seconds) {
        if (isNaN(seconds) || seconds === null) return '00:00.00';
        seconds = Math.round(seconds * 100) / 100;
        const total = Math.floor(seconds);
        const h = Math.floor(total / 3600);
        const m = Math.floor((total % 3600) / 60);
        const s = total % 60;
        const ms = Math.round((seconds - total) * 100);
        const msStr = ms.toString().padStart(2, '0');

        if (h > 0) {
            return `${h.toString().padStart(2, '0')}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}.${msStr}`;
        }
        return `${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}.${msStr}`;
    }

    function escapeHtml(text) {
        return (text || '').replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
    }

    function showLoader(text) {
        loaderText.textContent = text || 'Đang xử lý...';
        loader.classList.remove('hidden');
    }

    function hideLoader() {
        loader.classList.add('hidden');
    }

    btnClearStrip.addEventListener('click', () => {
        detectedBubbles = [];
        selectedDialogueId = null;
        drawCanvasOverlay();
        renderEmptyStrip();
    });

    btnClear.addEventListener('click', () => {
        detectedBubbles = [];
        selectedDialogueId = null;
        drawCanvasOverlay();
        renderEmptyStrip();
    });

    const btnSuggestGlossary = document.getElementById('btn-suggest-glossary');
    if (btnSuggestGlossary) {
        btnSuggestGlossary.addEventListener('click', async () => {
            if (isProcessing) return;
            if (!detectedBubbles || detectedBubbles.length === 0) {
                alert('Chưa có câu thoại OCR nào! Vui lòng thực hiện "1. Quét Frame & OCR" trước khi gợi ý.');
                return;
            }
            const apiKey = inputGeminiKey.value.trim();
            if (!apiKey) {
                alert('Vui lòng dán Gemini API Key ở ô góc trên bên phải để sử dụng AI!');
                return;
            }

            const originalHTML = btnSuggestGlossary.innerHTML;
            btnSuggestGlossary.disabled = true;
            btnSuggestGlossary.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i> Đang phân tích...';

            try {
                if (window.go && window.go.main && window.go.main.App) {
                    const suggestions = await window.go.main.App.SuggestGlossary(detectedBubbles, apiKey);
                    if (suggestions && suggestions.length > 0) {
                        const glossaryTextarea = document.getElementById('translation-glossary');
                        let currentText = glossaryTextarea ? glossaryTextarea.value.trim() : '';
                        const existingLines = new Set(currentText.split('\n').map(l => l.trim()).filter(Boolean));

                        let addedCount = 0;
                        const newLines = [];
                        for (const item of suggestions) {
                            if (item.source && item.target) {
                                const rule = `${item.source} => ${item.target}`;
                                if (!existingLines.has(rule)) {
                                    newLines.push(rule);
                                    addedCount++;
                                }
                            }
                        }

                        if (newLines.length > 0 && glossaryTextarea) {
                            glossaryTextarea.value = (currentText ? currentText + '\n' : '') + newLines.join('\n');
                            alert(`✨ Đã tự động thêm ${addedCount} quy tắc gợi ý nhân vật vào Glossary!`);
                        } else {
                            alert('Các gợi ý nhân vật từ AI đều đã có trong danh sách Glossary của bạn.');
                        }
                    } else {
                        alert('AI không phát hiện tên nhân vật hoặc danh xưng lặp lại đặc biệt nào từ bản OCR.');
                    }
                }
            } catch (err) {
                alert('Lỗi gợi ý Glossary: ' + (err.message || err));
            } finally {
                btnSuggestGlossary.disabled = false;
                btnSuggestGlossary.innerHTML = originalHTML;
            }
        });
    }

    if (btnCancelScan) {
        btnCancelScan.addEventListener('click', async () => {
            if (window.go && window.go.main && window.go.main.App) {
                scanProgressStatus.textContent = 'Đang dừng tiến trình quét...';
                try {
                    await window.go.main.App.CancelScan();
                } catch (e) {
                    console.error("CancelScan error:", e);
                }
            }
        });
    }
});
