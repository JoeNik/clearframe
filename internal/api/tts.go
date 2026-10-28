package api

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"jxivideo/internal/store"
	"jxivideo/internal/tts"
)

func (s *Server) ttsClient() (*tts.Client, store.TTSConfig) {
	cfg := s.st.Settings().TTS
	return tts.New(cfg.BaseURL, cfg.APIKey, cfg.Model, cfg.PresetModel, cfg.DesignModel,
		time.Duration(cfg.TimeoutSec)*time.Second), cfg
}

func (s *Server) handleTTSConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.st.Settings()
	t := cfg.TTS
	writeOK(w, map[string]any{
		"enabled":      t.Enabled,
		"hasKey":       strings.TrimSpace(t.APIKey) != "",
		"maxUploadMb":  t.MaxUploadMB,
		"maxChars":     t.MaxChars,
		"defaultVoice": t.DefaultVoice,
		"guestAllowed": t.GuestAllowed,
		"presets":      tts.PresetVoices,
		"styles":       tts.StylePresets,
		"models": map[string]string{
			"clone":  t.Model,
			"preset": t.PresetModel,
			"design": t.DesignModel,
		},
	})
}

// requireTTS 校验音色工坊可用性，返回 owner 标识
func (s *Server) requireTTS(w http.ResponseWriter, r *http.Request) (string, bool) {
	cfg := s.st.Settings()
	if !cfg.TTS.Enabled {
		writeErr(w, http.StatusForbidden, "disabled", "音色工坊已被管理员关闭")
		return "", false
	}
	_, logged := s.currentUser(r)
	if !cfg.TTS.GuestAllowed && !logged {
		writeErr(w, http.StatusUnauthorized, "login_required", "音色工坊需要登录后使用")
		return "", false
	}
	return s.ownerKey(w, r), true
}

func (s *Server) handleListVoices(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.requireTTS(w, r)
	if !ok {
		return
	}
	list, err := s.st.ListVoices(owner, 300)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "读取音色失败")
		return
	}
	writeOK(w, map[string]any{"items": list})
}

// handleCreateVoice 上传或录音保存音色
func (s *Server) handleCreateVoice(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.requireTTS(w, r)
	if !ok {
		return
	}
	cfg := s.st.Settings().TTS
	maxBytes := int64(cfg.MaxUploadMB) << 20
	if maxBytes <= 0 {
		maxBytes = 2 << 20
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	if err := r.ParseMultipartForm(maxBytes + (256 << 10)); err != nil {
		writeErr(w, http.StatusBadRequest, "too_large", fmt.Sprintf("音频超出 %d MB 限制", cfg.MaxUploadMB))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	file, hdr, err := r.FormFile("audio")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "no_file", "请选择要上传的音频文件")
		return
	}
	defer file.Close()

	if hdr.Size > maxBytes {
		writeErr(w, http.StatusBadRequest, "too_large", fmt.Sprintf("音频超出 %d MB 限制", cfg.MaxUploadMB))
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "read_failed", "读取音频失败")
		return
	}
	if int64(len(data)) > maxBytes {
		writeErr(w, http.StatusBadRequest, "too_large", fmt.Sprintf("音频超出 %d MB 限制", cfg.MaxUploadMB))
		return
	}
	if len(data) < 1024 {
		writeErr(w, http.StatusBadRequest, "too_short", "音频过短或已损坏，建议 10 秒以上")
		return
	}

	mime := tts.DetectMime(data)
	if mime == "" {
		writeErr(w, http.StatusBadRequest, "bad_format",
			"仅支持 WAV / MP3 格式。浏览器录音已自动转为 WAV，请重新录制")
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = "音色 " + time.Now().Format("01-02 15:04")
	}
	if len([]rune(name)) > 40 {
		name = string([]rune(name)[:40])
	}
	source := r.FormValue("source")
	if source != "record" {
		source = "upload"
	}
	durationMs, _ := strconv.Atoi(r.FormValue("durationMs"))
	if durationMs <= 0 {
		durationMs = tts.EstimateDurationMs(data, mime)
	}

	v, err := s.st.CreateVoice(store.Voice{
		Name: name, Owner: owner, Mime: mime, DurationMs: durationMs, Source: source,
	}, data)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "保存音色失败")
		return
	}
	s.st.IncrCounter("voices_total", 1)
	writeOK(w, map[string]any{"voice": v})
}

