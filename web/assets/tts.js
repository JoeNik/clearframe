/* 音色工坊 */
(function () {
  const { $, $$, icon, esc, api, renderHeader, renderFooter, renderAds, bytes, dur, timeAgo, toast } = window.UI;

  const state = {
    cfg: null,
    voices: [],
    renders: [],
    selVoices: new Set(),
    selRenders: new Set(),
    mode: 'clone',
    src: 'upload',
    pending: null,        // {blob, name, source, durationMs}
    pendingUrl: null,
    style: ''
  };

  // ================= 录音 =================
  let mediaStream = null, recorder = null, chunks = [], audioCtx = null, analyser = null, rafId = 0, recTimer = 0, recStart = 0;

  function fmtTime(msv) {
    const s = Math.floor(msv / 1000);
    return String(Math.floor(s / 60)).padStart(2, '0') + ':' + String(s % 60).padStart(2, '0');
  }

  function pickMime() {
    const cands = ['audio/webm;codecs=opus', 'audio/webm', 'audio/ogg;codecs=opus', 'audio/mp4'];
    for (const c of cands) {
      if (window.MediaRecorder && MediaRecorder.isTypeSupported && MediaRecorder.isTypeSupported(c)) return c;
    }
    return '';
  }

  async function startRec() {
    try {
      mediaStream = await navigator.mediaDevices.getUserMedia({
        audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true }
      });
    } catch (e) {
      toast('无法访问麦克风：' + (e.message || '请检查浏览器权限'), 'err');
      return;
    }
    const mime = pickMime();
    try {
      recorder = mime ? new MediaRecorder(mediaStream, { mimeType: mime }) : new MediaRecorder(mediaStream);
    } catch (e) {
      toast('当前浏览器不支持录音', 'err');
      stopTracks();
      return;
    }
    chunks = [];
    recorder.ondataavailable = function (e) { if (e.data && e.data.size) chunks.push(e.data); };
    recorder.onstop = onRecStop;
    recorder.start(200);

    $('#recBtn').classList.add('on');
    $('#recPreview').classList.add('hidden');
    recStart = Date.now();
    $('#recTime').textContent = '00:00';
    recTimer = setInterval(function () {
      $('#recTime').textContent = fmtTime(Date.now() - recStart);
    }, 120);

    // 电平表
    try {
      audioCtx = audioCtx || new (window.AudioContext || window.webkitAudioContext)();
      const src = audioCtx.createMediaStreamSource(mediaStream);
      analyser = audioCtx.createAnalyser();
      analyser.fftSize = 512;
      src.connect(analyser);
      const buf = new Uint8Array(analyser.frequencyBinCount);
      (function tick() {
        analyser.getByteTimeDomainData(buf);
        let peak = 0;
        for (let i = 0; i < buf.length; i++) peak = Math.max(peak, Math.abs(buf[i] - 128));
        $('#meterBar').style.width = Math.min(100, (peak / 128) * 170) + '%';
        rafId = requestAnimationFrame(tick);
      })();
    } catch (e) { /* 电平表失败不影响录音 */ }
  }

  function stopRec() {
    if (recorder && recorder.state === 'recording') recorder.stop();
    $('#recBtn').classList.remove('on');
    clearInterval(recTimer);
    cancelAnimationFrame(rafId);
    $('#meterBar').style.width = '0%';
  }

  function stopTracks() {
    if (mediaStream) {
      mediaStream.getTracks().forEach(function (t) { t.stop(); });
      mediaStream = null;
    }
  }

  async function onRecStop() {
    stopTracks();
    const elapsed = Date.now() - recStart;
    const blob = new Blob(chunks, { type: recorder.mimeType || 'audio/webm' });
    if (!blob.size) { toast('没有录到声音', 'err'); return; }
    try {
      const ab = await blob.arrayBuffer();
      audioCtx = audioCtx || new (window.AudioContext || window.webkitAudioContext)();
      const audioBuf = await audioCtx.decodeAudioData(ab);
      const wav = encodeWav(toMono(audioBuf), audioBuf.sampleRate);
      setPending(wav, 'record', Math.round(audioBuf.duration * 1000));
      const url = URL.createObjectURL(wav);
      const p = $('#recPlayer');
      p.src = url;
      $('#recPreview').classList.remove('hidden');
      $('#recTime').textContent = fmtTime(Math.round(audioBuf.duration * 1000));
      if (audioBuf.duration < 8) {
        $('#saveStatus').innerHTML = '<span style="color:var(--amber)">录音仅 ' +
          audioBuf.duration.toFixed(1) + ' 秒，建议 10 秒以上以获得更好的复刻效果</span>';
      } else {
        $('#saveStatus').textContent = '录音完成，' + audioBuf.duration.toFixed(1) + ' 秒';
      }
    } catch (e) {
      toast('音频解码失败：' + e.message, 'err');
    }
  }

  function toMono(buf) {
    if (buf.numberOfChannels === 1) return buf.getChannelData(0);
    const n = buf.length, out = new Float32Array(n);
    for (let c = 0; c < buf.numberOfChannels; c++) {
      const d = buf.getChannelData(c);
      for (let i = 0; i < n; i++) out[i] += d[i] / buf.numberOfChannels;
    }
    return out;
  }

  function encodeWav(samples, sr) {
    const TARGET = 44100;
    let s = samples, r = sr;
    if (sr !== TARGET) {
      r = TARGET;
      const L = Math.floor(samples.length * TARGET / sr);
      const out = new Float32Array(L);
      for (let i = 0; i < L; i++) {
        const t = i * sr / TARGET, j = Math.floor(t), f = t - j;
        out[i] = (samples[j] || 0) * (1 - f) + (samples[j + 1] || 0) * f;
      }
      s = out;
    }
    const b = new ArrayBuffer(44 + s.length * 2), v = new DataView(b);
    const ws = function (o, t) { for (let i = 0; i < t.length; i++) v.setUint8(o + i, t.charCodeAt(i)); };
    ws(0, 'RIFF'); v.setUint32(4, 36 + s.length * 2, true); ws(8, 'WAVE'); ws(12, 'fmt ');
    v.setUint32(16, 16, true); v.setUint16(20, 1, true); v.setUint16(22, 1, true);
    v.setUint32(24, r, true); v.setUint32(28, r * 2, true); v.setUint16(32, 2, true); v.setUint16(34, 16, true);
    ws(36, 'data'); v.setUint32(40, s.length * 2, true);
    for (let i = 0; i < s.length; i++) {
      const x = Math.max(-1, Math.min(1, s[i]));
      v.setInt16(44 + i * 2, x < 0 ? x * 0x8000 : x * 0x7fff, true);
    }
    return new Blob([b], { type: 'audio/wav' });
  }

  // ================= 上传 =================
  function setPending(blob, source, durationMs) {
    state.pending = { blob: blob, source: source, durationMs: durationMs || 0 };
    $('#btnSaveVoice').disabled = false;
    if (!source || source === 'upload') {
      $('#fileName').textContent = blob.name || '音频片段';
      $('#fileSize').textContent = bytes(blob.size);
      const url = URL.createObjectURL(blob);
      $('#filePlayer').src = url;
      $('#fileInfo').classList.remove('hidden');
    }
  }

  async function handleFile(file) {
    if (!file) return;
    const max = (state.cfg.maxUploadMb || 2) * 1024 * 1024;
    if (file.size > max) {
      toast('文件超过 ' + state.cfg.maxUploadMb + ' MB 限制', 'err');
      return;
    }
    const okType = /audio\/(wav|mpeg|mp3|x-wav)/.test(file.type) || /\.(wav|mp3)$/i.test(file.name);
    if (!okType) {
      toast('仅支持 WAV / MP3 格式', 'err');
      return;
    }
    // 本地解析时长
    let durationMs = 0;
    try {
      const ab = await file.arrayBuffer();
      audioCtx = audioCtx || new (window.AudioContext || window.webkitAudioContext)();
      const buf = await audioCtx.decodeAudioData(ab.slice(0));
      durationMs = Math.round(buf.duration * 1000);
      if (buf.duration < 8) {
        $('#saveStatus').innerHTML = '<span style="color:var(--amber)">样本仅 ' + buf.duration.toFixed(1) +
          ' 秒，建议 10 秒以上</span>';
      } else {
        $('#saveStatus').textContent = '样本时长 ' + buf.duration.toFixed(1) + ' 秒';
      }
    } catch (e) {
      $('#saveStatus').textContent = '';
    }
    setPending(file, 'upload', durationMs);
    if (!$('#voiceName').value.trim()) {
      $('#voiceName').value = file.name.replace(/\.[^.]+$/, '').slice(0, 40);
    }
  }

  async function saveVoice() {
    if (!state.pending) return;
    const btn = $('#btnSaveVoice');
    btn.disabled = true;
    const old = btn.textContent;
    btn.innerHTML = '<span class="spinner"></span> 保存中';
    const fd = new FormData();
    const name = $('#voiceName').value.trim() || ('音色 ' + new Date().toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }));
    fd.append('audio', state.pending.blob, state.pending.blob.name || 'sample.wav');
    fd.append('name', name);
    fd.append('source', state.pending.source);
    fd.append('durationMs', String(state.pending.durationMs || 0));

    try {
      const res = await fetch('/api/tts/voices', { method: 'POST', body: fd, credentials: 'same-origin' });
      const data = await res.json().catch(function () { return null; });
      if (!res.ok) throw new Error((data && data.message) || '保存失败');
      toast('音色已保存', 'ok');
      resetPending();
      await loadVoices();
      $('#saveStatus').textContent = '';
    } catch (e) {
      toast(e.message, 'err');
      btn.disabled = false;
    }
    btn.textContent = old;
  }

  function resetPending() {
    state.pending = null;
    $('#btnSaveVoice').disabled = true;
    $('#voiceName').value = '';
    $('#fileInfo').classList.add('hidden');
    $('#fileInput').value = '';
    $('#recPreview').classList.add('hidden');
    $('#recTime').textContent = '00:00';
  }

  // ================= 音色库 =================
  async function loadVoices() {
    try {
      const res = await api.get('/api/tts/voices');
      state.voices = res.items || [];
    } catch (e) {
      state.voices = [];
      toast(e.message, 'err');
    }
    // 清理已失效的选择
    const ids = new Set(state.voices.map(function (v) { return v.id; }));
    state.selVoices.forEach(function (id) { if (!ids.has(id)) state.selVoices.delete(id); });
    renderVoices();
    fillVoiceSelect();
  }

  function renderVoices() {
    const grid = $('#voiceGrid');
    $('#voiceCount').textContent = state.voices.length;
    if (!state.voices.length) {
      grid.innerHTML = '<div class="empty" style="grid-column:1/-1;padding:40px 20px">' +
        '还没有音色，先在上方上传或录制一段人声</div>';
    } else {
      grid.innerHTML = state.voices.map(function (v) {
        const sel = state.selVoices.has(v.id) ? ' sel' : '';
        return '<div class="voice-item' + sel + '" data-id="' + v.id + '">' +
          '<div class="row1">' +
          '<input type="checkbox" class="vsel" data-id="' + v.id + '"' + (sel ? ' checked' : '') +
          ' style="accent-color:var(--accent)">' +
          '<span class="nm" title="' + esc(v.name) + '">' + esc(v.name) + '</span>' +
          '<span class="ops">' +
          '<button class="iconbtn" data-act="rename" data-id="' + v.id + '" title="重命名">' + icon('edit', 12) + '</button>' +
          '<button class="iconbtn danger" data-act="del" data-id="' + v.id + '" title="删除">' + icon('trash', 12) + '</button>' +
          '</span></div>' +
          '<div class="meta"><span>' + (v.durationMs ? dur(v.durationMs) : '—') + '</span>' +
          '<span>' + bytes(v.size) + '</span>' +
          '<span>' + (v.source === 'record' ? '录音' : '上传') + '</span></div>' +
          '<audio preload="none" controls src="/api/tts/voices/' + v.id + '/audio"></audio>' +
          '</div>';
      }).join('');
    }
    updateVoiceBar();
  }

  function updateVoiceBar() {
    const n = state.selVoices.size;
    $('#voiceSelCount').textContent = n ? ('已选 ' + n + ' 个') : '未选择';
    $('#btnDlVoices').disabled = !n;
    $('#btnDelVoices').disabled = !n;
    const all = state.voices.length > 0 && n === state.voices.length;
    $('#voiceSelAll').checked = all;
  }

  function fillVoiceSelect() {
    const sel = $('#selVoice');
    const prev = sel.value;
    if (!state.voices.length) {
      sel.innerHTML = '<option value="">（音色库为空，请先采集）</option>';
      return;
    }
    sel.innerHTML = state.voices.map(function (v) {
      return '<option value="' + v.id + '">' + esc(v.name) +
        (v.durationMs ? ' · ' + dur(v.durationMs) : '') + '</option>';
    }).join('');
    if (prev && state.voices.some(function (v) { return String(v.id) === prev; })) sel.value = prev;
  }

  // ================= 合成 =================
  async function synthesize() {
    const text = $('#ttsText').value.trim();
    if (!text) { toast('请输入要合成的文本', 'err'); return; }
    const max = state.cfg.maxChars || 2000;

    const payloadBase = { mode: state.mode, style: state.style, format: 'wav' };
    if (state.mode === 'clone') {
      const vid = $('#selVoice').value;
      if (!vid) { toast('请先选择一个克隆音色', 'err'); return; }
      payloadBase.voiceId = Number(vid);
    } else if (state.mode === 'preset') {
      payloadBase.preset = $('#selPreset').value;
    } else {
      const d = $('#designHint').value.trim();
      if (!d) { toast('请填写音色描述', 'err'); return; }
      payloadBase.design = d;
    }

    const batch = $('#batchMode').checked;
    let lines = batch ? text.split(/\n+/).map(function (s) { return s.trim(); }).filter(Boolean) : [text];
    if (batch && lines.length === 1) { toast('批量模式下请用换行分隔每句话', 'err'); return; }
    for (const l of lines) {
      if (l.length > max) { toast('单行文本超出 ' + max + ' 字上限', 'err'); return; }
    }

    const btn = $('#btnSynth');
    btn.disabled = true;
    const status = $('#synthStatus');
    let okCount = 0, failCount = 0, last = null;

    for (let i = 0; i < lines.length; i++) {
      const label = lines.length > 1 ? ('生成中 ' + (i + 1) + '/' + lines.length + ' · ') : '生成中…';
      status.innerHTML = '<span class="spinner"></span> ' + esc(label) +
        esc(lines[i].slice(0, 20)) + (lines[i].length > 20 ? '…' : '');
      try {
        const res = await api.post('/api/tts/synthesize', Object.assign({}, payloadBase, { text: lines[i] }));
        okCount++;
        last = res;
      } catch (e) {
        failCount++;
        status.innerHTML = '<span style="color:var(--red)">' + esc(e.message) + '</span>';
        if (lines.length === 1) break;
      }
    }

    if (last) {
      $('#synthResult').classList.remove('hidden');
      $('#synthPlayer').src = last.url;
      $('#synthPlayer').play().catch(function () {});
      const vname = last.render.voiceName || '';
      $('#synthMeta').innerHTML = '<span style="color:var(--text)">' + esc(vname) + '</span>' +
        ' · ' + dur(last.duration) + ' · ' + bytes(last.size) +
        ' · <span class="mono">' + window.UI.ms(last.ms) + '</span>';
      $('#btnDlOne').href = last.url + '?download=1';
      $('#btnDlOne').setAttribute('download', '');
    }

    if (lines.length > 1) {
      status.innerHTML = '批量完成：成功 ' + okCount + ' 句' +
        (failCount ? '，<span style="color:var(--red)">失败 ' + failCount + ' 句</span>' : '');
      toast('已生成 ' + okCount + ' 条音频', okCount ? 'ok' : 'err');
    } else if (okCount) {
      status.textContent = '生成完成';
    }

    btn.disabled = false;
    await loadRenders();
  }

  // ================= 记录 =================
  async function loadRenders() {
    try {
      const res = await api.get('/api/tts/renders');
      state.renders = res.items || [];
    } catch (e) {
      state.renders = [];
    }
    const ids = new Set(state.renders.map(function (r) { return r.id; }));
    state.selRenders.forEach(function (id) { if (!ids.has(id)) state.selRenders.delete(id); });
    renderRenders();
  }

  function renderRenders() {
    const box = $('#renderList');
    $('#renderCount').textContent = state.renders.length;
    if (!state.renders.length) {
      box.innerHTML = '<div class="empty" style="padding:36px 20px">还没有生成记录</div>';
    } else {
      box.innerHTML = state.renders.map(function (r) {
        const sel = state.selRenders.has(r.id) ? ' sel' : '';
        return '<div class="render-item' + sel + '" data-id="' + r.id + '">' +
          '<input type="checkbox" class="rsel" data-id="' + r.id + '"' + (sel ? ' checked' : '') +
          ' style="accent-color:var(--accent)">' +
          '<span class="txt" title="' + esc(r.text) + '">' + esc(r.text) + '</span>' +
          '<audio preload="none" controls src="/api/tts/renders/' + r.id + '/audio"></audio>' +
          '<span class="meta">' + (r.durationMs ? dur(r.durationMs) : '—') + ' · ' + bytes(r.size) + '</span>' +
          '<a class="iconbtn" href="/api/tts/renders/' + r.id + '/audio?download=1" title="下载">' + icon('download', 12) + '</a>' +
          '<button class="iconbtn danger" data-act="delrender" data-id="' + r.id + '" title="删除">' + icon('trash', 12) + '</button>' +
          '</div>';
      }).join('');
    }
    updateRenderBar();
  }

  function updateRenderBar() {
    const n = state.selRenders.size;
    $('#renderSelCount').textContent = n ? ('已选 ' + n + ' 条') : '未选择';
    $('#btnDlRenders').disabled = !n;
    $('#btnDelRenders').disabled = !n;
    $('#renderSelAll').checked = state.renders.length > 0 && n === state.renders.length;
  }

  // ================= 交互绑定 =================
  function bind() {
    // 采集方式切换
    $('#srcTabs').addEventListener('click', function (e) {
      const b = e.target.closest('button[data-src]');
      if (!b) return;
      state.src = b.getAttribute('data-src');
      $$('#srcTabs button').forEach(function (x) { x.classList.remove('on'); });
      b.classList.add('on');
      $('#paneUpload').classList.toggle('hidden', state.src !== 'upload');
      $('#paneRecord').classList.toggle('hidden', state.src !== 'record');
      if (state.src !== 'record' && recorder && recorder.state === 'recording') stopRec();
    });

    // 上传
    const dz = $('#dropzone');
    dz.addEventListener('click', function () { $('#fileInput').click(); });
    dz.addEventListener('dragover', function (e) { e.preventDefault(); dz.classList.add('drag'); });
    dz.addEventListener('dragleave', function () { dz.classList.remove('drag'); });
    dz.addEventListener('drop', function (e) {
      e.preventDefault();
      dz.classList.remove('drag');
      if (e.dataTransfer.files.length) handleFile(e.dataTransfer.files[0]);
    });
    $('#fileInput').addEventListener('change', function (e) {
      if (e.target.files.length) handleFile(e.target.files[0]);
    });
    $('#fileClear').addEventListener('click', function (e) {
      e.stopPropagation();
      resetPending();
    });

    // 录音
    $('#recBtn').addEventListener('click', function () {
      if (recorder && recorder.state === 'recording') stopRec();
      else startRec();
    });

    $('#btnSaveVoice').addEventListener('click', saveVoice);

    // 音色库
    $('#voiceGrid').addEventListener('change', function (e) {
      if (!e.target.classList.contains('vsel')) return;
      const id = Number(e.target.getAttribute('data-id'));
      if (e.target.checked) state.selVoices.add(id); else state.selVoices.delete(id);
      const card = e.target.closest('.voice-item');
      if (card) card.classList.toggle('sel', e.target.checked);
      updateVoiceBar();
    });
    $('#voiceGrid').addEventListener('click', async function (e) {
      const b = e.target.closest('[data-act]');
      if (!b) return;
      const id = Number(b.getAttribute('data-id'));
      const act = b.getAttribute('data-act');
      if (act === 'del') {
        if (!confirm('确定删除这个音色？')) return;
        try {
          await api.post('/api/tts/voices/delete', { ids: [id] });
          state.selVoices.delete(id);
          toast('已删除', 'ok');
          await loadVoices();
        } catch (err) { toast(err.message, 'err'); }
      } else if (act === 'rename') {
        const v = state.voices.find(function (x) { return x.id === id; });
        const name = prompt('新的音色名称', v ? v.name : '');
        if (!name || !name.trim()) return;
        try {
          await api.patch('/api/tts/voices/' + id, { name: name.trim() });
          await loadVoices();
          toast('已重命名', 'ok');
        } catch (err) { toast(err.message, 'err'); }
      }
    });
    $('#voiceSelAll').addEventListener('change', function (e) {
      state.selVoices.clear();
      if (e.target.checked) state.voices.forEach(function (v) { state.selVoices.add(v.id); });
      renderVoices();
    });
    $('#btnDlVoices').addEventListener('click', function () {
      if (!state.selVoices.size) return;
      location.href = '/api/tts/voices/download?ids=' + Array.from(state.selVoices).join(',');
      toast('正在打包下载 ' + state.selVoices.size + ' 个音色', 'ok');
    });
    $('#btnDelVoices').addEventListener('click', async function () {
      const n = state.selVoices.size;
      if (!n) return;
      if (!confirm('确定删除选中的 ' + n + ' 个音色？此操作不可恢复。')) return;
      try {
        await api.post('/api/tts/voices/delete', { ids: Array.from(state.selVoices) });
        state.selVoices.clear();
        toast('已删除 ' + n + ' 个音色', 'ok');
        await loadVoices();
      } catch (e) { toast(e.message, 'err'); }
    });
    $('#btnClearVoices').addEventListener('click', async function () {
      if (!state.voices.length) { toast('音色库已经是空的'); return; }
      if (!confirm('确定清空全部 ' + state.voices.length + ' 个音色？此操作不可恢复。')) return;
      try {
        const res = await api.post('/api/tts/voices/clear', {});
        state.selVoices.clear();
        toast('已清空 ' + res.deleted + ' 个音色', 'ok');
        await loadVoices();
      } catch (e) { toast(e.message, 'err'); }
    });
    $('#btnRefreshVoices').addEventListener('click', loadVoices);

    // 合成模式
    $('#modeTabs').addEventListener('click', function (e) {
      const b = e.target.closest('button[data-mode]');
      if (!b) return;
      state.mode = b.getAttribute('data-mode');
      $$('#modeTabs button').forEach(function (x) { x.classList.remove('on'); });
      b.classList.add('on');
      $('#voicePick').classList.toggle('hidden', state.mode !== 'clone');
      $('#presetPick').classList.toggle('hidden', state.mode !== 'preset');
      $('#designPick').classList.toggle('hidden', state.mode !== 'design');
    });

    // 字数
    $('#ttsText').addEventListener('input', function () {
      const n = $('#ttsText').value.length;
      const max = state.cfg.maxChars || 2000;
      const el = $('#charCount');
      el.textContent = n + ' / ' + max;
      el.style.color = n > max ? 'var(--red)' : 'var(--text-3)';
    });
    $('#batchMode').addEventListener('change', function (e) {
      $('#batchHint').textContent = e.target.checked
        ? '每一行单独生成一个音频，适合批量配音'
        : '长文本会自动按标点分段合成后拼接';
    });

    $('#btnSynth').addEventListener('click', synthesize);
    $('#btnRefreshRenders').addEventListener('click', loadRenders);

    // 记录
    $('#renderList').addEventListener('change', function (e) {
      if (!e.target.classList.contains('rsel')) return;
      const id = Number(e.target.getAttribute('data-id'));
      if (e.target.checked) state.selRenders.add(id); else state.selRenders.delete(id);
      const row = e.target.closest('.render-item');
      if (row) row.classList.toggle('sel', e.target.checked);
      updateRenderBar();
    });
    $('#renderList').addEventListener('click', async function (e) {
      const b = e.target.closest('[data-act="delrender"]');
      if (!b) return;
      const id = Number(b.getAttribute('data-id'));
      try {
        await api.post('/api/tts/renders/delete', { ids: [id] });
        state.selRenders.delete(id);
        await loadRenders();
      } catch (err) { toast(err.message, 'err'); }
    });
    $('#renderSelAll').addEventListener('change', function (e) {
      state.selRenders.clear();
      if (e.target.checked) state.renders.forEach(function (r) { state.selRenders.add(r.id); });
      renderRenders();
    });
    $('#btnDlRenders').addEventListener('click', function () {
      if (!state.selRenders.size) return;
      location.href = '/api/tts/renders/download?ids=' + Array.from(state.selRenders).join(',');
      toast('正在打包下载 ' + state.selRenders.size + ' 条音频', 'ok');
    });
    $('#btnDelRenders').addEventListener('click', async function () {
      const n = state.selRenders.size;
      if (!n) return;
      if (!confirm('确定删除选中的 ' + n + ' 条记录？')) return;
      try {
        await api.post('/api/tts/renders/delete', { ids: Array.from(state.selRenders) });
        state.selRenders.clear();
        toast('已删除 ' + n + ' 条', 'ok');
        await loadRenders();
      } catch (e) { toast(e.message, 'err'); }
    });
    $('#btnClearRenders').addEventListener('click', async function () {
      if (!state.renders.length) { toast('记录已经是空的'); return; }
      if (!confirm('确定清空全部 ' + state.renders.length + ' 条合成记录？此操作不可恢复。')) return;
      try {
        const res = await api.post('/api/tts/renders/clear', {});
        state.selRenders.clear();
        toast('已清空 ' + res.deleted + ' 条记录', 'ok');
        await loadRenders();
      } catch (e) { toast(e.message, 'err'); }
    });
  }

  // ================= 启动 =================
  async function boot() {
    let bootData;
    try {
      bootData = await api.get('/api/public/bootstrap');
    } catch (e) {
      document.body.innerHTML = '<div style="padding:80px;text-align:center;color:#9aa2ad">无法连接服务</div>';
      return;
    }
    renderHeader($('#header'), bootData.site, 'tts', bootData.me);
    renderFooter($('#footer'), bootData.site);
    renderAds(bootData.ads);

    let cfg;
    try {
      cfg = await api.get('/api/tts/config');
    } catch (e) {
      $('#limitHint').textContent = '音色工坊不可用';
      return;
    }
    state.cfg = cfg;

    if (!cfg.enabled) {
      document.querySelector('.tts-shell').innerHTML =
        '<div class="panel" style="padding:60px;text-align:center;grid-column:1/-1">' +
        '<h2 style="font-size:20px;margin-bottom:10px">音色工坊已关闭</h2>' +
        '<p style="color:var(--text-3);font-size:14px">管理员暂未开放此功能。</p></div>';
      return;
    }

    $('#limitHint').textContent = '样本 ≤ ' + cfg.maxUploadMb + ' MB · WAV / MP3';
    $('#dzHint').textContent = '支持 WAV / MP3，单个不超过 ' + cfg.maxUploadMb + ' MB';
    $('#cfgMax').textContent = cfg.maxUploadMb + ' MB';
    $('#cfgChars').textContent = cfg.maxChars + ' 字';
    $('#cfgKey').innerHTML = cfg.hasKey
      ? '<span style="color:var(--green)">已配置</span>'
      : '<span style="color:var(--red)">未配置</span>';
    $('#cfgGuest').textContent = cfg.guestAllowed ? '游客可用' : '需登录';
    $('#keyHint').innerHTML = cfg.hasKey
      ? ''
      : '<span style="color:var(--red)">后台尚未配置 MIMO 密钥，生成会失败</span>';

    // 预置音色
    $('#selPreset').innerHTML = (cfg.presets || []).map(function (p) {
      return '<option value="' + esc(p.id) + '">' + esc(p.name) + ' · ' + esc(p.lang.toUpperCase()) + ' ' + esc(p.sex) + '</option>';
    }).join('');
    if (cfg.defaultVoice) $('#selPreset').value = cfg.defaultVoice;

    // 风格
    const chips = $('#styleChips');
    chips.innerHTML = (cfg.styles || []).map(function (s, i) {
      return '<button data-prompt="' + esc(s.prompt) + '" class="' + (i === 0 ? 'on' : '') + '">' + esc(s.name) + '</button>';
    }).join('');
    chips.addEventListener('click', function (e) {
      const b = e.target.closest('button');
      if (!b) return;
      $$('#styleChips button').forEach(function (x) { x.classList.remove('on'); });
      b.classList.add('on');
      state.style = b.getAttribute('data-prompt') || '';
    });
    state.style = '';

    $('#dzIcon').innerHTML = icon('upload', 26);

    bind();
    await loadVoices();
    await loadRenders();
  }

  document.addEventListener('DOMContentLoaded', boot);
})();
