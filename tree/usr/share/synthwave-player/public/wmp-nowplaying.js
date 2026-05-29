/*
 * WMP10-style "Now Playing" visualizer + graphic equalizer.
 *
 * Standalone: does not touch the Alpine app.
 *
 * Visualizer: by default it taps each <audio> element via
 * HTMLMediaElement.captureStream() into an AnalyserNode (a COPY of the stream),
 * so normal playback - including cross-origin radio - is never muted. Cross-origin
 * radio yields no analysable data, so we fall back to an ambient animation.
 *
 * Equalizer: a 10-band BiquadFilter chain. It is OFF by default and only engages
 * when the user enables it on the "Equalizer" screen. Enabling it routes the
 * players through createMediaElementSource(...) -> filters -> destination, which
 * shapes the sound for local library tracks. Cross-origin radio streams are
 * silenced while the equalizer is on (the source becomes CORS-tainted) - this is
 * an accepted trade-off; turn the EQ off and reload to restore radio.
 */
(function () {
    'use strict';

    var canvas = null;
    var ctx = null;
    var audioCtx = null;
    var analyser = null;
    var freqData = null;
    var timeData = null;
    var streamSource = null;
    var capturedEl = null;
    var peaks = null;
    var rafId = null;

    var MODES = ['bars', 'mirror', 'radial', 'scope', 'eq', 'off'];
    var LABELS = {
        bars: 'Bars',
        mirror: 'Mirror Bars',
        radial: 'Spectrum',
        scope: 'Oscilloscope',
        eq: 'Equalizer',
        off: 'Visualizer Off'
    };
    var mode = localStorage.getItem('wmpVisualizerMode') || 'bars';
    if (MODES.indexOf(mode) === -1) mode = 'bars';
    var labelUntil = 0;
    var radialPhase = 0;

    // ---- Equalizer state ----
    var EQ_BANDS = [31, 62, 125, 250, 500, 1000, 2000, 4000, 8000, 16000];
    var EQ_LABELS = ['31', '62', '125', '250', '500', '1K', '2K', '4K', '8K', '16K'];
    var EQ_PRESETS = {
        Flat:        [0, 0, 0, 0, 0, 0, 0, 0, 0, 0],
        Rock:        [5, 4, 3, 1, -1, -1, 1, 3, 4, 5],
        Pop:         [-1, 1, 3, 4, 4, 3, 1, 0, -1, -1],
        Jazz:        [4, 3, 1, 2, -1, -1, 0, 1, 3, 4],
        Classical:   [4, 3, 2, 1, -1, -1, 0, 2, 3, 4],
        'Bass Boost':[7, 6, 5, 3, 1, 0, 0, 0, 0, 0],
        'Treble Boost':[0, 0, 0, 0, 0, 1, 3, 5, 6, 7],
        Vocal:       [-2, -1, 0, 2, 4, 4, 3, 1, 0, -1]
    };
    var eqGains = loadEqGains();
    var eqEnabled = localStorage.getItem('wmpEqEnabled') === '1';
    var eqActive = false;            // true once the graph is engaged
    var eqFilters = null;            // array of BiquadFilterNode
    var eqMaster = null;             // GainNode feeding destination + analyser
    var eqSources = {};              // element -> MediaElementAudioSourceNode
    var eqPanel = null;              // DOM control panel

    function loadEqGains() {
        try {
            var raw = JSON.parse(localStorage.getItem('wmpEqGains'));
            if (Array.isArray(raw) && raw.length === 10) {
                return raw.map(function (n) { return clampGain(+n || 0); });
            }
        } catch (e) {}
        return [0, 0, 0, 0, 0, 0, 0, 0, 0, 0];
    }
    function clampGain(v) { return Math.max(-12, Math.min(12, v)); }
    function saveEqGains() {
        try { localStorage.setItem('wmpEqGains', JSON.stringify(eqGains)); } catch (e) {}
    }

    function getCanvas() {
        if (!canvas || !canvas.isConnected) {
            canvas = document.getElementById('wmp-visualizer');
            ctx = canvas ? canvas.getContext('2d') : null;
        }
        return canvas;
    }

    function ensureContext() {
        if (audioCtx) return audioCtx;
        var AC = window.AudioContext || window.webkitAudioContext;
        if (!AC) return null;
        try {
            audioCtx = new AC();
        } catch (e) {
            return null;
        }
        analyser = audioCtx.createAnalyser();
        analyser.fftSize = 2048;
        analyser.smoothingTimeConstant = 0.82;
        analyser.minDecibels = -85;
        analyser.maxDecibels = -10;
        freqData = new Uint8Array(analyser.frequencyBinCount);
        timeData = new Uint8Array(analyser.fftSize);
        return audioCtx;
    }

    function captureStreamOf(el) {
        if (el.captureStream) {
            try { return el.captureStream(); } catch (e) {}
        }
        if (el.mozCaptureStream) {
            try { return el.mozCaptureStream(); } catch (e) {}
        }
        return null;
    }

    // Tap the given audio element's output into the analyser (copy only).
    function tap(el) {
        if (!el || el === capturedEl) return;
        if (!ensureContext()) return;
        var stream = captureStreamOf(el);
        if (!stream || !stream.getAudioTracks || stream.getAudioTracks().length === 0) return;
        try {
            var src = audioCtx.createMediaStreamSource(stream);
            if (streamSource) { try { streamSource.disconnect(); } catch (e) {} }
            streamSource = src;
            streamSource.connect(analyser); // analyser is a sink; not routed to destination
            capturedEl = el;
        } catch (e) {
            // Tainted/cross-origin stream or unsupported: leave analyser idle.
        }
    }

    // ---- Equalizer audio graph ----

    // Build the shared filter chain + master gain once.
    function buildEqGraph() {
        if (eqFilters || !ensureContext()) return;
        eqFilters = [];
        for (var i = 0; i < EQ_BANDS.length; i++) {
            var f = audioCtx.createBiquadFilter();
            if (i === 0) {
                f.type = 'lowshelf';
            } else if (i === EQ_BANDS.length - 1) {
                f.type = 'highshelf';
            } else {
                f.type = 'peaking';
                f.Q.value = 1.1;
            }
            f.frequency.value = EQ_BANDS[i];
            f.gain.value = eqGains[i];
            if (eqFilters.length) eqFilters[eqFilters.length - 1].connect(f);
            eqFilters.push(f);
        }
        eqMaster = audioCtx.createGain();
        eqMaster.gain.value = 1;
        eqFilters[eqFilters.length - 1].connect(eqMaster);
        eqMaster.connect(audioCtx.destination);
        if (analyser) eqMaster.connect(analyser);
    }

    function sourceFor(el) {
        if (eqSources[el && el.id]) return eqSources[el.id];
        if (!ensureContext()) return null;
        try {
            var src = audioCtx.createMediaElementSource(el);
            eqSources[el.id] = src;
            return src;
        } catch (e) {
            // Already connected elsewhere or unsupported.
            return null;
        }
    }

    // Engage the EQ: route both players through the filter chain.
    function engageEq() {
        if (!ensureContext()) return;
        if (audioCtx.state === 'suspended') audioCtx.resume();
        buildEqGraph();
        if (!eqFilters) return;
        var els = document.querySelectorAll('audio');
        els.forEach(function (el) {
            if (!el.id) return;
            var src = sourceFor(el);
            if (!src) return;
            try { src.disconnect(); } catch (e) {}
            src.connect(eqFilters[0]);
        });
        eqActive = true;
        // Stop the captureStream tap; the analyser now reads the EQ master.
        if (streamSource) { try { streamSource.disconnect(); } catch (e) {} streamSource = null; }
        capturedEl = null;
    }

    // Bypass the EQ: route sources straight to the destination (flat). Local
    // audio plays normally; already-tainted radio stays silent until reload.
    function bypassEq() {
        if (!eqActive) return;
        var els = document.querySelectorAll('audio');
        els.forEach(function (el) {
            var src = el.id && eqSources[el.id];
            if (!src) return;
            try { src.disconnect(); } catch (e) {}
            try { src.connect(audioCtx.destination); } catch (e) {}
        });
        eqActive = false;
    }

    function applyEqGains() {
        if (!eqFilters) return;
        for (var i = 0; i < eqFilters.length; i++) {
            eqFilters[i].gain.value = eqGains[i];
        }
    }

    function setEqEnabled(on) {
        eqEnabled = !!on;
        try { localStorage.setItem('wmpEqEnabled', eqEnabled ? '1' : '0'); } catch (e) {}
        if (eqEnabled) engageEq(); else bypassEq();
    }

    function activeAudio() {
        var els = document.querySelectorAll('audio');
        for (var i = 0; i < els.length; i++) {
            if (!els[i].paused && !els[i].ended && els[i].currentTime > 0) return els[i];
        }
        for (var j = 0; j < els.length; j++) {
            if (!els[j].paused && !els[j].ended) return els[j];
        }
        return null;
    }

    function resizeCanvas() {
        if (!getCanvas()) return;
        var rect = canvas.getBoundingClientRect();
        var dpr = window.devicePixelRatio || 1;
        var w = Math.max(1, Math.floor(rect.width * dpr));
        var h = Math.max(1, Math.floor(rect.height * dpr));
        if (canvas.width !== w || canvas.height !== h) {
            canvas.width = w;
            canvas.height = h;
        }
    }

    function draw() {
        rafId = requestAnimationFrame(draw);
        if (!getCanvas() || canvas.offsetParent === null) return; // not visible
        resizeCanvas();
        var w = canvas.width, h = canvas.height;
        ctx.clearRect(0, 0, w, h);

        if (mode === 'off') {
            drawModeLabel(w, h);
            return;
        }

        var playing = activeAudio();
        if (playing && audioCtx && audioCtx.state === 'suspended') {
            audioCtx.resume();
        }
        // When the EQ graph is engaged the analyser reads the EQ master; otherwise
        // tap a copy of the active element's stream.
        if (!eqActive && playing) tap(playing);

        var haveData = false;
        if (analyser && playing && (eqActive || (capturedEl && !capturedEl.paused))) {
            analyser.getByteFrequencyData(freqData);
            for (var k = 0; k < 32; k++) { if (freqData[k] > 0) { haveData = true; break; } }
        }

        if (mode === 'eq') {
            // Faint backdrop behind the control panel.
            if (haveData) { drawBars(w, h); } else { drawAmbient(w, h, !!playing); }
            drawModeLabel(w, h);
            return;
        }

        if (haveData) {
            switch (mode) {
                case 'mirror': drawMirrorBars(w, h); break;
                case 'radial': drawRadial(w, h); break;
                case 'scope': drawScopeFull(w, h); break;
                case 'bars':
                default:
                    drawBars(w, h);
                    drawScope(w, h);
                    break;
            }
        } else {
            drawAmbient(w, h, !!playing);
        }

        drawModeLabel(w, h);
    }

    function drawBars(w, h) {
        var bars = 48;
        var gap = Math.max(1, Math.floor(w / bars * 0.18));
        var bw = (w - gap * (bars - 1)) / bars;
        if (!peaks || peaks.length !== bars) peaks = new Float32Array(bars);
        // Use a log-ish slice of the spectrum (skip the very top bins).
        var usable = Math.floor(analyser.frequencyBinCount * 0.7);
        for (var i = 0; i < bars; i++) {
            var lo = Math.floor(Math.pow(i / bars, 1.6) * usable);
            var hi = Math.floor(Math.pow((i + 1) / bars, 1.6) * usable);
            if (hi <= lo) hi = lo + 1;
            var sum = 0;
            for (var j = lo; j < hi; j++) sum += freqData[j];
            var v = (sum / (hi - lo)) / 255; // 0..1
            v = Math.pow(v, 0.9);
            var bh = v * h * 0.92;
            var x = i * (bw + gap);
            var grad = ctx.createLinearGradient(0, h, 0, h - bh);
            grad.addColorStop(0, '#0a47a0');
            grad.addColorStop(0.5, '#2f8fe0');
            grad.addColorStop(1, '#bfe6ff');
            ctx.fillStyle = grad;
            ctx.fillRect(x, h - bh, bw, bh);
            // Peak cap
            var peakY = h - (peaks[i] * h * 0.92);
            if (bh > peaks[i] * h * 0.92) peaks[i] = v; else peaks[i] = Math.max(0, peaks[i] - 0.012);
            ctx.fillStyle = 'rgba(220,240,255,0.9)';
            ctx.fillRect(x, h - (peaks[i] * h * 0.92) - 2, bw, 2);
        }
    }

    function drawScope(w, h) {
        analyser.getByteTimeDomainData(timeData);
        ctx.lineWidth = Math.max(1, (window.devicePixelRatio || 1));
        ctx.strokeStyle = 'rgba(180,225,255,0.55)';
        ctx.beginPath();
        var step = w / timeData.length;
        for (var i = 0; i < timeData.length; i++) {
            var y = (timeData[i] / 255) * h;
            var x = i * step;
            if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
        }
        ctx.stroke();
    }

    var ambientPhase = 0;
    function drawAmbient(w, h, playing) {
        ambientPhase += playing ? 0.05 : 0.018;
        var dpr = window.devicePixelRatio || 1;
        ctx.lineWidth = 2 * dpr;
        for (var line = 0; line < 3; line++) {
            ctx.beginPath();
            var amp = h * (0.06 + line * 0.04) * (playing ? 1 : 0.5);
            var yc = h * 0.5;
            for (var x = 0; x <= w; x += 6 * dpr) {
                var y = yc + Math.sin((x / w) * Math.PI * (3 + line) + ambientPhase + line) * amp;
                if (x === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
            }
            ctx.strokeStyle = 'rgba(120,180,240,' + (0.18 + line * 0.08) + ')';
            ctx.stroke();
        }
    }

    // Symmetric bars mirrored around the vertical center.
    function drawMirrorBars(w, h) {
        var bars = 48;
        var gap = Math.max(1, Math.floor(w / bars * 0.18));
        var bw = (w - gap * (bars - 1)) / bars;
        var mid = h / 2;
        var usable = Math.floor(analyser.frequencyBinCount * 0.7);
        for (var i = 0; i < bars; i++) {
            var lo = Math.floor(Math.pow(i / bars, 1.6) * usable);
            var hi = Math.floor(Math.pow((i + 1) / bars, 1.6) * usable);
            if (hi <= lo) hi = lo + 1;
            var sum = 0;
            for (var j = lo; j < hi; j++) sum += freqData[j];
            var v = Math.pow((sum / (hi - lo)) / 255, 0.9);
            var half = v * (h * 0.46);
            var x = i * (bw + gap);
            var grad = ctx.createLinearGradient(0, mid - half, 0, mid + half);
            grad.addColorStop(0, '#bfe6ff');
            grad.addColorStop(0.5, '#2f8fe0');
            grad.addColorStop(1, '#bfe6ff');
            ctx.fillStyle = grad;
            ctx.fillRect(x, mid - half, bw, half * 2);
        }
        ctx.strokeStyle = 'rgba(180,225,255,0.25)';
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(0, mid);
        ctx.lineTo(w, mid);
        ctx.stroke();
    }

    // Circular spectrum: bars radiating from the center, slowly rotating.
    function drawRadial(w, h) {
        var bars = 64;
        var cx = w / 2, cy = h / 2;
        var inner = Math.min(w, h) * 0.16;
        var maxLen = Math.min(w, h) * 0.32;
        radialPhase += 0.004;
        var usable = Math.floor(analyser.frequencyBinCount * 0.6);
        ctx.lineWidth = Math.max(2, (window.devicePixelRatio || 1) * 1.5);
        for (var i = 0; i < bars; i++) {
            var lo = Math.floor(Math.pow(i / bars, 1.4) * usable);
            var hi = Math.floor(Math.pow((i + 1) / bars, 1.4) * usable);
            if (hi <= lo) hi = lo + 1;
            var sum = 0;
            for (var j = lo; j < hi; j++) sum += freqData[j];
            var v = Math.pow((sum / (hi - lo)) / 255, 0.9);
            var len = inner + v * maxLen;
            var ang = (i / bars) * Math.PI * 2 + radialPhase;
            var cos = Math.cos(ang), sin = Math.sin(ang);
            var grad = ctx.createLinearGradient(cx + cos * inner, cy + sin * inner, cx + cos * len, cy + sin * len);
            grad.addColorStop(0, '#0a47a0');
            grad.addColorStop(1, '#bfe6ff');
            ctx.strokeStyle = grad;
            ctx.beginPath();
            ctx.moveTo(cx + cos * inner, cy + sin * inner);
            ctx.lineTo(cx + cos * len, cy + sin * len);
            ctx.stroke();
        }
    }

    // Full-height glowing oscilloscope.
    function drawScopeFull(w, h) {
        analyser.getByteTimeDomainData(timeData);
        var mid = h / 2;
        ctx.save();
        ctx.lineWidth = Math.max(2, (window.devicePixelRatio || 1) * 1.5);
        ctx.strokeStyle = '#bfe6ff';
        ctx.shadowColor = 'rgba(120,190,255,0.9)';
        ctx.shadowBlur = 12 * (window.devicePixelRatio || 1);
        ctx.beginPath();
        var step = w / timeData.length;
        for (var i = 0; i < timeData.length; i++) {
            var amp = (timeData[i] / 128) - 1; // -1..1
            var y = mid + amp * (h * 0.45);
            var x = i * step;
            if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
        }
        ctx.stroke();
        ctx.restore();
    }

    // Brief caption shown after a mode switch (and once on load).
    function drawModeLabel(w, h) {
        if (Date.now() >= labelUntil) return;
        var dpr = window.devicePixelRatio || 1;
        ctx.save();
        ctx.font = 'bold ' + Math.round(13 * dpr) + 'px "Tahoma", "Segoe UI", sans-serif';
        ctx.textBaseline = 'top';
        var text = LABELS[mode] || mode;
        var padX = 10 * dpr, padY = 6 * dpr;
        var tw = ctx.measureText(text).width;
        ctx.fillStyle = 'rgba(4,12,24,0.6)';
        ctx.fillRect(8 * dpr, 8 * dpr, tw + padX * 2, 20 * dpr + padY);
        ctx.fillStyle = '#eaf3ff';
        ctx.fillText(text, 8 * dpr + padX, 8 * dpr + padY);
        ctx.restore();
    }

    function bindAudio() {
        var els = document.querySelectorAll('audio');
        els.forEach(function (el) {
            el.addEventListener('play', function () {
                ensureContext();
                if (audioCtx && audioCtx.state === 'suspended') audioCtx.resume();
            });
            el.addEventListener('playing', function () {
                if (eqActive) return; // analyser already reads the EQ master
                // Re-tap on (re)start so a new src is captured.
                capturedEl = null;
                tap(el);
            });
        });
    }

    // ---- Equalizer control panel (DOM) ----

    function buildEqPanel() {
        var host = document.querySelector('.wmp-np-visual');
        if (!host || eqPanel) return;
        var panel = document.createElement('div');
        panel.className = 'wmp-eq-panel';
        panel.style.display = 'none';

        var head = document.createElement('div');
        head.className = 'wmp-eq-head';

        var enableLabel = document.createElement('label');
        enableLabel.className = 'wmp-eq-enable';
        var enableBox = document.createElement('input');
        enableBox.type = 'checkbox';
        enableBox.checked = eqEnabled;
        enableBox.addEventListener('change', function () { setEqEnabled(enableBox.checked); });
        enableLabel.appendChild(enableBox);
        enableLabel.appendChild(document.createTextNode(' Equalizer'));
        head.appendChild(enableLabel);

        var preset = document.createElement('select');
        preset.className = 'wmp-eq-preset';
        var ph = document.createElement('option');
        ph.value = ''; ph.textContent = 'Presets...';
        preset.appendChild(ph);
        Object.keys(EQ_PRESETS).forEach(function (name) {
            var opt = document.createElement('option');
            opt.value = name; opt.textContent = name;
            preset.appendChild(opt);
        });
        preset.addEventListener('change', function () {
            if (EQ_PRESETS[preset.value]) {
                applyPreset(EQ_PRESETS[preset.value]);
                preset.value = '';
            }
        });
        head.appendChild(preset);

        var resetBtn = document.createElement('button');
        resetBtn.type = 'button';
        resetBtn.className = 'wmp-eq-reset';
        resetBtn.textContent = 'Flat';
        resetBtn.addEventListener('click', function () { applyPreset(EQ_PRESETS.Flat); });
        head.appendChild(resetBtn);

        panel.appendChild(head);

        var bands = document.createElement('div');
        bands.className = 'wmp-eq-bands';
        var sliders = [];
        for (var i = 0; i < EQ_BANDS.length; i++) {
            (function (idx) {
                var band = document.createElement('div');
                band.className = 'wmp-eq-band';
                var range = document.createElement('input');
                range.type = 'range';
                range.min = '-12'; range.max = '12'; range.step = '1';
                range.value = String(eqGains[idx]);
                range.className = 'wmp-eq-slider';
                range.addEventListener('input', function () {
                    eqGains[idx] = clampGain(+range.value);
                    eqFilters && (eqFilters[idx].gain.value = eqGains[idx]);
                    saveEqGains();
                });
                var freq = document.createElement('span');
                freq.className = 'wmp-eq-freq';
                freq.textContent = EQ_LABELS[idx];
                band.appendChild(range);
                band.appendChild(freq);
                bands.appendChild(band);
                sliders.push(range);
            })(i);
        }
        panel.appendChild(bands);

        var note = document.createElement('div');
        note.className = 'wmp-eq-note';
        note.textContent = 'Internet radio is silenced while the equalizer is on.';
        panel.appendChild(note);

        // Keep clicks inside the panel from cycling the visualizer.
        panel.addEventListener('click', function (e) { e.stopPropagation(); });
        panel.addEventListener('mousedown', function (e) { e.stopPropagation(); });

        panel._sliders = sliders;
        panel._enableBox = enableBox;
        host.appendChild(panel);
        eqPanel = panel;
    }

    function applyPreset(gains) {
        for (var i = 0; i < eqGains.length; i++) {
            eqGains[i] = clampGain(gains[i] || 0);
            if (eqPanel && eqPanel._sliders[i]) eqPanel._sliders[i].value = String(eqGains[i]);
        }
        applyEqGains();
        saveEqGains();
    }

    function syncEqPanel() {
        if (!eqPanel) return;
        eqPanel.style.display = (mode === 'eq') ? 'flex' : 'none';
    }

    function cycleMode() {
        mode = MODES[(MODES.indexOf(mode) + 1) % MODES.length];
        try { localStorage.setItem('wmpVisualizerMode', mode); } catch (e) {}
        labelUntil = Date.now() + 1200;
        syncEqPanel();
    }

    function init() {
        getCanvas();
        bindAudio();
        buildEqPanel();
        syncEqPanel();
        if (eqEnabled) {
            // Engage lazily on first user gesture (autoplay policy); arm a one-shot.
            var arm = function () {
                engageEq();
                document.removeEventListener('click', arm, true);
            };
            document.addEventListener('click', arm, true);
        }
        if (getCanvas()) {
            canvas.title = 'Click to change visualizer';
            canvas.addEventListener('click', cycleMode);
        }
        if (window.ResizeObserver && getCanvas()) {
            new ResizeObserver(resizeCanvas).observe(canvas);
        }
        labelUntil = Date.now() + 1200;
        if (!rafId) draw();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