func (s *Server) handleRenameVoice(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.requireTTS(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "名称不能为空")
		return
	}
	v, err := s.st.GetVoice(pathID(r))
	if err != nil || v.Owner != owner {
		writeErr(w, http.StatusNotFound, "not_found", "音色不存在")
		return
	}
	if err := s.st.RenameVoice(v.ID, strings.TrimSpace(body.Name)); err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "重命名失败")
		return
	}
	writeOK(w, map[string]any{"ok": true})
}

func (s *Server) handleVoiceAudio(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.requireTTS(w, r)
	if !ok {
		return
	}
	id := pathID(r)
	v, err := s.st.GetVoice(id)
	if err != nil || (v.Owner != owner && !isAdminRequest(s, r)) {
		writeErr(w, http.StatusNotFound, "not_found", "音色不存在")
		return
	}
	data, mime, err := s.st.VoiceAudio(id)
	if err != nil || len(data) == 0 {
		writeErr(w, http.StatusNotFound, "not_found", "音频不存在")
		return
	}
	serveAudio(w, r, data, mime, safeFilename(v.Name)+extForMime(mime))
}

func (s *Server) handleDeleteVoices(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.requireTTS(w, r)
	if !ok {
		return
	}
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := readJSON(r, &body); err != nil || len(body.IDs) == 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "请选择要删除的音色")
		return
	}
	n, err := s.st.DeleteVoices(body.IDs, owner)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "删除失败")
		return
	}
	writeOK(w, map[string]any{"deleted": n})
}

func (s *Server) handleClearVoices(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.requireTTS(w, r)
	if !ok {
		return
	}
	list, err := s.st.ListVoices(owner, 5000)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "读取失败")
		return
	}
	ids := make([]int64, 0, len(list))
	for _, v := range list {
		ids = append(ids, v.ID)
	}
	n, _ := s.st.DeleteVoices(ids, owner)
	writeOK(w, map[string]any{"deleted": n})
}

// handleDownloadVoices 打包下载选中音色
func (s *Server) handleDownloadVoices(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.requireTTS(w, r)
	if !ok {
		return
	}
	ids := parseIDs(r.URL.Query().Get("ids"))
	if len(ids) == 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "请选择要下载的音色")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition",
		`attachment; filename="voices-`+time.Now().Format("20060102-1504")+`.zip"`)
	zw := zip.NewWriter(w)
	defer zw.Close()

	used := map[string]int{}
	for _, id := range ids {
		v, err := s.st.GetVoice(id)
		if err != nil || v.Owner != owner {
			continue
		}
		data, mime, err := s.st.VoiceAudio(id)
		if err != nil || len(data) == 0 {
			continue
		}
		base := safeFilename(v.Name)
		if c := used[base]; c > 0 {
			used[base] = c + 1
			base = fmt.Sprintf("%s-%d", base, c+1)
		} else {
			used[base] = 1
		}
		f, err := zw.Create(base + extForMime(mime))
		if err != nil {
			return
		}
		_, _ = f.Write(data)
	}
}

