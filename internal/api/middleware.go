package api

import (
	"context"
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"jxivideo/internal/store"
)

type ctxKey string

const (
	ctxUser  ctxKey = "user"
	ctxOwner ctxKey = "owner"
	ctxIP    ctxKey = "ip"
)

// ---------- 客户端 IP ----------

func (s *Server) clientIP(r *http.Request) string {
	if s.trustFwd {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if ip := strings.TrimSpace(parts[0]); ip != "" {
				return ip
			}
		}
		if xr := r.Header.Get("X-Real-IP"); xr != "" {
			return strings.TrimSpace(xr)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ---------- 限流 ----------

type bucket struct {
	tokens float64
	last   time.Time
}

type ipLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	lastGC  time.Time
}

func newIPLimiter() *ipLimiter {
	return &ipLimiter{buckets: make(map[string]*bucket), lastGC: time.Now()}
}

// allow 令牌桶：rate 每分钟补充数，burst 桶容量
func (l *ipLimiter) allow(key string, rate, burst int) bool {
	if rate <= 0 {
		return true
	}
	if burst <= 0 {
		burst = rate
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(burst), last: now}
		l.buckets[key] = b
	}
	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * (float64(rate) / 60.0)
	if b.tokens > float64(burst) {
		b.tokens = float64(burst)
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (l *ipLimiter) gc() {
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := time.Now().Add(-10 * time.Minute)
	for k, b := range l.buckets {
		if b.last.Before(cut) {
			delete(l.buckets, k)
		}
	}
}

// ---------- 登录防爆破 ----------

type loginGuard struct {
	mu       sync.Mutex
	fails    map[string]*failState
}

type failState struct {
	count int
	until time.Time
}

func newLoginGuard() *loginGuard { return &loginGuard{fails: make(map[string]*failState)} }

func (g *loginGuard) blocked(ip string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	st, ok := g.fails[ip]
	if !ok {
		return false
	}
	return time.Now().Before(st.until)
}

func (g *loginGuard) fail(ip string, ban time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	st, ok := g.fails[ip]
	if !ok {
		st = &failState{}
		g.fails[ip] = st
	}
	st.count++
	if st.count >= 5 {
		st.until = time.Now().Add(ban)
		st.count = 0
	}
}

func (g *loginGuard) reset(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.fails, ip)
}

func (g *loginGuard) gc() {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	for k, v := range g.fails {
		if v.until.Before(now) && v.count == 0 {
			delete(g.fails, k)
		}
	}
}

// ---------- 中间件 ----------

func (s *Server) recoverMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				// panic 必须留痕：值 + 完整堆栈，进 stdout 与轮转日志文件，
				// 否则线上只能看到一句「服务内部错误」无从排查
				log.Printf("PANIC %s %s ip=%s: %v\n%s",
					r.Method, r.URL.Path, s.clientIP(r), rec, debug.Stack())
				writeErr(w, http.StatusInternalServerError, "internal", "服务内部错误")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) securityMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), geolocation=(), payment=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		// 允许麦克风（音色工坊录音）与 blob 音频，其余收紧
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: blob: https:; media-src 'self' data: blob:; "+
				"connect-src 'self'; font-src 'self' data:; frame-ancestors 'self'; base-uri 'self'; form-action 'self'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}

		// 拦截常见扫描路径，减少无效攻击面
		p := strings.ToLower(r.URL.Path)
		for _, bad := range []string{"/wp-admin", "/wp-login", "/.env", "/.git", "/phpmyadmin",
			"/xmlrpc.php", "/vendor/", "/.aws", "/config.json", "/actuator"} {
			if strings.HasPrefix(p, bad) || strings.HasSuffix(p, ".php") {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}

		// 变更类请求校验 Origin，抵御 CSRF
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if origin := r.Header.Get("Origin"); origin != "" {
				if !sameOrigin(origin, r.Host) {
					writeErr(w, http.StatusForbidden, "csrf", "请求来源不合法")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func sameOrigin(origin, host string) bool {
	o := strings.TrimSuffix(strings.ToLower(origin), "/")
	o = strings.TrimPrefix(o, "https://")
	o = strings.TrimPrefix(o, "http://")
	return o == strings.ToLower(host)
}

type statusWriter struct {
	http.ResponseWriter
	status int
	n      int64
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	n, err := w.ResponseWriter.Write(b)
	w.n += int64(n)
	return n, err
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) accessLogMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		ms := time.Since(start).Milliseconds()

		// 每请求一行进 stdout/文件（SQLite 里另存 /v1 与 /api 的明细供后台查询）
		log.Printf("%s %s %d %dms ip=%s", r.Method, r.URL.Path, sw.status, ms, s.clientIP(r))

		if strings.HasPrefix(r.URL.Path, "/v1/") {
			s.st.AddLog(store.LogEntry{
				IP: s.clientIP(r), Method: r.Method, Path: r.URL.Path,
				Status: sw.status, Ms: ms, Kind: "gw", UA: r.UserAgent(),
			})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			kind := "api"
			if strings.HasPrefix(r.URL.Path, "/api/admin") {
				kind = "admin"
			} else if strings.HasPrefix(r.URL.Path, "/api/tts") {
				kind = "tts"
			} else if r.URL.Path == "/api/public/playground" {
				kind = "playground"
			}
			s.st.AddLog(store.LogEntry{
				IP: s.clientIP(r), Method: r.Method, Path: r.URL.Path,
				Status: sw.status, Ms: ms, Kind: kind, UA: r.UserAgent(),
			})
			if kind == "playground" && sw.status < 400 {
				s.st.IncrCounter("calls_total", 1)
				s.st.IncrCounter("calls:"+time.Now().Format("2006-01-02"), 1)
			}
		}
	})
}

