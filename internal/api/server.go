package api

import (
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"jxivideo/internal/store"
)

// Server 聚合依赖
type Server struct {
	st        *store.Store
	web       fs.FS
	limiter   *ipLimiter
	guard     *loginGuard
	guest     *dayCounter
	trustFwd  bool
	publicDir string
	mux       *http.ServeMux
}

// New 构造服务
func New(st *store.Store, assets embed.FS) *Server {
	sub, err := fs.Sub(assets, "web")
	if err != nil {
		log.Fatalf("加载静态资源失败: %v", err)
	}
	s := &Server{
		st:       st,
		web:      sub,
		limiter:  newIPLimiter(),
		guard:    newLoginGuard(),
		guest:    newDayCounter(),
		trustFwd: os.Getenv("TRUST_PROXY") == "1",
	}
	s.routes()
	go s.housekeeping()
	return s
}

// Handler 返回根 handler
func (s *Server) Handler() http.Handler {
	return s.recoverMW(s.securityMW(s.accessLogMW(s.limitMW(s.mux))))
}

func (s *Server) routes() {
	m := http.NewServeMux()
	s.mux = m

	// ---- 页面 ----
	m.HandleFunc("GET /{$}", s.page("index.html"))
	m.HandleFunc("GET /cap/{slug}", s.page("cap.html"))
	m.HandleFunc("GET /interface/{slug}", s.redirectTo("/cap/{slug}"))
	m.HandleFunc("GET /console", s.page("console.html"))
	m.HandleFunc("GET /authors", s.page("authors.html"))
	m.HandleFunc("GET /tts", s.page("tts.html"))
	m.HandleFunc("GET /admin", s.page("admin.html"))
	m.HandleFunc("GET /admin/{rest...}", s.page("admin.html"))

	// ---- 对外 API 网关（/v1/* 透传到上游）----
	m.HandleFunc("/v1/", s.handleGateway)

	// ---- 公共 API ----
	m.HandleFunc("GET /api/public/bootstrap", s.handleBootstrap)
	m.HandleFunc("GET /api/public/interfaces", s.handlePublicInterfaces)
	m.HandleFunc("GET /api/public/interfaces/{slug}", s.handlePublicInterface)
	m.HandleFunc("POST /api/public/playground", s.handlePlayground)
	m.HandleFunc("GET /api/public/health", s.handleHealth)

	// ---- 账号 ----
	m.HandleFunc("POST /api/auth/login", s.handleLogin)
	m.HandleFunc("POST /api/auth/logout", s.handleLogout)
	m.HandleFunc("GET /api/auth/me", s.handleMe)
	m.HandleFunc("POST /api/auth/register", s.handleRegister)
	m.HandleFunc("POST /api/auth/password", s.handleChangePassword)
	m.HandleFunc("POST /api/auth/forgot-code", s.handleForgotCode)
	m.HandleFunc("POST /api/auth/reset-password", s.handleResetPassword)
	m.HandleFunc("POST /api/account/email-code", s.requireLogin(s.handleAccountEmailCode))
	m.HandleFunc("POST /api/account/bind-email", s.requireLogin(s.handleBindEmail))
	m.HandleFunc("POST /api/admin/email-test", s.requireAdmin(s.handleAdminEmailTest))

	// ---- 用户控制台：密钥与用量 ----
	login := func(h http.HandlerFunc) http.HandlerFunc { return s.requireLogin(h) }
	m.HandleFunc("GET /api/gateway/info", s.handleGatewayInfo)
	m.HandleFunc("GET /api/keys", login(s.handleListKeys))
	m.HandleFunc("POST /api/keys", login(s.handleCreateKey))
	m.HandleFunc("PATCH /api/keys/{id}", login(s.handleUpdateKey))
	m.HandleFunc("DELETE /api/keys/{id}", login(s.handleDeleteKey))
	m.HandleFunc("GET /api/usage", login(s.handleMyUsage))

	// ---- 我的作者（抖音关注与作品快照，登录用户）----
	m.HandleFunc("GET /api/dy/authors", s.requireLogin(s.handleDyAuthorList))
	m.HandleFunc("POST /api/dy/authors", s.requireLogin(s.handleDyAuthorAdd))
	m.HandleFunc("DELETE /api/dy/authors/{id}", s.requireLogin(s.handleDyAuthorDelete))
	m.HandleFunc("POST /api/dy/authors/{id}/refresh", s.requireLogin(s.handleDyAuthorRefresh))
	m.HandleFunc("GET /api/dy/authors/{id}/videos", s.requireLogin(s.handleDyAuthorVideos))
	m.HandleFunc("POST /api/dy/authors/{id}/stats", s.requireLogin(s.handleDyAuthorStats))

	// ---- 音色工坊 ----
	m.HandleFunc("GET /api/tts/config", s.handleTTSConfig)
	m.HandleFunc("GET /api/tts/voices", s.handleListVoices)
	m.HandleFunc("POST /api/tts/voices", s.handleCreateVoice)
	m.HandleFunc("PATCH /api/tts/voices/{id}", s.handleRenameVoice)
	m.HandleFunc("GET /api/tts/voices/{id}/audio", s.handleVoiceAudio)
	m.HandleFunc("POST /api/tts/voices/delete", s.handleDeleteVoices)
	m.HandleFunc("POST /api/tts/voices/clear", s.handleClearVoices)
	m.HandleFunc("GET /api/tts/voices/download", s.handleDownloadVoices)
	m.HandleFunc("POST /api/tts/synthesize", s.handleSynthesize)
	m.HandleFunc("GET /api/tts/renders", s.handleListRenders)
	m.HandleFunc("GET /api/tts/renders/{id}/audio", s.handleRenderAudio)
	m.HandleFunc("POST /api/tts/renders/delete", s.handleDeleteRenders)
	m.HandleFunc("POST /api/tts/renders/clear", s.handleClearRenders)
	m.HandleFunc("GET /api/tts/renders/download", s.handleDownloadRenders)

	// ---- 管理端 ----
	adm := func(h http.HandlerFunc) http.HandlerFunc { return s.requireAdmin(h) }
	m.HandleFunc("GET /api/admin/overview", adm(s.handleOverview))
	m.HandleFunc("GET /api/admin/interfaces", adm(s.handleAdminInterfaces))
	m.HandleFunc("POST /api/admin/interfaces", adm(s.handleAdminSaveInterface))
	m.HandleFunc("PUT /api/admin/interfaces/{id}", adm(s.handleAdminUpdateInterface))
	m.HandleFunc("PATCH /api/admin/interfaces/{id}", adm(s.handleAdminPatchInterface))
	m.HandleFunc("DELETE /api/admin/interfaces/{id}", adm(s.handleAdminDeleteInterface))
	m.HandleFunc("GET /api/admin/categories", adm(s.handleAdminCategories))
	m.HandleFunc("POST /api/admin/categories", adm(s.handleAdminSaveCategory))
	m.HandleFunc("DELETE /api/admin/categories/{slug}", adm(s.handleAdminDeleteCategory))
	m.HandleFunc("GET /api/admin/settings", adm(s.handleAdminGetSettings))
	m.HandleFunc("PUT /api/admin/settings", adm(s.handleAdminSaveSettings))
	m.HandleFunc("GET /api/admin/ads", adm(s.handleAdminAds))
	m.HandleFunc("POST /api/admin/ads", adm(s.handleAdminSaveAd))
	m.HandleFunc("GET /api/admin/users", adm(s.handleAdminUsers))
	m.HandleFunc("POST /api/admin/users", adm(s.handleAdminCreateUser))
	m.HandleFunc("PUT /api/admin/users/{id}", adm(s.handleAdminUpdateUser))
	m.HandleFunc("DELETE /api/admin/users/{id}", adm(s.handleAdminDeleteUser))
	m.HandleFunc("GET /api/admin/logs", adm(s.handleAdminLogs))
	m.HandleFunc("GET /api/admin/export", adm(s.handleAdminExport))
	m.HandleFunc("POST /api/admin/import", adm(s.handleAdminImport))
	m.HandleFunc("GET /api/admin/voices", adm(s.handleAdminVoices))
	m.HandleFunc("GET /api/admin/keys", adm(s.handleAdminKeys))
	m.HandleFunc("POST /api/admin/keys", adm(s.handleAdminCreateKeyFor))
	m.HandleFunc("PATCH /api/admin/keys/{id}", adm(s.handleAdminUpdateKey))
	m.HandleFunc("DELETE /api/admin/keys/{id}", adm(s.handleAdminDeleteKey))
	m.HandleFunc("GET /api/admin/usage", adm(s.handleAdminUsage))
	m.HandleFunc("GET /api/admin/upstream/probe", adm(s.handleUpstreamProbe))
	m.HandleFunc("POST /api/admin/sync", adm(s.handleAdminSync))

	// ---- 静态资源 ----
	m.Handle("GET /assets/", http.StripPrefix("/assets/", noDir(http.FileServer(http.FS(mustSub(s.web, "assets"))))))
	m.HandleFunc("GET /favicon.ico", s.handleFavicon)
	m.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, "User-agent: *\nDisallow: /admin\nDisallow: /api/admin\n")
	})

	// 兜底 404
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeErr(w, http.StatusNotFound, "not_found", "接口不存在")
			return
		}
		s.page("404.html")(w, r)
	})
}