// handleSynthesize 合成语音并落库
func (s *Server) handleSynthesize(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.requireTTS(w, r)
	if !ok {
		return
	}
	cfg := s.st.Settings().TTS

	var body struct {
		Mode      string `json:"mode"`
		VoiceID   int64  `json:"voiceId"`
		Preset    string `json:"preset"`
		Design    string `json:"design"`
		Text      string `json:"text"`
		Style     string `json:"style"`
		Format    string `json:"format"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式有误")
		return
	}
	body.Text = strings.TrimSpace(body.Text)
	if body.Text == "" {
		writeErr(w, http.StatusBadRequest, "empty_text", "请输入要合成的文本")
		return
	}
	if len([]rune(body.Text)) > cfg.MaxChars {
		writeErr(w, http.StatusBadRequest, "too_long",
			fmt.Sprintf("文本超出 %d 字上限", cfg.MaxChars))
		return
	}

	client, _ := s.ttsClient()
	req := tts.SynthRequest{
		Mode:     body.Mode,
		Text:     body.Text,
		Style:    strings.TrimSpace(body.Style),
		Format:   body.Format,
		MaxChars: 300,
	}

	var voiceName string
	switch body.Mode {
	case tts.ModePreset:
		req.Mode = tts.ModePreset
		req.Voice = body.Preset
		if req.Voice == "" {
			req.Voice = cfg.DefaultVoice
		}
		voiceName = "预置 · " + req.Voice
	case tts.ModeDesign:
		req.Mode = tts.ModeDesign
		req.DesignHint = strings.TrimSpace(body.Design)
		if req.DesignHint == "" {
			writeErr(w, http.StatusBadRequest, "bad_request", "请填写音色设计描述")
			return
		}
		voiceName = "设计音色"
	default:
		if body.VoiceID <= 0 {
			writeErr(w, http.StatusBadRequest, "no_voice", "请先选择一个克隆音色")
			return
		}
		v, err := s.st.GetVoice(body.VoiceID)
		if err != nil || (v.Owner != owner && !isAdminRequest(s, r)) {
			writeErr(w, http.StatusNotFound, "not_found", "音色不存在")
			return
		}
		sample, mime, err := s.st.VoiceAudio(v.ID)
		if err != nil || len(sample) == 0 {
			writeErr(w, http.StatusNotFound, "not_found", "音色样本已丢失，请重新上传")
			return
		}
		req.Mode = tts.ModeClone
		req.VoiceClone = tts.DataURI(mime, sample)
		req.VoiceID = v.ID
		voiceName = v.Name
	}

	ctx, cancel := context.WithTimeout(r.Context(),
		time.Duration(cfg.TimeoutSec)*time.Second)
	defer cancel()

	start := time.Now()
	audio, err := client.Synthesize(ctx, req)
	if err != nil {
		msg := err.Error()
		code := http.StatusBadGateway
		kind := "upstream"
		if strings.Contains(msg, "尚未配置") {
			code, kind = http.StatusServiceUnavailable, "not_configured"
		}
		writeErr(w, code, kind, msg)
		return
	}
	elapsed := time.Since(start).Milliseconds()

	format := "wav"
	if body.Format == "mp3" {
		format = "wav" // 上游统一返回 wav，如需 mp3 由前端转码
	}
	render, err := s.st.CreateRender(store.Render{
		Owner: owner, VoiceID: req.VoiceID, VoiceName: voiceName,
		Text: body.Text, Style: req.Style, Format: format,
		DurationMs: tts.WAVDurationMs(audio),
	}, audio)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "保存合成结果失败")
		return
	}
	s.st.IncrCounter("renders_total", 1)

	writeOK(w, map[string]any{
		"render":   render,
		"url":      "/api/tts/renders/" + strconv.FormatInt(render.ID, 10) + "/audio",
		"ms":       elapsed,
		"size":     len(audio),
		"duration": render.DurationMs,
	})
}

func (s *Server) handleListRenders(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.requireTTS(w, r)
	if !ok {
		return
	}
	list, err := s.st.ListRenders(owner, 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "读取失败")
		return
	}
	writeOK(w, map[string]any{"items": list})
}

func (s *Server) handleRenderAudio(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.requireTTS(w, r)
	if !ok {
		return
	}
	id := pathID(r)
	data, format, text, err := s.st.RenderAudio(id)
	if err != nil || len(data) == 0 {
		writeErr(w, http.StatusNotFound, "not_found", "音频不存在")
		return
	}
	if !isAdminRequest(s, r) {
		list, _ := s.st.ListRenders(owner, 1000)
		found := false
		for _, x := range list {
			if x.ID == id {
				found = true
				break
			}
		}
		if !found {
			writeErr(w, http.StatusNotFound, "not_found", "音频不存在")
			return
		}
	}
	name := "tts-" + strconv.FormatInt(id, 10)
	if t := strings.TrimSpace(text); t != "" {
		if runes := []rune(t); len(runes) > 12 {
			name = string(runes[:12])
		} else {
			name = t
		}
	}
	serveAudio(w, r, data, "audio/"+format, safeFilename(name)+".wav")
}

func (s *Server) handleDeleteRenders(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.requireTTS(w, r)
	if !ok {
		return
	}
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := readJSON(r, &body); err != nil || len(body.IDs) == 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "请选择要删除的记录")
		return
	}
	n, err := s.st.DeleteRenders(body.IDs, owner)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "删除失败")
		return
	}
	writeOK(w, map[string]any{"deleted": n})
}

func (s *Server) handleClearRenders(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.requireTTS(w, r)
	if !ok {
		return
	}
	list, _ := s.st.ListRenders(owner, 1000)
	ids := make([]int64, 0, len(list))
	for _, x := range list {
		ids = append(ids, x.ID)
	}
	n, _ := s.st.DeleteRenders(ids, owner)
	writeOK(w, map[string]any{"deleted": n})
}

func (s *Server) handleDownloadRenders(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.requireTTS(w, r)
	if !ok {
		return
	}
	ids := parseIDs(r.URL.Query().Get("ids"))
	if len(ids) == 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "请选择要下载的音频")
		return
	}
	mine := map[int64]bool{}
	if list, err := s.st.ListRenders(owner, 1000); err == nil {
		for _, x := range list {
			mine[x.ID] = true
		}
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition",
		`attachment; filename="tts-`+time.Now().Format("20060102-1504")+`.zip"`)
	zw := zip.NewWriter(w)
	defer zw.Close()

	for i, id := range ids {
		if !mine[id] && !isAdminRequest(s, r) {
			continue
		}
		data, _, text, err := s.st.RenderAudio(id)
		if err != nil || len(data) == 0 {
			continue
		}
		base := strings.TrimSpace(text)
		if runes := []rune(base); len(runes) > 20 {
			base = string(runes[:20])
		}
		base = safeFilename(base)
		if base == "" {
			base = "tts"
		}
		f, err := zw.Create(fmt.Sprintf("%02d-%s.wav", i+1, base))
		if err != nil {
			return
		}
		_, _ = f.Write(data)
	}
}

// ---------- 工具 ----------

func parseIDs(s string) []int64 {
	var out []int64
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.ParseInt(part, 10, 64); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	return out
}

func safeFilename(s string) string {
	s = strings.TrimSpace(s)
	bad := []string{"/", "\\", ":", "*", "?", `"`, "<", ">", "|", "\n", "\r", "\x00"}
	for _, b := range bad {
		s = strings.ReplaceAll(s, b, "-")
	}
	s = strings.Trim(s, ".- ")
	if s == "" {
		s = "audio"
	}
	if len([]rune(s)) > 60 {
		s = string([]rune(s)[:60])
	}
	return s
}

func extForMime(mime string) string {
	switch {
	case strings.Contains(mime, "mpeg"), strings.Contains(mime, "mp3"):
		return ".mp3"
	case strings.Contains(mime, "mp4"), strings.Contains(mime, "m4a"):
		return ".m4a"
	case strings.Contains(mime, "ogg"):
		return ".ogg"
	default:
		return ".wav"
	}
}

func serveAudio(w http.ResponseWriter, r *http.Request, data []byte, mime, filename string) {
	if mime == "" {
		mime = "audio/wav"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Accept-Ranges", "none")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition",
			`attachment; filename="`+filename+`"; filename*=UTF-8''`+urlEncode(filename))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func urlEncode(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			b.WriteString("%")
			const hex = "0123456789ABCDEF"
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0xF])
		}
	}
	return b.String()
}

