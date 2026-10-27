package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"jxivideo/internal/auth"
	"jxivideo/internal/store"
)

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	stats := s.st.Stats()
	logs, total, _ := s.st.ListLogs(12, 0)
	voices, _ := s.st.ListVoices("", 1000)
	renders, _ := s.st.ListRenders("", 1000)

	var voiceBytes, renderBytes int64
	for _, v := range voices {
		voiceBytes += v.Size
	}
	for _, x := range renders {
		renderBytes += x.Size
	}
	writeOK(w, map[string]any{
		"stats":      stats,
		"recentLogs": logs,
		"logTotal":   total,
		"voices":     len(voices),
		"renders":    len(renders),
		"voiceBytes": voiceBytes,
		"renderBytes": renderBytes,
		"users":      s.st.CountUsers(),
		"serverTime": time.Now().Format("2006-01-02 15:04:05"),
	})
}

// ---------- 接口管理 ----------

func (s *Server) handleAdminInterfaces(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	list, total, err := s.st.ListInterfaces(status, r.URL.Query().Get("category"),
		r.URL.Query().Get("q"), queryInt(r, "limit", 500), queryInt(r, "offset", 0))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "读取失败")
		return
	}
	writeOK(w, map[string]any{"items": list, "total": total})
}

func (s *Server) handleAdminSaveInterface(w http.ResponseWriter, r *http.Request) {
	var it store.Interface
	if err := readJSON(r, &it); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式有误: "+err.Error())
		return
	}
	if strings.TrimSpace(it.Name) == "" || strings.TrimSpace(it.Slug) == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "接口名称与标识不能为空")
		return
	}
	it.ID = 0
	saved, err := s.st.SaveInterface(it)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "save_failed", "保存失败: "+err.Error())
		return
	}
	writeOK(w, map[string]any{"interface": saved})
}

func (s *Server) handleAdminUpdateInterface(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	cur, err := s.st.GetInterfaceByID(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	var it store.Interface
	if err := readJSON(r, &it); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式有误: "+err.Error())
		return
	}
	it.ID = cur.ID
	it.CreatedAt = cur.CreatedAt
	saved, err := s.st.SaveInterface(it)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "save_failed", "保存失败: "+err.Error())
		return
	}
	writeOK(w, map[string]any{"interface": saved})
}

func (s *Server) handleAdminPatchInterface(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式有误")
		return
	}
	if err := s.st.PatchInterface(id, body); err != nil {
		writeErr(w, http.StatusBadRequest, "patch_failed", "更新失败: "+err.Error())
		return
	}
	it, err := s.st.GetInterfaceByID(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	writeOK(w, map[string]any{"interface": it})
}

func (s *Server) handleAdminDeleteInterface(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteInterface(pathID(r)); err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "删除失败")
		return
	}
	writeOK(w, map[string]any{"ok": true})
}

// ---------- 分类 ----------

func (s *Server) handleAdminCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := s.st.Categories()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "读取失败")
		return
	}
	stats := s.st.Stats()
	writeOK(w, map[string]any{"items": cats, "counts": stats.Categories})
}

func (s *Server) handleAdminSaveCategory(w http.ResponseWriter, r *http.Request) {
	var c store.Category
	if err := readJSON(r, &c); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式有误")
		return
	}
	saved, err := s.st.UpsertCategory(c)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "save_failed", "保存失败: "+err.Error())
		return
	}
	writeOK(w, map[string]any{"category": saved})
}

func (s *Server) handleAdminDeleteCategory(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteCategory(r.PathValue("slug")); err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "删除失败")
		return
	}
	writeOK(w, map[string]any{"ok": true})
}

// ---------- 设置 ----------

func (s *Server) handleAdminGetSettings(w http.ResponseWriter, r *http.Request) {
	writeOK(w, map[string]any{"settings": s.st.Settings()})
}

func (s *Server) handleAdminSaveSettings(w http.ResponseWriter, r *http.Request) {
	var cfg store.Settings
	if err := readJSON(r, &cfg); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式有误: "+err.Error())
		return
	}
	if strings.TrimSpace(cfg.SiteName) == "" {
		cfg.SiteName = "清影 ClearFrame"
	}
	if cfg.RateLimitPerMin <= 0 {
		cfg.RateLimitPerMin = 120
	}
	if cfg.RateLimitBurst <= 0 {
		cfg.RateLimitBurst = 40
	}
	if cfg.MaxBodyMB <= 0 {
		cfg.MaxBodyMB = 4
	}
	if cfg.MaxBodyMB > 64 {
		cfg.MaxBodyMB = 64
	}
	if cfg.TTS.MaxUploadMB <= 0 {
		cfg.TTS.MaxUploadMB = 2
	}
	if cfg.TTS.MaxUploadMB > 10 {
		cfg.TTS.MaxUploadMB = 10
	}
	if cfg.TTS.MaxChars <= 0 {
		cfg.TTS.MaxChars = 2000
	}
	if cfg.TTS.TimeoutSec <= 0 {
		cfg.TTS.TimeoutSec = 90
	}
	if cfg.TTS.BaseURL == "" {
		cfg.TTS.BaseURL = "https://api.xiaomimimo.com/v1"
	}
	if cfg.TTS.Model == "" {
		cfg.TTS.Model = "mimo-v2.5-tts-voiceclone"
	}
	if err := s.st.SaveSettings(cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "保存失败")
		return
	}
	writeOK(w, map[string]any{"settings": s.st.Settings()})
}