// limitMW 全局限流：普通接口按配置速率，登录接口额外收紧
func (s *Server) limitMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := s.st.Settings()
		ip := s.clientIP(r)

		// 请求体大小上限（网关自行控制，避免二次包裹）
		maxBytes := int64(cfg.MaxBodyMB) << 20
		if maxBytes <= 0 {
			maxBytes = 4 << 20
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !strings.HasPrefix(r.URL.Path, "/v1/") {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		}

		path := r.URL.Path
		switch {
		case path == "/api/auth/login":
			if !s.limiter.allow("login:"+ip, 10, 6) {
				writeErr(w, http.StatusTooManyRequests, "rate_limited", "尝试过于频繁，请稍后再试")
				return
			}
		case strings.HasPrefix(path, "/api/tts/synthesize"):
			if !s.limiter.allow("tts:"+ip, 20, 6) {
				writeErr(w, http.StatusTooManyRequests, "rate_limited", "合成请求过于频繁，请稍后再试")
				return
			}
		case strings.HasPrefix(path, "/api/public/playground"):
			if !s.limiter.allow("pg:"+ip, 40, 12) {
				writeErr(w, http.StatusTooManyRequests, "rate_limited", "调试请求过于频繁")
				return
			}
		case strings.HasPrefix(path, "/v1/"):
			// 网关：按 IP 令牌桶，速率沿用全局配置
			if !s.limiter.allow("gw:"+ip, cfg.RateLimitPerMin, cfg.RateLimitBurst) {
				gwError(w, http.StatusTooManyRequests, "rate_limited", "请求过于频繁，请稍后再试")
				return
			}
		case strings.HasPrefix(path, "/api/"):
			if !s.limiter.allow("api:"+ip, cfg.RateLimitPerMin, cfg.RateLimitBurst) {
				writeErr(w, http.StatusTooManyRequests, "rate_limited", "请求过于频繁，请稍后再试")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ---------- 身份 ----------

func (s *Server) currentUser(r *http.Request) (store.User, bool) {
	if u, ok := r.Context().Value(ctxUser).(store.User); ok {
		return u, true
	}
	c, err := r.Cookie("bpk_sid")
	if err != nil {
		return store.User{}, false
	}
	u, ok := s.st.SessionUser(c.Value)
	if !ok {
		return store.User{}, false
	}
	return u, true
}

func isAdminRequest(s *Server, r *http.Request) bool {
	u, ok := s.currentUser(r)
	return ok && u.Role == "admin"
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := s.st.Settings()
		ip := s.clientIP(r)
		if len(cfg.AdminIPAllow) > 0 && !ipAllowed(ip, cfg.AdminIPAllow) {
			writeErr(w, http.StatusForbidden, "ip_denied", "当前 IP 不在后台白名单内")
			return
		}
		u, ok := s.currentUser(r)
		if !ok || u.Role != "admin" {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "请先登录管理员账号")
			return
		}
		// 使用默认密码的管理员：先改密，否则拒绝所有后台操作
		if u.MustChangePass {
			writeErr(w, http.StatusForbidden, "password_change_required", "首次登录请先修改默认密码")
			return
		}
		ctx := context.WithValue(r.Context(), ctxUser, u)
		next(w, r.WithContext(ctx))
	}
}

func ipAllowed(ip string, list []string) bool {
	host := ip
	if h, _, err := net.SplitHostPort(ip); err == nil {
		host = h
	}
	parsed := net.ParseIP(host)
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if item == host {
			return true
		}
		if _, cidr, err := net.ParseCIDR(item); err == nil && parsed != nil && cidr.Contains(parsed) {
			return true
		}
	}
	return false
}

// ownerKey 返回音色归属标识：登录用户优先，否则游客 Cookie
func (s *Server) ownerKey(w http.ResponseWriter, r *http.Request) string {
	if u, ok := s.currentUser(r); ok {
		return "u:" + itoa64(u.ID)
	}
	if c, err := r.Cookie("bpk_guest"); err == nil && len(c.Value) >= 16 {
		return "g:" + c.Value
	}
	tok := randomToken(16)
	http.SetCookie(w, &http.Cookie{
		Name:     "bpk_guest",
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 3600,
	})
	return "g:" + tok
}

// ---------- 后台任务 ----------

func (s *Server) housekeeping() {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for range t.C {
		s.limiter.gc()
		s.guard.gc()
		s.st.PurgeSessions()
		s.st.PurgeEmailCodes()
		cfg := s.st.Settings()
		s.st.PurgeOldRenders(cfg.TTS.KeepRenderDay)
		s.st.TrimLogs(20000)
	}
}
