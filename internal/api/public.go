package api

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"jxivideo/internal/auth"
	"jxivideo/internal/store"
)

func randomToken(n int) string { return auth.RandomToken(n) }

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }

// ---------- 启动数据 ----------

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	cfg := s.st.Settings()
	cats, _ := s.st.Categories()
	stats := s.st.Stats()
	if !cfg.ShowStats {
		stats.Calls, stats.Today = 0, 0
	}

	var me any
	if u, ok := s.currentUser(r); ok {
		me = map[string]any{"id": u.ID, "username": u.Username, "role": u.Role, "createdAt": u.CreatedAt,
			"mustChangePass": u.MustChangePass}
	}

	writeOK(w, map[string]any{
		"site": map[string]any{
			"name":          cfg.SiteName,
			"tagline":       cfg.SiteTagline,
			"desc":          cfg.SiteDesc,
			"logoText":      cfg.LogoText,
			"icp":           cfg.ICP,
			"footerText":    cfg.FooterText,
			"announcement":  cfg.Announcement,
			"showAnnounce":  cfg.ShowAnnounce,
			"allowGuest":    cfg.AllowGuest,
			"registerOpen":  cfg.RegisterOpen,
			"maintenance":   cfg.Maintenance,
			"publicBase":    s.publicBase(r),
			"gateway":       cfg.GatewayEnabled,
			"requireLogin":  cfg.RequireLoginAll,
			"showPlayground": cfg.AllowPlayground,
			"showStats":     cfg.ShowStats,
		},
		"stats":      stats,
		"categories": cats,
		"ads":        s.st.PublicAds(),
		"me":         me,
		"tts":        map[string]any{"enabled": cfg.TTS.Enabled},
	})
}

func (s *Server) handlePublicInterfaces(w http.ResponseWriter, r *http.Request) {
	cfg := s.st.Settings()
	if !cfg.AllowGuest {
		if _, ok := s.currentUser(r); !ok {
			writeErr(w, http.StatusUnauthorized, "login_required", "请先登录后查看接口列表")
			return
		}
	}
	limit := queryInt(r, "limit", 200)
	offset := queryInt(r, "offset", 0)
	list, total, err := s.st.ListInterfaces("online", r.URL.Query().Get("category"), r.URL.Query().Get("q"), limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "读取接口列表失败")
		return
	}
	writeOK(w, map[string]any{"items": list, "total": total})
}