// ---------- 广告位 ----------

func (s *Server) handleAdminAds(w http.ResponseWriter, r *http.Request) {
	slots, err := s.st.AdSlots()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "读取失败")
		return
	}
	writeOK(w, map[string]any{"items": slots})
}

func (s *Server) handleAdminSaveAd(w http.ResponseWriter, r *http.Request) {
	var a store.AdSlot
	if err := readJSON(r, &a); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式有误")
		return
	}
	if a.Slot == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "广告位标识不能为空")
		return
	}
	if err := s.st.SaveAdSlot(a); err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "保存失败")
		return
	}
	writeOK(w, map[string]any{"ok": true})
}

// ---------- 用户 ----------

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.st.ListUsers()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "读取失败")
		return
	}
	writeOK(w, map[string]any{"items": users})
}

type createUserReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
	Note     string `json:"note"`
}

func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式有误")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if len(req.Username) < 3 {
		writeErr(w, http.StatusBadRequest, "bad_request", "用户名至少 3 位")
		return
	}
	if len(req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "weak_password", "密码至少 8 位")
		return
	}
	if req.Role != "admin" {
		req.Role = "user"
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash", "密码处理失败")
		return
	}
	u, err := s.st.CreateUser(req.Username, hash, req.Role, req.Note)
	if err != nil {
		writeErr(w, http.StatusConflict, "exists", "用户名已存在")
		return
	}
	writeOK(w, map[string]any{"user": u})
}

type updateUserReq struct {
	Password string `json:"password"`
	Role     string `json:"role"`
	Status   string `json:"status"`
	Note     string `json:"note"`
}

func (s *Server) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	var req updateUserReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式有误")
		return
	}
	hash := ""
	if req.Password != "" {
		if len(req.Password) < 8 {
			writeErr(w, http.StatusBadRequest, "weak_password", "密码至少 8 位")
			return
		}
		h, err := auth.HashPassword(req.Password)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "hash", "密码处理失败")
			return
		}
		hash = h
	}
	if req.Role != "" && req.Role != "admin" {
		req.Role = "user"
	}
	if err := s.st.UpdateUser(id, hash, req.Role, req.Status, req.Note); err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "保存失败")
		return
	}
	writeOK(w, map[string]any{"ok": true})
}

func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	me, _ := s.currentUser(r)
	if me.ID == id {
		writeErr(w, http.StatusBadRequest, "self", "不能删除当前登录账号")
		return
	}
	if err := s.st.DeleteUser(id); err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "删除失败")
		return
	}
	writeOK(w, map[string]any{"ok": true})
}

// ---------- 日志 ----------

func (s *Server) handleAdminLogs(w http.ResponseWriter, r *http.Request) {
	logs, total, err := s.st.ListLogs(queryInt(r, "limit", 100), queryInt(r, "offset", 0))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "读取失败")
		return
	}
	writeOK(w, map[string]any{"items": logs, "total": total})
}

// ---------- 导入导出 ----------

func (s *Server) handleAdminExport(w http.ResponseWriter, r *http.Request) {
	list, _, err := s.st.ListInterfaces("", "", "", 5000, 0)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "导出失败")
		return
	}
	cats, _ := s.st.Categories()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="interfaces-`+
		time.Now().Format("20060102-1504")+`.json"`)
	writeJSON(w, http.StatusOK, map[string]any{
		"version":    1,
		"exportedAt": time.Now().Format(time.RFC3339),
		"categories": cats,
		"interfaces": list,
	})
}

type importReq struct {
	Wipe       bool              `json:"wipe"`
	Interfaces []store.Interface `json:"interfaces"`
	Categories []store.Category  `json:"categories"`
}

func (s *Server) handleAdminImport(w http.ResponseWriter, r *http.Request) {
	var req importReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "JSON 解析失败: "+err.Error())
		return
	}
	if len(req.Interfaces) == 0 && len(req.Categories) == 0 {
		writeErr(w, http.StatusBadRequest, "empty", "没有可导入的数据")
		return
	}
	for _, c := range req.Categories {
		if _, err := s.st.UpsertCategory(c); err != nil {
			writeErr(w, http.StatusBadRequest, "import_failed", "分类导入失败: "+err.Error())
			return
		}
	}
	n, err := s.st.BulkReplaceInterfaces(req.Interfaces, req.Wipe)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "import_failed", "导入失败: "+err.Error())
		return
	}
	writeOK(w, map[string]any{"imported": n, "categories": len(req.Categories)})
}

// ---------- 音色（全量） ----------

func (s *Server) handleAdminVoices(w http.ResponseWriter, r *http.Request) {
	voices, err := s.st.ListVoices("", 2000)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "读取失败")
		return
	}
	renders, _ := s.st.ListRenders("", 500)
	writeOK(w, map[string]any{"voices": voices, "renders": renders,
		"total": len(voices), "voiceBytes": sumSize(voices)})
}

func sumSize(vs []store.Voice) int64 {
	var n int64
	for _, v := range vs {
		n += v.Size
	}
	return n
}

var _ = strconv.Itoa
