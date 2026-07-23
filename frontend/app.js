(function () {
    const video = document.getElementById('video-player');
    const canvas = document.getElementById('overlay-canvas');
    const ctx = canvas.getContext('2d');
    const videoContainer = document.getElementById('video-container');

    const btnSelectVideo = document.getElementById('btn-select-video');
    const btnStartScan = document.getElementById('btn-start-scan');
    const btnFilterTrash = document.getElementById('btn-filter-trash');
    const btnAnalyzeContext = document.getElementById('btn-analyze-context');
    const btnTranslateAll = document.getElementById('btn-translate-all');
    const btnExportVideo = document.getElementById('btn-export-video');

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
    const modalProgressFill = document.getElementById('modal-progress-fill');
    const modalStatusText = document.getElementById('modal-status-text');
    const btnCancelModal = document.getElementById('btn-cancel-modal');

    let currentVideoPath = '';
    let dialogues = [];
    let selectedDialogueId = null;
    let isProcessing = false;
    let activeGlossary = [];

    // Interactive Drag / Resize State
    let isDragging = false;
    let dragTarget = null;
    let startX = 0, startY = 0;
    let initialBox = null;

    const savedKey = localStorage.getItem('gemini_api_key') || '';
    if (savedKey) {
        geminiKeyInput.value = savedKey;
    }

    geminiKeyInput.addEventListener('input', () => {
        const val = geminiKeyInput.value.trim();
        localStorage.setItem('gemini_api_key', val);
        checkHealth();
    });

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

    videoSeekbar.addEventListener('input', () => {
        if (!video.duration) return;
        video.currentTime = (videoSeekbar.value / 100) * video.duration;
        updateTimeDisplay();
        drawOverlay();
    });

    btnToggleMute.addEventListener('click', () => {
        video.muted = !video.muted;
        btnToggleMute.textContent = video.muted ? '🔇' : '🔊';
    });

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
                video.src = `/wails-local-video?path=${encodeURIComponent(filePath)}`;
                video.load();
                video.play().catch(e => console.log('Autoplay prevented:', e));
                btnStartScan.disabled = false;
                dialogues = [];
                renderDialogueList();
                drawOverlay();
            }
        } catch (e) {
            alert('Lỗi chọn video: ' + e);
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

    function loopDraw() {
        drawOverlay();
        if (!video.paused && !video.ended) {
            requestAnimationFrame(loopDraw);
        }
    }

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

        dialogues.forEach(d => {
            if (!showTrash && d.isTrash) return;

            const end = d.endTimestamp && d.endTimestamp > d.timestamp ? d.endTimestamp : (d.timestamp + (d.duration || 1.2));
            const isCurrent = (currentTime >= d.timestamp && currentTime <= end);
            const isSelected = (d.id === selectedDialogueId);

            if (isCurrent || isSelected) {
                const bx = layout.left + d.bbox.x * layout.scale;
                const by = layout.top + d.bbox.y * layout.scale;
                const bw = d.bbox.width * layout.scale;
                const bh = d.bbox.height * layout.scale;

                ctx.save();

                ctx.fillStyle = '#ffffff';
                ctx.beginPath();
                ctx.roundRect(bx, by, bw, bh, 12);
                ctx.fill();

                const text = d.translated || d.original || '';
                ctx.fillStyle = '#000000';
                const fontSize = Math.max(12, 28 * (layout.height / 1080.0));
                ctx.font = `bold ${fontSize}px sans-serif`;
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

                    ctx.fillStyle = '#6366f1';
                    ctx.fillRect(bx - 4, by - 4, 8, 8);
                    ctx.fillRect(bx + bw - 4, by - 4, 8, 8);
                    ctx.fillRect(bx - 4, by + bh - 4, 8, 8);
                    ctx.fillRect(bx + bw - 4, by + bh - 4, 8, 8);
                }

                ctx.restore();
            }
        });
    }

    canvas.addEventListener('mousedown', (e) => {
        const layout = previewLayout();
        if (!layout) return;

        const rect = canvas.getBoundingClientRect();
        const mx = e.clientX - rect.left;
        const my = e.clientY - rect.top;

        const currentTime = video.currentTime || 0;
        let clickedDialogue = null;

        for (let i = dialogues.length - 1; i >= 0; i--) {
            const d = dialogues[i];
            if (!chkShowTrash.checked && d.isTrash) continue;

            const end = d.endTimestamp && d.endTimestamp > d.timestamp ? d.endTimestamp : (d.timestamp + (d.duration || 1.2));
            const isCurrent = (currentTime >= d.timestamp && currentTime <= end);
            const isSelected = (d.id === selectedDialogueId);

            if (isCurrent || isSelected) {
                const bx = layout.left + d.bbox.x * layout.scale;
                const by = layout.top + d.bbox.y * layout.scale;
                const bw = d.bbox.width * layout.scale;
                const bh = d.bbox.height * layout.scale;

                if (mx >= bx && mx <= bx + bw && my >= by && my <= by + bh) {
                    clickedDialogue = d;
                    break;
                }
            }
        }

        if (clickedDialogue) {
            selectedDialogueId = clickedDialogue.id;
            isDragging = true;
            dragTarget = clickedDialogue;
            startX = mx;
            startY = my;
            initialBox = { ...clickedDialogue.bbox };
            renderDialogueList();
            drawOverlay();
        }
    });

    canvas.addEventListener('mousemove', (e) => {
        if (!isDragging || !dragTarget || !initialBox) return;
        const layout = previewLayout();
        if (!layout) return;

        const rect = canvas.getBoundingClientRect();
        const mx = e.clientX - rect.left;
        const my = e.clientY - rect.top;

        const dx = (mx - startX) / layout.scale;
        const dy = (my - startY) / layout.scale;

        dragTarget.bbox.x = Math.max(0, initialBox.x + dx);
        dragTarget.bbox.y = Math.max(0, initialBox.y + dy);
        drawOverlay();
    });

    window.addEventListener('mouseup', () => {
        isDragging = false;
        dragTarget = null;
        initialBox = null;
    });

    btnStartScan.addEventListener('click', async () => {
        if (!currentVideoPath || isProcessing) return;
        isProcessing = true;
        showModal('⚡ Đang Quét Thoại (Full-Frame + Tinh Chỉnh 30 FPS)...');

        if (window.runtime) {
            window.runtime.EventsOn('scan-progress', (p) => {
                modalStatusText.textContent = p.status;
                if (p.totalFrames > 0) {
                    const pct = Math.min(100, Math.round((p.currentFrame / p.totalFrames) * 100));
                    modalProgressFill.style.width = pct + '%';
                }
            });
        }

        try {
            const selScanInterval = document.getElementById('sel-scan-interval');
            const intervalVal = selScanInterval ? parseFloat(selScanInterval.value) : 0.30;

            const config = {
                videoPath: currentVideoPath,
                interval: intervalVal,
                threshold: 240,
                sourceLang: 'chi_sim',
                geminiKey: geminiKeyInput.value.trim()
            };

            const results = await window.go.main.App.StartFullPipelineScan(config);
            dialogues = results || [];
            renderDialogueList();
            drawOverlay();

            if (dialogues.length > 0) {
                btnFilterTrash.disabled = false;
                btnAnalyzeContext.disabled = false;
                btnTranslateAll.disabled = false;
                btnExportVideo.disabled = false;
            }
        } catch (e) {
            alert('Lỗi quét thoại: ' + e);
        } finally {
            isProcessing = false;
            hideModal();
        }
    });

    btnFilterTrash.addEventListener('click', async () => {
        if (dialogues.length === 0 || isProcessing) return;
        const key = geminiKeyInput.value.trim();
        if (!key) {
            alert('Vui lòng nhập Gemini API Key ở thanh tiêu đề!');
            return;
        }

        isProcessing = true;
        showModal('🧹 AI Đang Lọc Ký Tự Rác & Watermark (Gemini 3.5 Flash Lite)...');

        try {
            const filtered = await window.go.main.App.FilterTrashWithAI(key, dialogues);
            dialogues = filtered || dialogues;
            renderDialogueList();
            drawOverlay();
        } catch (e) {
            alert('Lỗi lọc rác AI: ' + e);
        } finally {
            isProcessing = false;
            hideModal();
        }
    });

    btnAnalyzeContext.addEventListener('click', async () => {
        if (dialogues.length === 0 || isProcessing) return;
        const key = geminiKeyInput.value.trim();
        if (!key) {
            alert('Vui lòng nhập Gemini API Key ở thanh tiêu đề!');
            return;
        }

        isProcessing = true;
        showModal('🧠 AI Đang Đọc Ngữ Cảnh & Lập Bảng Xưng Hô (Gemini 3.5 Flash Lite)...');

        try {
            const entries = await window.go.main.App.AnalyzeContextAndGlossary(key, dialogues);
            activeGlossary = entries || [];
            let msg = '🎉 Phân tích ngữ cảnh thành công!\nBảng xưng hô đề xuất:\n';
            activeGlossary.forEach(e => {
                msg += `- ${e.source} => ${e.target}\n`;
            });
            alert(msg);
        } catch (e) {
            alert('Lỗi phân tích ngữ cảnh AI: ' + e);
        } finally {
            isProcessing = false;
            hideModal();
        }
    });

    btnTranslateAll.addEventListener('click', async () => {
        if (dialogues.length === 0 || isProcessing) return;
        const key = geminiKeyInput.value.trim();
        if (!key) {
            alert('Vui lòng nhập Gemini API Key ở thanh tiêu đề!');
            return;
        }

        isProcessing = true;
        showModal('🌐 Đang Dịch AI Bypass 18+ (Gemini 3.5 Flash Lite)...');

        try {
            const glossaryJSON = JSON.stringify(activeGlossary);
            const translated = await window.go.main.App.TranslateDialogues(key, dialogues, glossaryJSON);
            dialogues = translated || dialogues;
            renderDialogueList();
            drawOverlay();
        } catch (e) {
            alert('Lỗi dịch thoại: ' + e);
        } finally {
            isProcessing = false;
            hideModal();
        }
    });

    btnExportVideo.addEventListener('click', async () => {
        if (!currentVideoPath || dialogues.length === 0 || isProcessing) return;
        try {
            const savePath = await window.go.main.App.SelectSaveLocation('video_translated.mp4');
            if (!savePath) return;

            isProcessing = true;
            showModal('💾 Đang Xuất Video qua GPU FFmpeg...');

            const activeDialogues = dialogues.filter(d => !d.isTrash);
            const outPath = await window.go.main.App.ExportVideo(currentVideoPath, JSON.stringify(activeDialogues), savePath);
            alert('🎉 Xuất video thành công:\n' + outPath);
        } catch (e) {
            alert('Lỗi xuất video: ' + e);
        } finally {
            isProcessing = false;
            hideModal();
        }
    });

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
                    <span>Thoại #${d.id}</span>
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
                    <button class="btn btn-sm btn-snap">🎯 Snap</button>
                    <button class="btn btn-sm btn-trash" style="color: var(--accent-rose);">${d.isTrash ? '↺ Phục hồi' : '🗑 Xóa'}</button>
                </div>
            `;

            card.addEventListener('click', (e) => {
                if (e.target.tagName === 'INPUT' || e.target.tagName === 'BUTTON') return;
                selectedDialogueId = d.id;
                video.currentTime = d.timestamp;
                renderDialogueList();
                drawOverlay();
            });

            const inputTrans = card.querySelector('.translated-input');
            inputTrans.addEventListener('input', () => {
                d.translated = inputTrans.value;
                drawOverlay();
            });

            const btnSnap = card.querySelector('.btn-snap');
            btnSnap.addEventListener('click', async () => {
                try {
                    const snapped = await window.go.main.App.SnapSingleDialogueBBox(currentVideoPath, d, 240, geminiKeyInput.value.trim());
                    d.bbox = snapped.bbox;
                    drawOverlay();
                } catch (e) {
                    console.error('Snap error:', e);
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
        if (!s) return '00:00.0';
        const m = Math.floor(s / 60);
        const sec = Math.floor(s % 60);
        const ms = Math.floor((s % 1) * 10);
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
        progressModal.style.display = 'flex';
    }

    function hideModal() {
        progressModal.style.display = 'none';
    }

    btnCancelModal.addEventListener('click', () => {
        if (window.go && window.go.main && window.go.main.App) {
            window.go.main.App.CancelScan();
        }
        hideModal();
    });
})();