func (s *Server) handlePublicInterface(w http.ResponseWriter, r *http.Request) {
	cfg := s.st.Settings()
	if !cfg.AllowGuest {
		if _, ok := s.currentUser(r); !ok {
			writeErr(w, http.StatusUnauthorized, "login_required", "请先登录后查看接口详情")
			return
		}
	}
	it, err := s.st.GetInterface(r.PathValue("slug"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	if it.Status != "online" {
		if u, ok := s.currentUser(r); !ok || u.Role != "admin" {
			writeErr(w, http.StatusNotFound, "not_found", "接口不存在或已下线")
			return
		}
	}
	writeOK(w, map[string]any{
		"interface":   it,
		"upstreamBase": cfg.UpstreamBase,
		"publicBase":  s.publicBase(r),
		"allowPlay":   cfg.AllowPlayground,
	})
}

// publicBase 本站对外地址，用于生成示例代码里的 base_url
func (s *Server) publicBase(r *http.Request) string {
	cfg := s.st.Settings()
	if b := strings.TrimSpace(cfg.PublicBase); b != "" {
		return strings.TrimRight(b, "/")
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// ---------- 在线调试 ----------

type playgroundReq struct {
	Slug    string            `json:"slug"`
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	APIKey  string            `json:"apiKey"`
}

func (s *Server) handlePlayground(w http.ResponseWriter, r *http.Request) {
	cfg := s.st.Settings()
	if !cfg.AllowPlayground {
		writeErr(w, http.StatusForbidden, "disabled", "在线调试已被管理员关闭")
		return
	}
	if !cfg.AllowGuest {
		if _, ok := s.currentUser(r); !ok {
			writeErr(w, http.StatusUnauthorized, "login_required", "请先登录后使用在线调试")
			return
		}
	}

	var req playgroundReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式有误")
		return
	}

	it, err := s.st.GetInterface(req.Slug)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	if it.Status != "online" {
		writeErr(w, http.StatusForbidden, "offline", "该接口当前未上线")
		return
	}
	if (it.RequiresLogin || cfg.RequireLoginAll) && func() bool { _, ok := s.currentUser(r); return !ok }() {
		writeErr(w, http.StatusUnauthorized, "login_required", "该接口需要登录后调用")
		return
	}

	// 只允许调用已登记的接口路径，避免成为开放代理
	target := it.Path
	if req.Path != "" && strings.HasPrefix(req.Path, it.Path) {
		target = req.Path
	}
	base := strings.TrimRight(cfg.UpstreamBase, "/")
	if base == "" {
		writeErr(w, http.StatusBadRequest, "no_upstream", "尚未配置上游网关地址")
		return
	}
	full := base + target
	if u, err := url.Parse(full); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		writeErr(w, http.StatusBadRequest, "bad_upstream", "上游地址不合法")
		return
	}

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = it.Method
	}
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
	default:
		writeErr(w, http.StatusBadRequest, "bad_method", "不支持的请求方法")
		return
	}

	// 只保留少量安全的自定义头
	safeHeaders := map[string]bool{
		"content-type": true, "accept": true, "x-api-key": true,
		"openai-beta": true, "anthropic-version": true,
	}
	body := strings.TrimSpace(req.Body)
	if body != "" && method == "GET" {
		body = ""
	}

	upReq, err := http.NewRequest(method, full, bytes.NewBufferString(body))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "无法构造上游请求")
		return
	}
	upReq.Header.Set("User-Agent", "BugPk-Playground/1.0")
	if body != "" {
		upReq.Header.Set("Content-Type", "application/json")
	}
	for k, v := range req.Headers {
		lk := strings.ToLower(strings.TrimSpace(k))
		if safeHeaders[lk] && v != "" {
			upReq.Header.Set(k, v)
		}
	}

	key := strings.TrimSpace(req.APIKey)
	if key == "" {
		key = cfg.PlaygroundKey
	}
	if key != "" {
		upReq.Header.Set("Authorization", "Bearer "+key)
	}

	client := &http.Client{Transport: upstreamTransport, Timeout: 75 * time.Second}
	start := time.Now()
	resp, err := client.Do(upReq)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		writeOK(w, map[string]any{
			"ok": false, "status": 0, "ms": elapsed,
			"error": "上游请求失败：" + sanitizeErr(err),
		})
		return
	}
	defer resp.Body.Close()

	const maxResp = 2 << 20
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxResp+1))
	truncated := false
	if len(raw) > maxResp {
		raw = raw[:maxResp]
		truncated = true
	}

	headers := map[string]string{}
	for _, k := range []string{"Content-Type", "X-Request-Id", "X-RateLimit-Remaining"} {
		if v := resp.Header.Get(k); v != "" {
			headers[k] = v
		}
	}

	writeOK(w, map[string]any{
		"ok":        resp.StatusCode < 400,
		"status":    resp.StatusCode,
		"ms":        elapsed,
		"headers":   headers,
		"body":      string(raw),
		"truncated": truncated,
		"size":      len(raw),
	})
}

// sanitizeErr 把底层网络错误转成对用户可读的提示，避免把内网地址、IP 直接抛到前台
func sanitizeErr(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded"):
		return "上游响应超时，请稍后重试"
	case strings.Contains(msg, "no such host"):
		return "上游域名解析失败，请检查服务器网络"
	case strings.Contains(msg, "connection refused"):
		return "上游拒绝连接，请稍后重试"
	case strings.Contains(msg, "no route to host") || strings.Contains(msg, "network is unreachable"):
		return "服务器网络不可达，请检查出网设置"
	case strings.Contains(msg, "certificate"):
		return "上游证书校验失败，请检查服务器时间与证书链"
	case strings.Contains(msg, "eof"):
		return "上游提前断开连接，请重试"
	}
	out := err.Error()
	if len(out) > 200 {
		out = out[:200] + "…"
	}
	return out
}

