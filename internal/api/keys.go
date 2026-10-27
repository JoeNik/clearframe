package api

import (
	"context"
	"net/http"
	"strings"

	"jxivideo/internal/store"
)

// requireLogin 要求已登录，并把用户放进 context
func (s *Server) requireLogin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if u, ok := s.currentUser(r); ok {
			next(w, r.WithContext(context.WithValue(r.Context(), ctxUser, u)))
			return
		}
		writeErr(w, http.StatusUnauthorized, "login_required", "请先登录")
	}
}

// ---------- 用户自助 ----------

func (s *Server) handleListKeys(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	list, err := s.st.ListAPIKeys(u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "读取密钥失败")
		return
	}
	writeOK(w, map[string]any{"items": list})
}

func (s *Server) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	var req struct {
		Name  string `json:"name"`
		Quota int    `json:"quota"`
	}
	_ = readJSON(r, &req)
	if req.Quota < 0 {
		req.Quota = 0
	}
	cfg := s.st.Settings()
	if req.Quota == 0 {
		req.Quota = cfg.DefaultQuota
	}
	k, plain, err := s.st.CreateAPIKey(u.ID, req.Name, req.Quota)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "创建密钥失败")
		return
	}
	writeOK(w, map[string]any{"key": k, "plain": plain})
}

func (s *Server) handleUpdateKey(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	var req struct {
		Name   string `json:"name"`
		Quota  int    `json:"quota"`
		Status string `json:"status"`
	}
	_ = readJSON(r, &req)
	quota := req.Quota
	if quota == 0 {
		quota = -1 // 0 视为"不修改"，负数表示跳过
	}
	if err := s.st.UpdateAPIKey(pathID(r), u.ID, req.Name, quota, req.Status); err != nil {
		writeErr(w, http.StatusBadRequest, "update_failed", "更新失败："+err.Error())
		return
	}
	writeOK(w, map[string]any{"ok": true})
}

func (s *Server) handleDeleteKey(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	if err := s.st.DeleteAPIKey(pathID(r), u.ID); err != nil {
		writeErr(w, http.StatusBadRequest, "delete_failed", "删除失败")
		return
	}
	writeOK(w, map[string]any{"ok": true})
}

func (s *Server) handleMyUsage(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	list, err := s.st.ListUsage(u.ID, queryInt(r, "limit", 50))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "读取用量失败")
		return
	}
	writeOK(w, map[string]any{
		"summary": s.st.UsageStats(u.ID),
		"items":   list,
	})
}

// ---------- 管理端 ----------

func (s *Server) handleAdminKeys(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.ListAPIKeys(0)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "读取密钥失败")
		return
	}
	writeOK(w, map[string]any{"items": list})
}

func (s *Server) handleAdminUpdateKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		Quota  int    `json:"quota"`
		Status string `json:"status"`
	}
	_ = readJSON(r, &req)
	quota := req.Quota
	if quota == 0 {
		quota = -1
	}
	if err := s.st.UpdateAPIKey(pathID(r), 0, req.Name, quota, req.Status); err != nil {
		writeErr(w, http.StatusBadRequest, "update_failed", "更新失败："+err.Error())
		return
	}
	writeOK(w, map[string]any{"ok": true})
}

func (s *Server) handleAdminDeleteKey(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteAPIKey(pathID(r), 0); err != nil {
		writeErr(w, http.StatusBadRequest, "delete_failed", "删除失败")
		return
	}
	writeOK(w, map[string]any{"ok": true})
}

func (s *Server) handleAdminUsage(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.ListUsage(0, queryInt(r, "limit", 100))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "读取用量失败")
		return
	}
	writeOK(w, map[string]any{
		"summary": s.st.UsageStats(0),
		"items":   list,
	})
}

// handleAdminCreateKeyFor 管理员为指定用户签发密钥
func (s *Server) handleAdminCreateKeyFor(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID int64  `json:"userId"`
		Name   string `json:"name"`
		Quota  int    `json:"quota"`
	}
	if err := readJSON(r, &req); err != nil || req.UserID <= 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "缺少 userId")
		return
	}
	k, plain, err := s.st.CreateAPIKey(req.UserID, req.Name, req.Quota)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "create_failed", "创建失败："+err.Error())
		return
	}
	writeOK(w, map[string]any{"key": k, "plain": plain})
}

// handleGatewayInfo 给前台/调试台用的网关接入信息（不含任何上游密钥）
func (s *Server) handleGatewayInfo(w http.ResponseWriter, r *http.Request) {
	cfg := s.st.Settings()
	base := strings.TrimSpace(cfg.PublicBase)
	if base == "" {
		base = "http://" + r.Host
	}
	base = strings.TrimRight(base, "/")
	u, logged := s.currentUser(r)
	out := map[string]any{
		"baseUrl":  base,
		"enabled":  cfg.GatewayEnabled,
		"loggedIn": logged,
		"endpoints": []string{
			"/v1/chat/completions", "/v1/embeddings", "/v1/images/generations",
			"/v1/audio/speech", "/v1/audio/transcriptions", "/v1/models",
		},
	}
	if logged {
		out["user"] = map[string]any{"id": u.ID, "username": u.Username}
	}
	writeOK(w, out)
}

var _ = store.Usage{}