func mustSub(f fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		log.Fatalf("静态子目录缺失: %v", err)
	}
	return sub
}

// noDir 禁止目录列表
func noDir(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") || r.URL.Path == "" {
			http.NotFound(w, r)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func (s *Server) page(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := s.st.Settings()
		if cfg.Maintenance && !strings.HasPrefix(r.URL.Path, "/admin") && !isAdminRequest(s, r) {
			s.renderMaintenance(w, cfg)
			return
		}
		data, err := fs.ReadFile(s.web, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(data)
	}
}

// redirectTo 生成 302 跳转，用于兼容旧链接（如 /interface/{slug} → /cap/{slug}）
func (s *Server) redirectTo(pattern string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target := pattern
		if strings.Contains(pattern, "{slug}") {
			target = strings.ReplaceAll(pattern, "{slug}", url.PathEscape(r.PathValue("slug")))
		}
		http.Redirect(w, r, target, http.StatusFound)
	}
}

func (s *Server) renderMaintenance(w http.ResponseWriter, cfg store.Settings) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	io.WriteString(w, `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>维护中 · `+esc(cfg.SiteName)+`</title>
<style>body{margin:0;min-height:100vh;display:grid;place-items:center;background:#0a0b0e;color:#e8eaed;
font-family:system-ui,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif}
.card{max-width:460px;padding:40px;text-align:center}
.dot{width:8px;height:8px;border-radius:50%;background:#c6f24e;display:inline-block;margin-right:8px;
box-shadow:0 0 12px #c6f24e}
h1{font-size:22px;font-weight:600;margin:18px 0 10px}p{color:#8b929c;line-height:1.7;font-size:14px}
code{background:#15171c;padding:2px 6px;border-radius:4px;font-size:13px}</style></head><body><div class="card">
<div><span class="dot"></span><span style="color:#8b929c;font-size:13px">`+esc(cfg.SiteName)+`</span></div>
<h1>服务维护中</h1><p>`+esc(cfg.MaintenanceMsg)+`</p>
<p style="margin-top:24px;font-size:12px">管理员入口 <code>/admin</code></p></div></body></html>`)
}

func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	io.WriteString(w, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="8" fill="#0f1115"/><path d="M9 22V10h5.2c2.4 0 4 1.3 4 3.4 0 1.5-.8 2.5-2.1 3 1.7.4 2.8 1.6 2.8 3.4 0 2.4-1.8 3.9-4.5 3.9H9Zm2.9-7.1h2c1 0 1.6-.5 1.6-1.3s-.6-1.3-1.6-1.3h-2v2.6Zm0 4.8h2.2c1.1 0 1.8-.5 1.8-1.5s-.7-1.5-1.8-1.5h-2.2v3Z" fill="#c6f24e"/><circle cx="23" cy="20" r="2.4" fill="#c6f24e"/></svg>`)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ts": time.Now().Unix()})
}

// ---------- 响应助手 ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeOK(w http.ResponseWriter, v any) { writeJSON(w, http.StatusOK, v) }

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": code, "message": msg})
}

func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 8<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	return nil
}

func queryInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func pathID(r *http.Request) int64 {
	n, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return n
}

var errBadRequest = errors.New("请求参数有误")

func esc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;")
	return r.Replace(s)
}