// ---------- 账号 ----------

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := s.clientIP(r)
	cfg := s.st.Settings()
	ban := time.Duration(cfg.BanMinutes) * time.Minute
	if ban <= 0 {
		ban = 15 * time.Minute
	}
	if s.guard.blocked(ip) {
		writeErr(w, http.StatusTooManyRequests, "locked", "失败次数过多，请稍后再试")
		return
	}

	var req loginReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式有误")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "请输入用户名与密码")
		return
	}

	u, hash, err := s.st.GetUserByName(req.Username)
	if err != nil || !auth.VerifyPassword(req.Password, hash) {
		s.guard.fail(ip, ban)
		writeErr(w, http.StatusUnauthorized, "invalid", "用户名或密码错误")
		return
	}
	if u.Status != "active" {
		writeErr(w, http.StatusForbidden, "disabled", "账号已被停用")
		return
	}

	s.guard.reset(ip)
	token, err := s.st.NewSession(u.ID, ip, r.UserAgent(), 7*24*time.Hour)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "session", "登录状态创建失败")
		return
	}
	s.st.TouchLogin(u.ID)
	http.SetCookie(w, &http.Cookie{
		Name:     "bpk_sid",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		MaxAge:   7 * 24 * 3600,
	})
	writeOK(w, map[string]any{"id": u.ID, "username": u.Username, "role": u.Role, "mustChangePass": u.MustChangePass})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("bpk_sid"); err == nil {
		s.st.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "bpk_sid", Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	writeOK(w, map[string]any{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(r)
	if !ok {
		writeOK(w, map[string]any{"user": nil})
		return
	}
	writeOK(w, map[string]any{"user": map[string]any{
		"id": u.ID, "username": u.Username, "role": u.Role, "createdAt": u.CreatedAt,
		"mustChangePass": u.MustChangePass,
	}})
}

type registerReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	cfg := s.st.Settings()
	if !cfg.RegisterOpen {
		writeErr(w, http.StatusForbidden, "closed", "当前未开放注册")
		return
	}
	var req registerReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式有误")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if len(req.Username) < 3 || len(req.Username) > 32 {
		writeErr(w, http.StatusBadRequest, "bad_username", "用户名长度需为 3 ~ 32 个字符")
		return
	}
	for _, c := range req.Username {
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-'
		if !ok {
			writeErr(w, http.StatusBadRequest, "bad_username", "用户名只能包含字母、数字、下划线与短横线")
			return
		}
	}
	if len(req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "weak_password", "密码至少 8 位")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash", "密码处理失败")
		return
	}
	u, err := s.st.CreateUser(req.Username, hash, "user", "自助注册")
	if err != nil {
		writeErr(w, http.StatusConflict, "exists", "该用户名已被占用")
		return
	}
	writeOK(w, map[string]any{"id": u.ID, "username": u.Username})
}

type passwordReq struct {
	Old string `json:"old"`
	New string `json:"new"`
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "请先登录")
		return
	}
	var req passwordReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式有误")
		return
	}
	if len(req.New) < 8 {
		writeErr(w, http.StatusBadRequest, "weak_password", "新密码至少 8 位")
		return
	}
	_, hash, err := s.st.GetUserByName(u.Username)
	if err != nil || !auth.VerifyPassword(req.Old, hash) {
		writeErr(w, http.StatusForbidden, "invalid", "原密码不正确")
		return
	}
	newHash, err := auth.HashPassword(req.New)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash", "密码处理失败")
		return
	}
	if err := s.st.UpdateUser(u.ID, newHash, "", "", ""); err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "保存失败")
		return
	}
	// 修改密码成功后清除强制改密标记
	_ = s.st.SetMustChangePass(u.ID, false)
	writeOK(w, map[string]any{"ok": true})
}

var _ = store.ErrNotFound
