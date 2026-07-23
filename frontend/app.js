// TranslateVideo Streamlined Frontend Controller
(function () {
    const video = document.getElementById('video-player');
    const canvas = document.getElementById('overlay-canvas');
    const ctx = canvas.getContext('2d');
    const videoContainer = document.getElementById('video-container');

    const btnSelectVideo = document.getElementById('btn-select-video');
    const btnStartScan = document.getElementById('btn-start-scan');
    const btnTranslateAll = document.getElementById('btn-translate-all');
    const btnExportVideo = document.getElementById('btn-export-video');

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
    let dragState = null;

    // Load saved Gemini API Key
    const savedKey = localStorage.getItem('gemini_api_key') || '';
    if (savedKey) {
        geminiKeyInput.value = savedKey;
    }

    geminiKeyInput.addEventListener('input', () => {
        const val = geminiKeyInput.value.trim();
        localStorage.setItem('gemini_api_key', val);
        checkHealth();
    });

    // Check backend health
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

    // Health check polling
    setInterval(checkHealth, 5000);
    setTimeout(checkHealth, 500);

    // File Selection
    btnSelectVideo.addEventListener('click', async () => {
        if (!window.go || !window.go.main || !window.go.main.App) return;
        try {
            const filePath = await window.go.main.App.SelectVideoFile();
            if (filePath) {
                currentVideoPath = filePath;
                video.src = `/wails-local-video?path=${encodeURIComponent(filePath)}`;
                btnStartScan.disabled = false;
                dialogues = [];
                renderDialogueList();
                drawOverlay();
            }
        } catch (e) {
            alert('Lỗi chọn video: ' + e);
        }
    });

    // ResizeObserver for Canvas
    const resizeObserver = new ResizeObserver(() => {
        if (videoContainer) {
            canvas.width = videoContainer.clientWidth;
            canvas.height = videoContainer.clientHeight;
            drawOverlay();
        }
    });
    if (videoContainer) resizeObserver.observe(videoContainer);

    video.addEventListener('timeupdate', drawOverlay);
    video.addEventListener('play', () => requestAnimationFrame(loopDraw));

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
                if (isSelected) {
                    ctx.strokeStyle = '#6366f1';
                    ctx.lineWidth = 2.5;
                    ctx.fillStyle = 'rgba(99, 102, 241, 0.15)';
                } else {
                    ctx.strokeStyle = '#10b981';
                    ctx.lineWidth = 1.5;
                    ctx.fillStyle = 'rgba(16, 185, 129, 0.10)';
                }

                ctx.fillRect(bx, by, bw, bh);
                ctx.strokeRect(bx, by, bw, bh);

                // Draw label badge
                ctx.fillStyle = isSelected ? '#6366f1' : '#10b981';
                ctx.fillRect(bx, by - 20, 40, 20);
                ctx.fillStyle = '#ffffff';
                ctx.font = 'bold 11px sans-serif';
                ctx.fillText(`#${d.id}`, bx + 6, by - 6);

                ctx.restore();
            }
        });
    }

    // Start Full Pipeline Scan
    btnStartScan.addEventListener('click', async () => {
        if (!currentVideoPath || isProcessing) return;
        isProcessing = true;
        showModal('⚡ Đang Quét Thoại (Full-Frame)...');

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
            const config = {
                videoPath: currentVideoPath,
                interval: 0.30,
                threshold: 240,
                sourceLang: 'chi_sim',
                geminiKey: geminiKeyInput.value.trim(),
                scanMode: 'full_frame'
            };

            const results = await window.go.main.App.StartFullPipelineScan(config);
            dialogues = results || [];
            renderDialogueList();
            drawOverlay();

            if (dialogues.length > 0) {
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

    // Translate All Dialogues with Gemini
    btnTranslateAll.addEventListener('click', async () => {
        if (dialogues.length === 0 || isProcessing) return;
        const key = geminiKeyInput.value.trim();
        if (!key) {
            alert('Vui lòng nhập Gemini API Key ở thanh tiêu đề!');
            return;
        }

        isProcessing = true;
        showModal('🌐 Đang Dịch Tự Động Qua Gemini 2.5 Flash...');

        try {
            const translated = await window.go.main.App.TranslateDialogues(key, dialogues, '');
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

    // Export Video
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

    // Render Streamlined Dialogue Cards
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

            // Card click & select
            card.addEventListener('click', (e) => {
                if (e.target.tagName === 'INPUT' || e.target.tagName === 'BUTTON') return;
                selectedDialogueId = d.id;
                video.currentTime = d.timestamp;
                renderDialogueList();
                drawOverlay();
            });

            // Input handlers
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
