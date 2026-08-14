(function () {
    const video = document.getElementById('video-player');
    const canvas = document.getElementById('overlay-canvas');
    const ctx = canvas.getContext('2d');
    const videoContainer = document.getElementById('video-container');

    const btnSelectVideo = document.getElementById('btn-select-video');
    const btnStartScan = document.getElementById('btn-start-scan');
    const btnFilterTrash = document.getElementById('btn-filter-trash');
    const btnOpenPronounModal = document.getElementById('btn-open-pronoun-modal');
    const btnTranslateAll = document.getElementById('btn-translate-all');
    const btnExportVideo = document.getElementById('btn-export-video');

    // Pronoun Modal Elements
    const pronounModal = document.getElementById('pronoun-modal');
    const btnClosePronounModal = document.getElementById('btn-close-pronoun-modal');
    const btnAddCharPair = document.getElementById('btn-add-char-pair');
    const btnAddGlossaryEntry = document.getElementById('btn-add-glossary-entry');
    const tbodyCharPairs = document.getElementById('tbody-char-pairs');
    const tbodyGlossary = document.getElementById('tbody-glossary');
    const txtPronounRules = document.getElementById('txt-pronoun-rules');
    const btnAIAutoContext = document.getElementById('btn-ai-auto-context');
    const btnBulkReplacePronouns = document.getElementById('btn-bulk-replace-pronouns');
    const btnSavePronounConfig = document.getElementById('btn-save-pronoun-config');

    // Player Control Bar Elements
    const btnPlayPause = document.getElementById('btn-play-pause');
    const btnPrevFrame = document.getElementById('btn-prev-frame');
    const btnNextFrame = document.getElementById('btn-next-frame');
    const videoSeekbar = document.getElementById('video-seekbar');
    const timeDisplay = document.getElementById('time-display');
    const btnToggleMute = document.getElementById('btn-toggle-mute');

    const statusOCR = document.getElementById('status-ocr');
    const statusGemini = document.getElementById('status-gemini');
    const textOCR = document.getElementById('text-ocr');
    const textGemini = document.getElementById('text-gemini');
    const geminiKeyInput = document.getElementById('gemini-key-input');

    const dialogueList = document.getElementById('dialogue-list');
    const dialogueCount = document.getElementById('dialogue-count');
    const chkShowTrash = document.getElementById('chk-show-trash');

    const progressModal = document.getElementById('progress-modal');
    const modalTitle = document.getElementById('modal-title');
    const modalTimer = document.getElementById('modal-timer');
    const modalProgressFill = document.getElementById('modal-progress-fill');
    const modalStatusText = document.getElementById('modal-status-text');
    const btnCancelModal = document.getElementById('btn-cancel-modal');
    const scanTimeBadge = document.getElementById('scan-time-badge');

    let modalTimerInterval = null;
    let modalStartTime = 0;

    function formatDuration(seconds) {
        const totalSec = Math.floor(seconds);
        const mins = Math.floor(totalSec / 60);
        const secs = totalSec % 60;
        const mStr = mins < 10 ? '0' + mins : mins;
        const sStr = secs < 10 ? '0' + secs : secs;
        return `${mStr}:${sStr}`;
    }

    function formatHumanDuration(seconds) {
        const totalSec = Math.round(seconds);
        const mins = Math.floor(totalSec / 60);
        const secs = totalSec % 60;
        if (mins > 0) {
            return `${mins} phút ${secs} giây`;
        }
        return `${secs} giây`;
    }

    let currentVideoPath = '';
    let dialogues = [];
    let selectedDialogueId = null;
    let isProcessing = false;
    let currentContextConfig = {
        presetStyle: 'ngon_tinh',
        pronounRules: '',
        characterPairs: [
            { source: 'Nam chính -> Nữ chính', target: 'Anh - Em' }
        ],
        glossaryEntries: []
    };

    // Interactive Drag / Resize State
    let isDragging = false;
    let dragTarget = null;
    let startX = 0, startY = 0;
    let initialBox = null;

    // Clear any previously saved API key from localStorage to ensure NO API KEY IS EVER SAVED
    localStorage.removeItem('gemini_api_key');
    geminiKeyInput.value = '';

    geminiKeyInput.addEventListener('input', () => {
        checkHealth();
    });
    geminiKeyInput.addEventListener('change', () => {
        checkHealth();
    });

    if (window.runtime && window.runtime.EventsOn) {
        window.runtime.EventsOn("system-health-changed", () => {
            checkHealth();
        });
    }

    setInterval(checkHealth, 3000);

    async function checkHealth() {
        if (!window.go || !window.go.main || !window.go.main.App) return;
        try {
            const status = await window.go.main.App.CheckSystemHealth(geminiKeyInput.value.trim());
            if (status.ocrOnline) {
                statusOCR.className = 'status-badge online';
                textOCR.textContent = `OCR: ${status.ocrEngine || 'Online'}`;
            } else {
                statusOCR.className = 'status-badge offline';
                textOCR.textContent = 'OCR Offline';
            }

            if (status.hasApiKey) {
                statusGemini.className = 'status-badge online';
                textGemini.textContent = 'Gemini Ready';
            } else {
                statusGemini.className = 'status-badge offline';
                textGemini.textContent = 'API Key Missing';
            }
        } catch (e) {
            console.error('CheckHealth error:', e);
        }
    }

    setInterval(checkHealth, 5000);
    setTimeout(checkHealth, 500);

    // Player Control Bar Event Listeners
    btnPlayPause.addEventListener('click', () => {
        if (!video.src) return;
        if (video.paused) {
            video.play();
            btnPlayPause.textContent = '⏸ Tạm dừng';
        } else {
            video.pause();
            btnPlayPause.textContent = '▶ Phát';
        }
    });

    btnPrevFrame.addEventListener('click', () => {
        if (!video.src) return;
        video.currentTime = Math.max(0, video.currentTime - 1.0);
        updateTimeDisplay();
        drawOverlay();
    });

    btnNextFrame.addEventListener('click', () => {
        if (!video.src) return;
        video.currentTime = Math.min(video.duration || 0, video.currentTime + 1.0);
        updateTimeDisplay();
        drawOverlay();
    });

    let seekRafId = null;
    function applyThrottledSeek() {
        if (!video.duration) return;
        const targetTime = (videoSeekbar.value / 100) * video.duration;
        video.currentTime = targetTime;
        drawOverlay();
        seekRafId = null;
    }

    videoSeekbar.addEventListener('input', () => {
        if (!video.duration) return;
        const targetTime = (videoSeekbar.value / 100) * video.duration;
        timeDisplay.textContent = `${formatTime(targetTime)} / ${formatTime(video.duration)}`;
        if (!seekRafId) {
            seekRafId = requestAnimationFrame(applyThrottledSeek);
        }
    });

    videoSeekbar.addEventListener('change', () => {
        if (!video.duration) return;
        if (seekRafId) {
            cancelAnimationFrame(seekRafId);
            seekRafId = null;
        }
        video.currentTime = (videoSeekbar.value / 100) * video.duration;
        updateTimeDisplay();
        drawOverlay();
    });

    btnToggleMute.addEventListener('click', () => {
        video.muted = !video.muted;
        btnToggleMute.textContent = video.muted ? '🔇' : '🔊';
    });

    function loopDraw() {
        if (!video.paused && !video.ended) {
            drawOverlay();
            requestAnimationFrame(loopDraw);
        }
    }

    video.addEventListener('play', () => {
        btnPlayPause.textContent = '⏸ Tạm dừng';
        requestAnimationFrame(loopDraw);
    });

    video.addEventListener('pause', () => {
        btnPlayPause.textContent = '▶ Phát';
    });

    video.addEventListener('timeupdate', () => {
        updateTimeDisplay();
        drawOverlay();
    });

    function updateTimeDisplay() {
        const cur = video.currentTime || 0;
        const dur = video.duration || 0;
        if (dur > 0) {
            videoSeekbar.value = (cur / dur) * 100;
        }
        timeDisplay.textContent = `${formatTime(cur)} / ${formatTime(dur)}`;
    }

    btnSelectVideo.addEventListener('click', async () => {
        if (!window.go || !window.go.main || !window.go.main.App) return;
        try {
            const filePath = await window.go.main.App.SelectVideoFile();
            if (filePath) {
                currentVideoPath = filePath;
                video.pause();
                video.src = `http://127.0.0.1:18989/video?path=${encodeURIComponent(filePath)}`;
                video.load();
                btnStartScan.disabled = false;
                dialogues = [];
                if (scanTimeBadge) {
                    scanTimeBadge.style.display = 'none';
                    scanTimeBadge.textContent = '';
                }
                renderDialogueList();
                drawOverlay();
            }
        } catch (e) {
            alert('Lỗi chọn video: ' + e);
        }
    });

    video.addEventListener('error', () => {
        console.warn('Video playback warning (non-fatal):', video.error);
        if (timeDisplay) {
            timeDisplay.textContent = 'Định dạng video (MKV/AVI/Codecs) không hỗ trợ xem trước trên trình duyệt, nhưng quét OCR & xuất video vẫn hoạt động 100% qua FFmpeg!';
        }
    });

    video.addEventListener('loadedmetadata', () => {
        if (videoContainer) {
            canvas.width = videoContainer.clientWidth;
            canvas.height = videoContainer.clientHeight;
        }
        updateTimeDisplay();
        drawOverlay();
    });

    const resizeObserver = new ResizeObserver(() => {
        if (videoContainer) {
            canvas.width = videoContainer.clientWidth;
            canvas.height = videoContainer.clientHeight;
            drawOverlay();
        }
    });
    if (videoContainer) resizeObserver.observe(videoContainer);

    function previewLayout() {
        if (!video.videoWidth || !video.videoHeight || !canvas.width || !canvas.height) return null;
        const scale = Math.min(canvas.width / video.videoWidth, canvas.height / video.videoHeight);
        const width = video.videoWidth * scale;
        const height = video.videoHeight * scale;
        const left = (canvas.width - width) / 2;
        const top = (canvas.height - height) / 2;
        return { scale, left, top, width, height };
    }

    function drawOverlay() {
        ctx.clearRect(0, 0, canvas.width, canvas.height);
        const layout = previewLayout();
        if (!layout) return;

        const currentTime = video.currentTime || 0;
        const showTrash = chkShowTrash.checked;
        const isPlaying = !video.paused && !video.ended;

        dialogues.forEach(d => {
            if (!showTrash && d.isTrash) return;

            const end = d.endTimestamp && d.endTimestamp > d.timestamp ? d.endTimestamp : (d.timestamp + (d.duration || 1.2));
            const isCurrent = (currentTime >= d.timestamp && currentTime <= end);
            const isSelected = (d.id === selectedDialogueId);

            const shouldRender = isPlaying ? isCurrent : (isCurrent || isSelected);

            if (shouldRender) {
                const bx = layout.left + d.bbox.x * layout.scale;
                const by = layout.top + d.bbox.y * layout.scale;
                const bw = d.bbox.width * layout.scale;
                const bh = d.bbox.height * layout.scale;

                ctx.save();

                const isSimsStyle = (document.getElementById('sel-subtitle-style')?.value === 'sims_bar');
                if (isSimsStyle) {
                    ctx.fillStyle = 'rgba(15, 15, 24, 0.90)';
                    ctx.beginPath();
                    ctx.roundRect(bx, by, bw, bh, 8);
                    ctx.fill();
                    ctx.strokeStyle = 'rgba(120, 120, 150, 0.6)';
                    ctx.lineWidth = 1.5;
                    ctx.stroke();
                } else {
                    ctx.fillStyle = '#ffffff';
                    ctx.beginPath();
                    ctx.roundRect(bx, by, bw, bh, 12);
                    ctx.fill();
                }

                const text = d.translated || d.original || '';
                ctx.fillStyle = isSimsStyle ? '#fffdf2' : '#000000';
                const fontSize = Math.max(12, 28 * (layout.height / 1080.0));
                ctx.font = isSimsStyle ? `italic 600 ${fontSize}px sans-serif` : `bold ${fontSize}px sans-serif`;
                ctx.textAlign = 'center';
                ctx.textBaseline = 'middle';

                const words = text.split(/\s+/);
                let lines = [];
                let currentLine = '';
                const maxWidth = bw * 0.85;

                words.forEach(w => {
                    const testLine = currentLine ? `${currentLine} ${w}` : w;
                    if (ctx.measureText(testLine).width > maxWidth && currentLine) {
                        lines.push(currentLine);
                        currentLine = w;
                    } else {
                        currentLine = testLine;
                    }
                });
                if (currentLine) lines.push(currentLine);

                const lineHeight = fontSize * 1.3;
                const startY = by + (bh / 2.0) - ((lines.length - 1) * lineHeight / 2.0);

                lines.forEach((line, idx) => {
                    ctx.fillText(line, bx + (bw / 2.0), startY + idx * lineHeight);
                });

                if (isSelected) {
                    ctx.strokeStyle = '#6366f1';
                    ctx.lineWidth = 2.5;
                    ctx.strokeRect(bx, by, bw, bh);

                    // Draw 8 interactive handles
                    const handleCoords = [
                        [bx, by],
                        [bx + bw, by],
                        [bx + bw, by + bh],
                        [bx, by + bh],
                        [bx + bw / 2, by],
                        [bx + bw, by + bh / 2],
                        [bx + bw / 2, by + bh],
                        [bx, by + bh / 2]
                    ];

                    handleCoords.forEach(([hx, hy]) => {
                        ctx.fillStyle = '#ffffff';
                        ctx.strokeStyle = '#6366f1';
                        ctx.lineWidth = 2;
                        ctx.beginPath();
                        ctx.rect(hx - 5, hy - 5, 10, 10);
                        ctx.fill();
                        ctx.stroke();
                    });
                }

                ctx.restore();
            }
        });
    }

    let dragMode = null;

    function getHandleAtPoint(mx, my, d, layout) {
        if (!d || !layout) return null;
        const bx = layout.left + d.bbox.x * layout.scale;
        const by = layout.top + d.bbox.y * layout.scale;
        const bw = d.bbox.width * layout.scale;
        const bh = d.bbox.height * layout.scale;

        const handles = [
            { type: 'resize-nw', x: bx, y: by },
            { type: 'resize-ne', x: bx + bw, y: by },
            { type: 'resize-se', x: bx + bw, y: by + bh },
            { type: 'resize-sw', x: bx, y: by + bh },
            { type: 'resize-n',  x: bx + bw / 2, y: by },
            { type: 'resize-e',  x: bx + bw, y: by + bh / 2 },
            { type: 'resize-s',  x: bx + bw / 2, y: by + bh },
            { type: 'resize-w',  x: bx, y: by + bh / 2 }
        ];

        const hitRadius = 10;
        for (const h of handles) {
            if (Math.abs(mx - h.x) <= hitRadius && Math.abs(my - h.y) <= hitRadius) {
                return h.type;
            }
        }

        if (mx >= bx && mx <= bx + bw && my >= by && my <= by + bh) {
            return 'move';
        }

        return null;
    }

    function getCursorForMode(mode) {
        switch (mode) {
            case 'resize-nw':
            case 'resize-se': return 'nwse-resize';
            case 'resize-ne':
            case 'resize-sw': return 'nesw-resize';
            case 'resize-n':
            case 'resize-s':  return 'ns-resize';
            case 'resize-e':
            case 'resize-w':  return 'ew-resize';
            case 'move':      return 'move';
            default:          return 'default';
        }
    }

    canvas.addEventListener('mousedown', (e) => {
        const layout = previewLayout();
        if (!layout) return;

        const rect = canvas.getBoundingClientRect();
        const mx = e.clientX - rect.left;
        const my = e.clientY - rect.top;

        let activeTarget = null;
        let mode = null;

        if (selectedDialogueId) {
            const selected = dialogues.find(d => d.id === selectedDialogueId);
            if (selected && (!selected.isTrash || chkShowTrash.checked)) {
                mode = getHandleAtPoint(mx, my, selected, layout);
                if (mode) {
                    activeTarget = selected;
                }
            }
        }

        if (!activeTarget) {
            const currentTime = video.currentTime || 0;
            for (let i = dialogues.length - 1; i >= 0; i--) {
                const d = dialogues[i];
                if (!chkShowTrash.checked && d.isTrash) continue;

                const end = d.endTimestamp && d.endTimestamp > d.timestamp ? d.endTimestamp : (d.timestamp + (d.duration || 1.2));
                const isCurrent = (currentTime >= d.timestamp && currentTime <= end);
                const isSelected = (d.id === selectedDialogueId);

                if (isCurrent || isSelected) {
                    const hitMode = getHandleAtPoint(mx, my, d, layout);
                    if (hitMode) {
                        activeTarget = d;
                        mode = hitMode;
                        break;
                    }
                }
            }
        }

        if (activeTarget && mode) {
            selectedDialogueId = activeTarget.id;
            isDragging = true;
            dragTarget = activeTarget;
            dragMode = mode;
            startX = mx;
            startY = my;
            initialBox = { ...activeTarget.bbox };
            updateActiveDialogueCard();
            drawOverlay();
        } else {
            selectedDialogueId = null;
            updateActiveDialogueCard();
            drawOverlay();
        }
    });

    window.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && selectedDialogueId !== null) {
            selectedDialogueId = null;
            updateActiveDialogueCard();
            drawOverlay();
        }
    });

    canvas.addEventListener('mousemove', (e) => {
        const layout = previewLayout();
        if (!layout) return;

        const rect = canvas.getBoundingClientRect();
        const mx = e.clientX - rect.left;
        const my = e.clientY - rect.top;

        if (!isDragging) {
            let cursor = 'default';
            if (selectedDialogueId) {
                const selected = dialogues.find(d => d.id === selectedDialogueId);
                if (selected) {
                    const hMode = getHandleAtPoint(mx, my, selected, layout);
                    if (hMode) cursor = getCursorForMode(hMode);
                }
            }
            canvas.style.cursor = cursor;
            return;
        }

        if (!dragTarget || !initialBox || !dragMode) return;
        canvas.style.cursor = getCursorForMode(dragMode);

        const dx = (mx - startX) / layout.scale;
        const dy = (my - startY) / layout.scale;

        let newX = initialBox.x;
        let newY = initialBox.y;
        let newW = initialBox.width;
        let newH = initialBox.height;

        const minSize = 20;

        switch (dragMode) {
            case 'move':
                newX = Math.max(0, initialBox.x + dx);
                newY = Math.max(0, initialBox.y + dy);
                break;
            case 'resize-se':
                newW = Math.max(minSize, initialBox.width + dx);
                newH = Math.max(minSize, initialBox.height + dy);
                break;
            case 'resize-nw':
                newW = Math.max(minSize, initialBox.width - dx);
                newH = Math.max(minSize, initialBox.height - dy);
                newX = initialBox.x + (initialBox.width - newW);
                newY = initialBox.y + (initialBox.height - newH);
                break;
            case 'resize-ne':
                newW = Math.max(minSize, initialBox.width + dx);
                newH = Math.max(minSize, initialBox.height - dy);
                newY = initialBox.y + (initialBox.height - newH);
                break;
            case 'resize-sw':
                newW = Math.max(minSize, initialBox.width - dx);
                newH = Math.max(minSize, initialBox.height + dy);
                newX = initialBox.x + (initialBox.width - newW);
                break;
            case 'resize-e':
                newW = Math.max(minSize, initialBox.width + dx);
                break;
            case 'resize-s':
                newH = Math.max(minSize, initialBox.height + dy);
                break;
            case 'resize-w':
                newW = Math.max(minSize, initialBox.width - dx);
                newX = initialBox.x + (initialBox.width - newW);
                break;
            case 'resize-n':
                newH = Math.max(minSize, initialBox.height - dy);
                newY = initialBox.y + (initialBox.height - newH);
                break;
        }

        dragTarget.bbox.x = newX;
        dragTarget.bbox.y = newY;
        dragTarget.bbox.width = newW;
        dragTarget.bbox.height = newH;
        drawOverlay();
    });

    window.addEventListener('mouseup', () => {
        isDragging = false;
        dragTarget = null;
        dragMode = null;
        initialBox = null;
        if (canvas) canvas.style.cursor = 'default';
    });

    const selVideoType = document.getElementById('sel-video-type');
    const selSubtitleStyle = document.getElementById('sel-subtitle-style');
    const btnExportSRT = document.getElementById('btn-export-srt');

    if (selVideoType && selSubtitleStyle) {
        selVideoType.addEventListener('change', () => {
            const selScanInterval = document.getElementById('sel-scan-interval');
            if (selVideoType.value === 'sims_en') {
                selSubtitleStyle.value = 'sims_bar';
                if (selScanInterval) selScanInterval.value = '0.50';
                currentContextConfig.presetStyle = 'hien_dai';
            } else {
                selSubtitleStyle.value = 'bubble';
                if (selScanInterval) selScanInterval.value = '0.30';
                currentContextConfig.presetStyle = 'ngon_tinh';
            }
            drawOverlay();
        });
    }

    if (selSubtitleStyle) {
        selSubtitleStyle.addEventListener('change', () => {
            drawOverlay();
        });
    }

    function updateToolbarButtons() {
        const hasDialogues = dialogues && dialogues.length > 0;
        const hasVideo = !!currentVideoPath;

        if (btnStartScan) btnStartScan.disabled = isProcessing || !hasVideo;

        const btnSnapAll = document.getElementById('btn-snap-all');
        if (btnSnapAll) btnSnapAll.disabled = isProcessing || !hasDialogues;

        const btnRefine30fpsAll = document.getElementById('btn-refine-30fps-all');
        if (btnRefine30fpsAll) btnRefine30fpsAll.disabled = isProcessing || !hasDialogues;

        if (btnFilterTrash) btnFilterTrash.disabled = isProcessing || !hasDialogues;
        if (btnOpenPronounModal) btnOpenPronounModal.disabled = isProcessing;
        if (btnTranslateAll) btnTranslateAll.disabled = isProcessing || !hasDialogues;
        const btnPolishScript = document.getElementById('btn-polish-script');
        if (btnPolishScript) btnPolishScript.disabled = isProcessing || !hasDialogues;
        if (btnExportSRT) btnExportSRT.disabled = isProcessing || !hasDialogues;
        if (btnExportVideo) btnExportVideo.disabled = isProcessing || !hasDialogues;
    }

    function initWailsEvents() {
        if (window.runtime && window.runtime.EventsOn) {
            window.runtime.EventsOn('scan-progress', (p) => {
                if (p.status) modalStatusText.textContent = p.status;
                if (p.totalFrames > 0) {
                    const pct = Math.min(100, Math.round((p.currentFrame / p.totalFrames) * 100));
                    modalProgressFill.style.width = pct + '%';
                }
            });
        } else {
            setTimeout(initWailsEvents, 150);
        }
    }
    initWailsEvents();

    btnStartScan.addEventListener('click', async () => {
        if (!currentVideoPath || isProcessing) return;
        if (!confirm('Bạn có chắc chắn muốn BẮT ĐẦU QUÉT THOẠI từ video không?')) return;
        isProcessing = true;
        updateToolbarButtons();
        showModal('⚡ Đang Quét Thoại (DirectML GPU OCR)...');
        const scanStartTime = Date.now();

        try {
            const selScanInterval = document.getElementById('sel-scan-interval');
            const intervalVal = selScanInterval ? parseFloat(selScanInterval.value) : 0.30;
            const videoTypeVal = selVideoType ? selVideoType.value : 'comic_cjk';
            const subStyleVal = selSubtitleStyle ? selSubtitleStyle.value : 'bubble';
            const sourceLangVal = (videoTypeVal === 'sims_en') ? 'en' : 'chi_sim';

            const config = {
                videoPath: currentVideoPath,
                interval: intervalVal,
                threshold: 240,
                sourceLang: sourceLangVal,
                videoType: videoTypeVal,
                subtitleStyle: subStyleVal,
                geminiKey: geminiKeyInput.value.trim()
            };

            const results = await window.go.main.App.StartFullPipelineScan(config);
            const totalElapsedSec = (Date.now() - scanStartTime) / 1000;
            const timeFormatted = formatHumanDuration(totalElapsedSec);

            if (results && results.length > 0) {
                dialogues = results;
            }
            renderDialogueList();
            drawOverlay();

            if (scanTimeBadge) {
                scanTimeBadge.textContent = `(Quét xong trong: ${timeFormatted})`;
                scanTimeBadge.style.display = 'inline';
            }
        } catch (e) {
            const errStr = String(e);
            if (!errStr.includes('canceled') && !errStr.includes('cancelled') && !errStr.includes('context')) {
                alert('Lỗi quét thoại: ' + e);
            }
        } finally {
            isProcessing = false;
            hideModal();
            updateToolbarButtons();
        }
    });

    const btnAddDialogue = document.getElementById('btn-add-dialogue');
    if (btnAddDialogue) {
        btnAddDialogue.addEventListener('click', () => {
            const currentTime = video.currentTime || 0;
            const maxId = dialogues.reduce((max, d) => (d.id > max ? d.id : max), 0);
            const newId = maxId + 1;

            const videoW = video.videoWidth || 1920;
            const videoH = video.videoHeight || 1080;

            const newDialogue = {
                id: newId,
                frameIndex: Math.floor(currentTime * 30),
                timestamp: Math.max(0, currentTime - 0.2),
                endTimestamp: currentTime + 2.0,
                duration: 2.2,
                original: "Thoại thủ công #" + newId,
                translated: "",
                isTrash: false,
                bbox: {
                    x: Math.round(videoW * 0.25),
                    y: Math.round(videoH * 0.35),
                    width: Math.round(videoW * 0.50),
                    height: Math.round(videoH * 0.08)
                }
            };

            dialogues.push(newDialogue);
            dialogues.sort((a, b) => a.timestamp - b.timestamp);
            selectedDialogueId = newId;

            renderDialogueList();
            drawOverlay();
            updateToolbarButtons();

            setTimeout(() => {
                const activeCard = dialogueList.querySelector(`.dialogue-card[data-id="${newId}"]`);
                if (activeCard) {
                    activeCard.scrollIntoView({ behavior: 'smooth', block: 'center' });
                    const transInput = activeCard.querySelector('.translated-input');
                    if (transInput) transInput.focus();
                }
            }, 50);
        });
    }

    const btnSnapAll = document.getElementById('btn-snap-all');
    if (btnSnapAll) {
        btnSnapAll.addEventListener('click', async () => {
            if (dialogues.length === 0 || isProcessing) return;
            if (!confirm('Bạn có chắc chắn muốn TỰ ĐỘNG CĂN VỊ TRÍ hộp thoại cho tất cả thoại không?')) return;
            isProcessing = true;
            updateToolbarButtons();
            const apiKey = geminiKeyInput.value.trim();
            const method = apiKey ? 'AI (Gemini Vision)' : 'OCR Fallback';
            showModal(`🤖 [${method}] Đang Tự Động Căn Vị Trí Hộp Thoại Đè Thoại Gốc...`);

            try {
                const cleanDialogues = dialogues.filter(d => !d.isTrash);
                const snapped = await window.go.main.App.SnapAllDialoguesBBoxes(currentVideoPath, cleanDialogues, apiKey);
                if (snapped && snapped.length > 0) {
                    const mapSnap = new Map(snapped.map(s => [s.id, s]));
                    dialogues.forEach(d => {
                        if (mapSnap.has(d.id)) {
                            d.bbox = mapSnap.get(d.id).bbox;
                        }
                    });
                }
                renderDialogueList();
                drawOverlay();
            } catch (e) {
                const errStr = String(e);
                if (!errStr.includes('canceled') && !errStr.includes('cancelled') && !errStr.includes('context')) {
                    alert('Lỗi Căn Hộp Thoại AI: ' + e);
                }
            } finally {
                isProcessing = false;
                hideModal();
                updateToolbarButtons();
            }
        });
    }

    const btnRefine30fpsAll = document.getElementById('btn-refine-30fps-all');
    if (btnRefine30fpsAll) {
        btnRefine30fpsAll.addEventListener('click', async () => {
            if (dialogues.length === 0 || isProcessing) return;
            if (!confirm('Bạn có chắc chắn muốn TINH CHỈNH MỐC THỜI GIAN 30 FPS cho tất cả thoại không?')) return;
            isProcessing = true;
            updateToolbarButtons();
            showModal('🎯 Đang Tinh Chỉnh Mốc Thời Gian 30 FPS Cho Tất Cả Thoại...');

            try {
                const selScanInterval = document.getElementById('sel-scan-interval');
                const intervalVal = selScanInterval ? parseFloat(selScanInterval.value) : 0.30;
                const cleanDialogues = dialogues.filter(d => !d.isTrash);
                const refined = await window.go.main.App.RefineAllDialoguesTimestamps(currentVideoPath, cleanDialogues, intervalVal);

                if (refined && refined.length > 0) {
                    const mapRefined = new Map(refined.map(r => [r.id, r]));
                    dialogues.forEach(d => {
                        if (mapRefined.has(d.id)) {
                            const r = mapRefined.get(d.id);
                            d.timestamp = r.timestamp;
                            d.endTimestamp = r.endTimestamp;
                            d.duration = r.duration;
                        }
                    });
                }
                renderDialogueList();
                drawOverlay();
            } catch (e) {
                const errStr = String(e);
                if (!errStr.includes('canceled') && !errStr.includes('cancelled') && !errStr.includes('context')) {
                    alert('Lỗi tinh chỉnh 30 FPS: ' + e);
                }
            } finally {
                isProcessing = false;
                hideModal();
                updateToolbarButtons();
            }
        });
    }

    btnFilterTrash.addEventListener('click', async () => {
        if (dialogues.length === 0 || isProcessing) return;
        if (!confirm('Bạn có chắc chắn muốn LỌC KÝ TỰ RÁC các câu thoại lỗi không?')) return;
        const key = geminiKeyInput.value.trim();

        isProcessing = true;
        updateToolbarButtons();
        showModal('🧹 AI & Quy Tắc Đang Lọc Ký Tự Rác/Watermark...');

        try {
            const filtered = await window.go.main.App.FilterTrashWithAI(key, dialogues);
            dialogues = filtered || dialogues;
            renderDialogueList();
            drawOverlay();
        } catch (e) {
            const errStr = String(e);
            if (!errStr.includes('canceled') && !errStr.includes('cancelled') && !errStr.includes('context')) {
                alert('Lỗi lọc rác: ' + e);
            }
        } finally {
            isProcessing = false;
            hideModal();
            updateToolbarButtons();
        }
    });

    // --- PRONOUN MODAL LOGIC & HANDLERS ---
    function renderPronounModal() {
        if (!tbodyCharPairs || !tbodyGlossary) return;
        tbodyCharPairs.innerHTML = '';
        tbodyGlossary.innerHTML = '';

        if (pronounModal) {
            const presetBtns = pronounModal.querySelectorAll('.chip-btn');
            presetBtns.forEach(btn => {
                btn.classList.toggle('active', btn.dataset.preset === currentContextConfig.presetStyle);
            });
        }

        if (currentContextConfig.characterPairs && currentContextConfig.characterPairs.length > 0) {
            currentContextConfig.characterPairs.forEach(cp => addCharPairRow(cp.source, cp.target));
        } else {
            addCharPairRow('Nam chính -> Nữ chính', 'Anh - Em');
        }

        if (currentContextConfig.glossaryEntries && currentContextConfig.glossaryEntries.length > 0) {
            currentContextConfig.glossaryEntries.forEach(ge => addGlossaryRow(ge.source, ge.target));
        }

        if (txtPronounRules) {
            txtPronounRules.value = currentContextConfig.pronounRules || '';
        }
    }

    function addCharPairRow(source = '', target = '') {
        if (!tbodyCharPairs) return;
        const tr = document.createElement('tr');
        tr.innerHTML = `
            <td><input type="text" class="input-char-source" value="${escapeHtmlAttr(source)}" placeholder="Ví dụ: Nam chính -> Nữ chính"></td>
            <td><input type="text" class="input-char-target" value="${escapeHtmlAttr(target)}" placeholder="Ví dụ: Anh - Em"></td>
            <td style="text-align: center;"><button type="button" class="btn btn-sm btn-delete-row" style="padding: 2px 6px; color: var(--accent-rose);">&times;</button></td>
        `;
        tr.querySelector('.btn-delete-row').addEventListener('click', () => tr.remove());
        tbodyCharPairs.appendChild(tr);
    }

    function addGlossaryRow(source = '', target = '') {
        if (!tbodyGlossary) return;
        const tr = document.createElement('tr');
        tr.innerHTML = `
            <td><input type="text" class="input-glossary-source" value="${escapeHtmlAttr(source)}" placeholder="Ví dụ: 凌天"></td>
            <td><input type="text" class="input-glossary-target" value="${escapeHtmlAttr(target)}" placeholder="Ví dụ: Lăng Thiên"></td>
            <td style="text-align: center;"><button type="button" class="btn btn-sm btn-delete-row" style="padding: 2px 6px; color: var(--accent-rose);">&times;</button></td>
        `;
        tr.querySelector('.btn-delete-row').addEventListener('click', () => tr.remove());
        tbodyGlossary.appendChild(tr);
    }

    function escapeHtmlAttr(str) {
        if (!str) return '';
        return String(str).replace(/"/g, '&quot;');
    }

    function collectPronounConfigFromModal() {
        const charPairs = [];
        if (tbodyCharPairs) {
            tbodyCharPairs.querySelectorAll('tr').forEach(tr => {
                const src = tr.querySelector('.input-char-source')?.value.trim() || '';
                const tgt = tr.querySelector('.input-char-target')?.value.trim() || '';
                if (src || tgt) {
                    charPairs.push({ source: src, target: tgt });
                }
            });
        }

        const glossaryEntries = [];
        if (tbodyGlossary) {
            tbodyGlossary.querySelectorAll('tr').forEach(tr => {
                const src = tr.querySelector('.input-glossary-source')?.value.trim() || '';
                const tgt = tr.querySelector('.input-glossary-target')?.value.trim() || '';
                if (src || tgt) {
                    glossaryEntries.push({ source: src, target: tgt });
                }
            });
        }

        currentContextConfig.characterPairs = charPairs;
        currentContextConfig.glossaryEntries = glossaryEntries;
        currentContextConfig.pronounRules = txtPronounRules ? txtPronounRules.value.trim() : '';
    }

    function applyPresetStyle(presetKey) {
        currentContextConfig.presetStyle = presetKey;
        switch (presetKey) {
            case 'ngon_tinh':
                currentContextConfig.characterPairs = [
                    { source: 'Nam chính -> Nữ chính', target: 'Anh - Em' },
                    { source: 'Nam chính -> Bạn gái / Người ngoài', target: 'Tôi - Cô / Tôi - Anh' }
                ];
                currentContextConfig.pronounRules = 'Thể loại Ngôn tình / Đô thị. Nam chính xưng Anh gọi Em với Nữ chính. Thể hiện sự ngọt ngào, nhất quán.';
                break;
            case 'tien_hiep':
                currentContextConfig.characterPairs = [
                    { source: 'Nhân vật chính -> Kẻ thù', target: 'Ta - Ngươi' },
                    { source: 'Sư huynh -> Sư đệ', target: 'Huynh - Đệ' },
                    { source: 'Tiểu bối -> Tiền bối', target: 'Vãn bối - Tiền bối' }
                ];
                currentContextConfig.pronounRules = 'Thể loại Tiên hiệp / Cổ trang. Xưng Ta gọi Ngươi với đối thủ. Tuyệt đối KHÔNG dùng xưng hô hiện đại như Tôi/Cậu/Anh/Em.';
                break;
            case 'ban_be':
                currentContextConfig.characterPairs = [
                    { source: 'Bạn thân', target: 'Tớ - Cậu / Mày - Tớ' },
                    { source: 'Bạn cùng lớp', target: 'Tớ - Cậu' }
                ];
                currentContextConfig.pronounRules = 'Thể loại Học đường / Bạn bè. Xưng hô thân mật Tớ - Cậu hoặc Mày - Tớ.';
                break;
            case 'hien_dai':
                currentContextConfig.characterPairs = [
                    { source: 'Đồng nghiệp', target: 'Tôi - Anh / Chị' },
                    { source: 'Sếp -> Nhân viên', target: 'Tôi - Cậu' }
                ];
                currentContextConfig.pronounRules = 'Thể loại Công sở / Hiện đại. Xưng hô lịch sự, chuyên nghiệp Tôi - Anh/Chị.';
                break;
        }
        renderPronounModal();
    }

    if (btnOpenPronounModal) {
        btnOpenPronounModal.addEventListener('click', () => {
            renderPronounModal();
            if (pronounModal) pronounModal.style.display = 'flex';
        });
    }

    if (btnClosePronounModal) {
        btnClosePronounModal.addEventListener('click', () => {
            if (pronounModal) pronounModal.style.display = 'none';
        });
    }

    if (pronounModal) {
        pronounModal.addEventListener('click', (e) => {
            if (e.target === pronounModal) {
                pronounModal.style.display = 'none';
            }
        });

        const presetBtns = pronounModal.querySelectorAll('.chip-btn');
        presetBtns.forEach(btn => {
            btn.addEventListener('click', () => {
                applyPresetStyle(btn.dataset.preset);
            });
        });
    }

    if (btnAddCharPair) {
        btnAddCharPair.addEventListener('click', () => addCharPairRow());
    }

    if (btnAddGlossaryEntry) {
        btnAddGlossaryEntry.addEventListener('click', () => addGlossaryRow());
    }

    if (btnSavePronounConfig) {
        btnSavePronounConfig.addEventListener('click', () => {
            collectPronounConfigFromModal();
            if (pronounModal) pronounModal.style.display = 'none';
            alert('🎉 Đã lưu quy tắc xưng hô!');
        });
    }

    if (btnAIAutoContext) {
        btnAIAutoContext.addEventListener('click', async () => {
            if (dialogues.length === 0 || isProcessing) {
                alert('Vui lòng mở video và quét thoại trước khi gọi AI phân tích!');
                return;
            }
            const key = geminiKeyInput.value.trim();
            if (!key) {
                alert('Vui lòng nhập Gemini API Key ở thanh tiêu đề!');
                return;
            }

            isProcessing = true;
            updateToolbarButtons();
            showModal('🧠 AI Đang Phân Tích Kịch Bản & Gợi Ý Quy Tắc Xưng Hô (Gemini 3.5 Flash Lite)...');

            try {
                const configRes = await window.go.main.App.AnalyzeContextAndGlossary(key, dialogues);
                if (configRes) {
                    if (configRes.presetStyle) currentContextConfig.presetStyle = configRes.presetStyle;
                    if (configRes.pronounRules) currentContextConfig.pronounRules = configRes.pronounRules;
                    if (configRes.characterPairs && configRes.characterPairs.length > 0) {
                        currentContextConfig.characterPairs = configRes.characterPairs;
                    }
                    if (configRes.glossaryEntries && configRes.glossaryEntries.length > 0) {
                        currentContextConfig.glossaryEntries = configRes.glossaryEntries;
                    }
                    renderPronounModal();
                    alert('🎉 AI đã đọc kịch bản và tự động lập bảng xưng hô đề xuất!');
                }
            } catch (e) {
                const errStr = String(e);
                if (!errStr.includes('canceled') && !errStr.includes('cancelled') && !errStr.includes('context')) {
                    alert('Lỗi phân tích ngữ cảnh AI: ' + e);
                }
            } finally {
                isProcessing = false;
                hideModal();
                updateToolbarButtons();
            }
        });
    }

    if (btnBulkReplacePronouns) {
        btnBulkReplacePronouns.addEventListener('click', async () => {
            if (dialogues.length === 0) {
                alert('Chưa có câu thoại nào để đổi xưng hô!');
                return;
            }
            collectPronounConfigFromModal();

            // Prepare replacement list from character pairs and glossary
            const replacements = [];
            currentContextConfig.characterPairs.forEach(cp => {
                if (cp.source && cp.target && cp.target.includes('-')) {
                    const parts = cp.target.split('-').map(s => s.trim());
                    if (parts.length >= 2) {
                        // Optional simple replacement pair
                    }
                }
            });

            currentContextConfig.glossaryEntries.forEach(ge => {
                if (ge.source && ge.target) {
                    replacements.push({ source: ge.source, target: ge.target });
                }
            });

            if (replacements.length === 0) {
                alert('Vui lòng thêm ít nhất một cặp từ/tên ở bảng "Glossary" hoặc "Xưng hô nhân vật" để thực hiện đổi hàng loạt!');
                return;
            }

            try {
                const updated = await window.go.main.App.BulkReplacePronouns(dialogues, replacements);
                if (updated && updated.length > 0) {
                    dialogues = updated;
                    renderDialogueList();
                    drawOverlay();
                    alert(`🎉 Đã áp dụng thay thế xưng hô / thuật ngữ hàng loạt cho ${dialogues.length} thoại!`);
                }
            } catch (e) {
                alert('Lỗi đổi xưng hô hàng loạt: ' + e);
            }
        });
    }

    btnTranslateAll.addEventListener('click', async () => {
        if (dialogues.length === 0 || isProcessing) return;
        const key = geminiKeyInput.value.trim();
        if (!key) {
            alert('Vui lòng nhập Gemini API Key ở thanh tiêu đề!');
            return;
        }
        if (!confirm('Bạn có chắc chắn muốn BẮT ĐẦU DỊCH AI & Trau Chuốt Mạch Hội Thoại tất cả câu thoại không?')) return;

        collectPronounConfigFromModal();

        isProcessing = true;
        updateToolbarButtons();
        showModal('🌐 Đang Dịch AI & Đọc Lại Trau Chuốt Mạch Hội Thoại (Gemini 3.5 Flash Lite)...');

        try {
            const contextJSON = JSON.stringify(currentContextConfig);
            const translated = await window.go.main.App.TranslateDialogues(key, dialogues, contextJSON);
            dialogues = translated || dialogues;

            renderDialogueList();
            drawOverlay();
        } catch (e) {
            const errStr = String(e);
            if (!errStr.includes('canceled') && !errStr.includes('cancelled') && !errStr.includes('context')) {
                alert('Lỗi dịch thoại: ' + e);
            }
        } finally {
            isProcessing = false;
            hideModal();
            updateToolbarButtons();
        }
    });

    const btnPolishScript = document.getElementById('btn-polish-script');
    if (btnPolishScript) {
        btnPolishScript.addEventListener('click', async () => {
            if (dialogues.length === 0 || isProcessing) return;
            const key = geminiKeyInput.value.trim();
            if (!key) {
                alert('Vui lòng nhập Gemini API Key ở thanh tiêu đề!');
                return;
            }
            if (!confirm('Bạn có chắc chắn muốn cho AI ĐỌC LẠI TOÀN BỘ KỊCH BẢN và trau chuốt cho thoại liền mạch hơn không?')) return;

            collectPronounConfigFromModal();

            isProcessing = true;
            updateToolbarButtons();
            showModal('✨ AI Đang Đọc Lại Toàn Bộ Kịch Bản & Trau Chuốt Mạch Hội Thoại (Gemini 3.5 Flash Lite)...');

            try {
                const contextJSON = JSON.stringify(currentContextConfig);
                const polished = await window.go.main.App.RefineFullScriptTranslation(key, dialogues, contextJSON);
                dialogues = polished || dialogues;
                renderDialogueList();
                drawOverlay();
                alert('🎉 Đã trau chuốt và tinh chỉnh lại toàn bộ thoại liền mạch thành công!');
            } catch (e) {
                const errStr = String(e);
                if (!errStr.includes('canceled') && !errStr.includes('cancelled') && !errStr.includes('context')) {
                    alert('Lỗi trau chuốt kịch bản: ' + e);
                }
            } finally {
                isProcessing = false;
                hideModal();
                updateToolbarButtons();
            }
        });
    }

    const btnExportJSON = document.getElementById('btn-export-json');
    if (btnExportJSON) {
        btnExportJSON.addEventListener('click', async () => {
            if (dialogues.length === 0) {
                alert('Chưa có dữ liệu thoại để xuất!');
                return;
            }
            try {
                const jsonContent = JSON.stringify(dialogues, null, 2);
                const saved = await window.go.main.App.SaveJSONFile(jsonContent, 'backup_dialogues.json');
                if (saved) {
                    alert('🎉 Lưu file JSON dự phòng thành công:\n' + saved);
                }
            } catch (e) {
                alert('Lỗi xuất JSON: ' + e);
            }
        });
    }

    const btnImportJSON = document.getElementById('btn-import-json');
    if (btnImportJSON) {
        btnImportJSON.addEventListener('click', async () => {
            try {
                const jsonContent = await window.go.main.App.SelectJSONFile();
                if (jsonContent) {
                    const parsed = JSON.parse(jsonContent);
                    if (Array.isArray(parsed) && parsed.length > 0) {
                        dialogues = parsed;
                        renderDialogueList();
                        drawOverlay();
                        updateToolbarButtons();
                        alert(`🎉 Đã nhập thành công ${dialogues.length} câu thoại từ file JSON!`);
                    } else {
                        alert('File JSON không chứa danh sách thoại hợp lệ!');
                    }
                }
            } catch (e) {
                alert('Lỗi nhập JSON: ' + e);
            }
        });
    }

    btnExportVideo.addEventListener('click', async () => {
        if (!currentVideoPath || dialogues.length === 0 || isProcessing) return;
        if (!confirm('Bạn có chắc chắn muốn BẮT ĐẦU XUẤT VIDEO MP4 không?')) return;
        try {
            let defaultName = 'video_vietsub.mp4';
            if (currentVideoPath) {
                const slashIndex = Math.max(currentVideoPath.lastIndexOf('/'), currentVideoPath.lastIndexOf('\\'));
                const filename = slashIndex >= 0 ? currentVideoPath.substring(slashIndex + 1) : currentVideoPath;
                const dotIndex = filename.lastIndexOf('.');
                const nameWithoutExt = dotIndex > 0 ? filename.substring(0, dotIndex) : filename;
                defaultName = `${nameWithoutExt}_vietsub.mp4`;
            }

            const savePath = await window.go.main.App.SelectSaveLocation(defaultName);
            if (!savePath) return;

            isProcessing = true;
            updateToolbarButtons();
            showModal('💾 Đang Xuất Video qua GPU FFmpeg...');

            const activeDialogues = dialogues.filter(d => !d.isTrash);
            const subStyle = selSubtitleStyle ? selSubtitleStyle.value : 'bubble';
            const outPath = await window.go.main.App.ExportVideoWithOptions(currentVideoPath, JSON.stringify(activeDialogues), savePath, subStyle);
            alert('🎉 Xuất video thành công:\n' + outPath);
        } catch (e) {
            const errStr = String(e);
            if (!errStr.includes('canceled') && !errStr.includes('cancelled') && !errStr.includes('context')) {
                alert('Lỗi xuất video: ' + e);
            }
        } finally {
            isProcessing = false;
            hideModal();
            updateToolbarButtons();
        }
    });

    if (btnExportSRT) {
        btnExportSRT.addEventListener('click', async () => {
            if (dialogues.length === 0 || isProcessing) return;
            try {
                let defaultName = 'subtitles_vietsub.srt';
                if (currentVideoPath) {
                    const slashIndex = Math.max(currentVideoPath.lastIndexOf('/'), currentVideoPath.lastIndexOf('\\'));
                    const filename = slashIndex >= 0 ? currentVideoPath.substring(slashIndex + 1) : currentVideoPath;
                    const dotIndex = filename.lastIndexOf('.');
                    const nameWithoutExt = dotIndex > 0 ? filename.substring(0, dotIndex) : filename;
                    defaultName = `${nameWithoutExt}_vietsub.srt`;
                }

                const activeDialogues = dialogues.filter(d => !d.isTrash);
                if (activeDialogues.length === 0) {
                    alert('Không có câu thoại nào để xuất file SRT!');
                    return;
                }

                const outPath = await window.go.main.App.ExportSRT(JSON.stringify(activeDialogues), defaultName);
                if (outPath) {
                    alert('🎉 Xuất file phụ đề SRT thành công:\n' + outPath);
                }
            } catch (e) {
                const errStr = String(e);
                if (!errStr.includes('canceled') && !errStr.includes('cancelled') && !errStr.includes('context')) {
                    alert('Lỗi xuất file SRT: ' + e);
                }
            }
        });
    }

    function updateActiveDialogueCard() {
        const cards = dialogueList.querySelectorAll('.dialogue-card');
        cards.forEach(card => {
            const cardId = parseInt(card.dataset.id, 10);
            if (cardId === selectedDialogueId) {
                if (!card.classList.contains('active')) {
                    card.classList.add('active');
                    card.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
                }
            } else {
                card.classList.remove('active');
            }
        });
    }

    function parseTimeString(str) {
        if (!str) return NaN;
        const parts = str.trim().split(':');
        if (parts.length === 2) {
            const m = parseFloat(parts[0]);
            const s = parseFloat(parts[1]);
            if (!isNaN(m) && !isNaN(s)) return m * 60 + s;
        }
        return parseFloat(str);
    }

    function renderDialogueList() {
        dialogueCount.textContent = dialogues.filter(d => chkShowTrash.checked || !d.isTrash).length;
        dialogueList.innerHTML = '';

        if (dialogues.length === 0) {
            dialogueList.innerHTML = `
                <div style="text-align: center; color: var(--text-muted); margin-top: 40px; font-size: 0.88rem;">
                    Chưa có thoại nào.<br>Hãy chọn video và nhấn <b>Quét Thoại</b> để bắt đầu.
                </div>`;
            return;
        }

        dialogues.forEach((d) => {
            if (!chkShowTrash.checked && d.isTrash) return;

            const card = document.createElement('div');
            card.className = `dialogue-card ${d.id === selectedDialogueId ? 'active' : ''}`;
            card.dataset.id = d.id;

            const startTimeStr = formatTime(d.timestamp);
            const endTimeStr = formatTime(d.endTimestamp || (d.timestamp + (d.duration || 1.2)));

            card.innerHTML = `
                <div class="card-header">
                    <span>Thoại #${d.id} ${d.id === selectedDialogueId ? '<button class="btn btn-sm btn-deselect" style="padding: 1px 6px; font-size: 0.72rem; margin-left: 6px; background: rgba(239, 68, 68, 0.2); color: #f87171; border: 1px solid rgba(239, 68, 68, 0.3);" title="Bỏ chọn câu thoại này">✖ Bỏ chọn</button>' : ''}</span>
                    <div style="display: flex; align-items: center; gap: 4px;">
                        <input type="text" class="time-input time-start" value="${startTimeStr}">
                        <span>-</span>
                        <input type="text" class="time-input time-end" value="${endTimeStr}">
                    </div>
                </div>

                <div class="card-body">
                    <div class="original-text">${escapeHtml(d.original)}</div>
                    <input type="text" class="translated-input" value="${escapeHtml(d.translated || '')}" placeholder="Nhập bản dịch tiếng Việt...">
                </div>

                <div class="card-actions">
                    <button class="btn btn-sm btn-translate-single" style="background: rgba(99, 102, 241, 0.15); color: #818cf8; border: 1px solid rgba(99, 102, 241, 0.3);" title="Dịch riêng câu thoại này bằng AI">🌐 Dịch AI</button>
                    <button class="btn btn-sm btn-refine-fps" title="Tinh chỉnh mốc thời gian 30 FPS cho riêng thoại này">🎯 30 FPS</button>
                    <button class="btn btn-sm btn-snap" title="Tự động dùng AI căn vị trí hộp thoại đè kín thoại gốc">🤖 AI Fit</button>
                    <button class="btn btn-sm btn-trash" style="color: var(--accent-rose);">${d.isTrash ? '↺ Phục hồi' : '🗑 Xóa'}</button>
                </div>
            `;

            card.addEventListener('click', (e) => {
                if (e.target.tagName === 'INPUT' || e.target.tagName === 'BUTTON') return;
                selectedDialogueId = d.id;
                video.currentTime = d.timestamp;
                updateActiveDialogueCard();
                drawOverlay();
            });

            const btnDeselect = card.querySelector('.btn-deselect');
            if (btnDeselect) {
                btnDeselect.addEventListener('click', (e) => {
                    e.stopPropagation();
                    selectedDialogueId = null;
                    renderDialogueList();
                    drawOverlay();
                });
            }

            const inputStart = card.querySelector('.time-start');
            inputStart.addEventListener('change', () => {
                const parsed = parseTimeString(inputStart.value);
                if (!isNaN(parsed) && parsed >= 0) {
                    d.timestamp = parsed;
                    drawOverlay();
                }
            });

            const inputEnd = card.querySelector('.time-end');
            inputEnd.addEventListener('change', () => {
                const parsed = parseTimeString(inputEnd.value);
                if (!isNaN(parsed) && parsed > d.timestamp) {
                    d.endTimestamp = parsed;
                    d.duration = parsed - d.timestamp;
                    drawOverlay();
                }
            });

            const inputTrans = card.querySelector('.translated-input');
            inputTrans.addEventListener('input', () => {
                d.translated = inputTrans.value;
                drawOverlay();
            });

            const btnTransSingle = card.querySelector('.btn-translate-single');
            if (btnTransSingle) {
                btnTransSingle.addEventListener('click', async () => {
                    if (isProcessing) return;
                    const key = geminiKeyInput.value.trim();
                    if (!key) {
                        alert('Vui lòng nhập Gemini API Key ở thanh tiêu đề!');
                        return;
                    }
                    btnTransSingle.disabled = true;
                    btnTransSingle.innerText = '🌐 Đang dịch & Quét AI...';
                    try {
                        collectPronounConfigFromModal();
                        const contextJSON = JSON.stringify(currentContextConfig);
                        const res = await window.go.main.App.TranslateSingleDialogue(key, d, contextJSON);
                        if (res && res.translated) {
                            d.translated = res.translated;
                            d.translationFailed = false;
                        }

                        if (currentVideoPath) {
                            const snapped = await window.go.main.App.SnapSingleDialogueBBox(currentVideoPath, d, 240, key);
                            if (snapped && snapped.bbox) {
                                d.bbox = snapped.bbox;
                            }
                        }

                        renderDialogueList();
                        drawOverlay();
                    } catch (e) {
                        alert('Lỗi dịch câu thoại này: ' + e);
                    } finally {
                        btnTransSingle.disabled = false;
                        btnTransSingle.innerText = '🌐 Dịch AI';
                    }
                });
            }

            const btnRefineFps = card.querySelector('.btn-refine-fps');
            btnRefineFps.addEventListener('click', async () => {
                if (isProcessing) return;
                btnRefineFps.disabled = true;
                btnRefineFps.innerText = '⌛...';
                try {
                    const selScanInterval = document.getElementById('sel-scan-interval');
                    const intervalVal = selScanInterval ? parseFloat(selScanInterval.value) : 0.30;
                    const refined = await window.go.main.App.RefineDialogueTimestamp(currentVideoPath, d, intervalVal);
                    d.timestamp = refined.timestamp;
                    d.endTimestamp = refined.endTimestamp;
                    d.duration = refined.duration;
                    renderDialogueList();
                    drawOverlay();
                } catch (e) {
                    alert('Lỗi tinh chỉnh 30 FPS: ' + e);
                } finally {
                    btnRefineFps.disabled = false;
                    btnRefineFps.innerText = '🎯 30 FPS';
                }
            });

            const btnSnap = card.querySelector('.btn-snap');
            btnSnap.addEventListener('click', async () => {
                if (isProcessing) return;
                btnSnap.disabled = true;
                btnSnap.innerText = '⌛...';
                try {
                    const snapped = await window.go.main.App.SnapSingleDialogueBBox(currentVideoPath, d, 240, geminiKeyInput.value.trim());
                    if (snapped && snapped.bbox) {
                        d.bbox = snapped.bbox;
                    }
                    renderDialogueList();
                    drawOverlay();
                } catch (e) {
                    console.error('AI Fit BBox error:', e);
                    alert('Lỗi căn AI thoại này: ' + e);
                } finally {
                    btnSnap.disabled = false;
                    btnSnap.innerText = '🤖 AI Fit';
                }
            });

            const btnTrash = card.querySelector('.btn-trash');
            btnTrash.addEventListener('click', () => {
                d.isTrash = !d.isTrash;
                renderDialogueList();
                drawOverlay();
            });

            dialogueList.appendChild(card);
        });
    }

    chkShowTrash.addEventListener('change', () => {
        renderDialogueList();
        drawOverlay();
    });

    function formatTime(s) {
        if (!s || isNaN(s) || !isFinite(s) || s < 0) return '00:00.0';
        const h = Math.floor(s / 3600);
        const m = Math.floor((s % 3600) / 60);
        const sec = Math.floor(s % 60);
        const ms = Math.floor((s % 1) * 10);
        if (h > 0) {
            return `${h.toString().padStart(2, '0')}:${m.toString().padStart(2, '0')}:${sec.toString().padStart(2, '0')}.${ms}`;
        }
        return `${m.toString().padStart(2, '0')}:${sec.toString().padStart(2, '0')}.${ms}`;
    }

    function escapeHtml(str) {
        if (!str) return '';
        return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    }

    function showModal(title) {
        modalTitle.textContent = title;
        modalProgressFill.style.width = '0%';
        modalStatusText.textContent = 'Đang khởi tạo...';

        modalStartTime = Date.now();
        if (modalTimer) {
            modalTimer.textContent = '⏱️ 00:00';
        }
        if (modalTimerInterval) clearInterval(modalTimerInterval);
        modalTimerInterval = setInterval(() => {
            if (modalTimer) {
                const elapsedSec = (Date.now() - modalStartTime) / 1000;
                modalTimer.textContent = '⏱️ ' + formatDuration(elapsedSec);
            }
        }, 500);

        progressModal.style.display = 'flex';
    }

    function hideModal() {
        if (modalTimerInterval) {
            clearInterval(modalTimerInterval);
            modalTimerInterval = null;
        }
        progressModal.style.display = 'none';
    }

    btnCancelModal.addEventListener('click', async () => {
        isProcessing = false;
        hideModal();
        if (window.go && window.go.main && window.go.main.App) {
            try {
                await window.go.main.App.CancelScan();
            } catch (e) {
                console.log('CancelScan error:', e);
            }
        }
        updateToolbarButtons();
    });
})();