var _ = json.Marshal
var _ = filepath.Base

// ---------- 参考音色 ----------

// handleRefVoices 参考音色目录（官方预置 + 原创设计，含试听缓存状态）
func (s *Server) handleRefVoices(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireTTS(w, r); !ok {
		return
	}
	items := tts.ReferenceVoices()
	for i := range items {
		items[i].Demo = s.st.HasVoiceDemo(items[i].Key)
	}
	writeOK(w, map[string]any{"items": items})
}

// handleRefVoiceDemo 播放试听：命中缓存直接回，未命中现场合成并缓存
func (s *Server) handleRefVoiceDemo(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireTTS(w, r); !ok {
		return
	}
	key := r.PathValue("key")
	v, found := tts.FindReferenceVoice(key)
	if !found {
		writeErr(w, http.StatusNotFound, "not_found", "参考音色不存在")
		return
	}
	if audio, mime, ok := s.st.GetVoiceDemo(key); ok {
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Cache-Control", "private, max-age=86400")
		w.Write(audio)
		return
	}
	client, _ := s.ttsClient()
	req := tts.SynthRequest{Text: v.Sentence, Format: "wav", MaxChars: 300}
	if v.Kind == "preset" {
		req.Mode = tts.ModePreset
		req.Voice = v.Voice
	} else {
		req.Mode = tts.ModeDesign
		req.DesignHint = v.Hint
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	audio, err := client.Synthesize(ctx, req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "synth", err.Error())
		return
	}
	mime := "audio/wav"
	s.st.SaveVoiceDemo(key, audio, mime)
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Write(audio)
}
