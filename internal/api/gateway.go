package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"jxivideo/internal/store"
)

// dayCounter 按 key（IP / 用户）统计当日次数，用于游客免费额度
type dayCounter struct {
	mu sync.Mutex
	m  map[string]*dayEntry
}

type dayEntry struct {
	day string
	n   int
}

func newDayCounter() *dayCounter {
	return &dayCounter{m: map[string]*dayEntry{}}
}

// allow 尝试占用一次额度；limit<=0 表示不限制
func (d *dayCounter) allow(key string, limit int) bool {
	if limit <= 0 {
		return true
	}
	today := time.Now().Format("2006-01-02")
	d.mu.Lock()
	defer d.mu.Unlock()
	e := d.m[key]
	if e == nil || e.day != today {
		e = &dayEntry{day: today}
		d.m[key] = e
	}
	if e.n >= limit {
		return false
	}
	e.n++
	return true
}

// remaining 剩余次数
func (d *dayCounter) remaining(key string, limit int) int {
	if limit <= 0 {
		return -1
	}
	today := time.Now().Format("2006-01-02")
	d.mu.Lock()
	defer d.mu.Unlock()
	e := d.m[key]
	if e == nil || e.day != today {
		return limit
	}
	if e.n >= limit {
		return 0
	}
	return limit - e.n
}

// gc 清理过期记录
func (d *dayCounter) gc() {
	today := time.Now().Format("2006-01-02")
	d.mu.Lock()
	defer d.mu.Unlock()
	for k, e := range d.m {
		if e.day != today {
			delete(d.m, k)
		}
	}
}

// 网关：把外部请求按接口登记表转发到上游，并注入后台配置的上游密钥。
// 客户端永远拿不到上游密钥，只持有本站签发的 sk- key。

const gwMaxBody = 8 << 20

// 允许透传给上游的请求头（其余一律丢弃，避免头注入）
var passthroughHeaders = []string{
	"Content-Type", "Accept", "Anthropic-Version", "Anthropic-Beta",
	"OpenAI-Beta", "OpenAI-Organization", "Accept-Language",
}

// gwError 输出 OpenAI 兼容的错误结构，方便官方 SDK 直接解析
func gwError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"message": msg,
			"type":    "invalid_request_error",
			"code":    code,
		},
	})
}

// clientKey 从请求中提取本站签发的密钥
func clientKey(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("x-api-key")); v != "" {
		return v
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return ""
}

// handleGateway 对外网关入口，挂在 /v1/ 前缀下
func (s *Server) handleGateway(w http.ResponseWriter, r *http.Request) {
	cfg := s.st.Settings()
	start := time.Now()

	if !cfg.GatewayEnabled {
		gwError(w, http.StatusServiceUnavailable, "gateway_disabled", "网关已暂停服务，请稍后再试")
		return
	}

	path := strings.TrimRight(r.URL.Path, "/")
	if path == "" {
		path = r.URL.Path
	}

	// 1. 接口是否登记且在线
	it, err := s.st.GetInterfaceByPath(path)
	if err != nil {
		gwError(w, http.StatusNotFound, "model_not_found", "接口不存在："+path)
		return
	}
	if it.Status == "offline" {
		gwError(w, http.StatusServiceUnavailable, "interface_offline", "该接口已下线")
		return
	}
	if it.Status == "maintenance" {
		gwError(w, http.StatusServiceUnavailable, "interface_maintenance", "该接口维护中")
		return
	}
	if it.Method != "" && !strings.EqualFold(it.Method, r.Method) && r.Method != http.MethodGet {
		// 允许 GET 探测，其余严格匹配
		gwError(w, http.StatusMethodNotAllowed, "method_not_allowed", "该接口只接受 "+it.Method)
		return
	}

	// 2. 鉴权
	plain := clientKey(r)
	var key store.APIKey
	authed := false
	if plain != "" {
		if k, err := s.st.FindAPIKey(plain); err == nil {
			if k.Status != "active" {
				gwError(w, http.StatusForbidden, "key_disabled", "该 API Key 已被禁用")
				return
			}
			if k.Quota > 0 && k.UsedToday >= k.Quota {
				gwError(w, http.StatusTooManyRequests, "quota_exceeded", "今日调用额度已用完，请明天再试或提升额度")
				return
			}
			key = k
			authed = true
		} else {
			gwError(w, http.StatusUnauthorized, "invalid_api_key", "API Key 无效")
			return
		}
	}

	if !authed {
		// 未携带密钥：仅当全局允许游客 + 接口不需要登录时放行
		if cfg.RequireLoginAll || it.RequiresLogin || !cfg.AllowGuest {
			gwError(w, http.StatusUnauthorized, "login_required", "该接口需要 API Key，请在控制台创建后携带 x-api-key 请求头")
			return
		}
		// 游客按 IP 每日免费额度
		if !s.guest.allow(s.clientIP(r), cfg.GuestDailyQuota) {
			gwError(w, http.StatusTooManyRequests, "guest_quota", "游客今日免费次数已用完，登录后额度更高")
			return
		}
		w.Header().Set("X-Guest-Remaining", strconv.Itoa(s.guest.remaining(s.clientIP(r), cfg.GuestDailyQuota)))
	}

	// 2.5 本地能力：由本站直接实现、不走上游转发（如抖音作者作品分析）。
	//     鉴权 / 限流 / 游客额度与转发能力完全一致，计量也走同一张用量表。
	if h := s.localCapability(it.Slug); h != nil {
		status := h(w, r)
		if key.ID > 0 {
			s.st.TouchAPIKey(key.ID)
		}
		s.st.IncrCounter("calls_total", 1)
		s.st.IncrCounter("calls:"+time.Now().Format("2006-01-02"), 1)
		s.st.IncrCounter("gw:"+it.Slug, 1)
		s.st.RecordUsage(store.Usage{
			KeyID: key.ID, UserID: key.UserID, Username: key.Username, Slug: it.Slug,
			Path: path, Method: r.Method, Status: status,
			Ms: time.Since(start).Milliseconds(), IP: s.clientIP(r), Stream: false,
		})
		return
	}

	// 3. 组装上游地址：接口单独配置优先，否则按"去掉 /v1 前缀 + 上游前缀"推导
	upPath := it.UpstreamPath
	if upPath == "" {
		p := it.Path
		if strings.HasPrefix(p, "/v1/") {
			p = strings.TrimPrefix(p, "/v1")
		}
		upPath = strings.TrimRight(strings.TrimSpace(cfg.UpstreamPrefix), "/") + p
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.UpstreamBase), "/")
	if base == "" {
		gwError(w, http.StatusServiceUnavailable, "no_upstream", "尚未配置上游地址，请联系管理员")
		return
	}
	target := base + upPath
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		gwError(w, http.StatusBadGateway, "bad_upstream", "上游地址配置不合法")
		return
	}

	// 4. 读取请求体
	limit := int64(gwMaxBody)
	if cfg.MaxBodyMB > 0 {
		limit = int64(cfg.MaxBodyMB) << 20
	}
	var body []byte
	if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodHead {
		body, err = io.ReadAll(io.LimitReader(r.Body, limit+1))
		if err != nil {
			gwError(w, http.StatusBadRequest, "bad_body", "读取请求体失败")
			return
		}
		if int64(len(body)) > limit {
			gwError(w, http.StatusRequestEntityTooLarge, "body_too_large", "请求体超出大小限制")
			return
		}
	}

	stream := wantsStream(r, body)

	upMethod := it.UpstreamMethod
	if upMethod == "" {
		upMethod = r.Method
	}

	upReq, err := http.NewRequestWithContext(r.Context(), strings.ToUpper(upMethod), target, bytes.NewReader(body))
	if err != nil {
		gwError(w, http.StatusInternalServerError, "bad_request", "无法构造上游请求")
		return
	}
	for _, h := range passthroughHeaders {
		if v := r.Header.Get(h); v != "" {
			upReq.Header.Set(h, v)
		}
	}
	if len(body) > 0 && upReq.Header.Get("Content-Type") == "" {
		upReq.Header.Set("Content-Type", "application/json")
	}
	upReq.Header.Set("User-Agent", "ClearFrame-Gateway/1.0")

	// 注入上游密钥（后台配置，绝不下发给客户端）
	upKey := strings.TrimSpace(cfg.UpstreamKey)
	if upKey == "" {
		gwError(w, http.StatusServiceUnavailable, "upstream_key_missing", "上游密钥未配置，请联系管理员")
		return
	}
	hdr := strings.TrimSpace(cfg.UpstreamAuthHdr)
	if hdr == "" {
		hdr = "x-api-key"
	}
	val := cfg.UpstreamAuthPfx + upKey
	if strings.EqualFold(hdr, "Authorization") && cfg.UpstreamAuthPfx == "" {
		val = "Bearer " + upKey
	}
	upReq.Header.Set(hdr, val)

	client := s.gwClient(stream)
	reqHeader := upReq.Header.Clone()
	resp, err := client.Do(upReq)
	if err != nil && retryableNetErr(err) {
		// 直连不通时回退到系统代理重试一次（部分服务器只能经代理出网）
		if req2, e2 := http.NewRequestWithContext(r.Context(), upReq.Method, target, bytes.NewReader(body)); e2 == nil {
			req2.Header = reqHeader
			resp, err = s.gwClientProxy(stream).Do(req2)
		}
	}
	if err != nil {
		s.st.RecordUsage(store.Usage{
			KeyID: key.ID, UserID: key.UserID, Username: key.Username, Slug: it.Slug,
			Path: path, Method: r.Method, Status: 502, Ms: time.Since(start).Milliseconds(),
			IP: s.clientIP(r), Stream: stream,
		})
		gwError(w, http.StatusBadGateway, "upstream_unreachable", "上游请求失败："+sanitizeErr(err))
		return
	}
	defer resp.Body.Close()

	if key.ID > 0 {
		s.st.TouchAPIKey(key.ID)
	}
	s.st.IncrCounter("calls_total", 1)
	s.st.IncrCounter("calls:"+time.Now().Format("2006-01-02"), 1)
	s.st.IncrCounter("gw:"+it.Slug, 1)

	// 5. 回传
	for _, h := range []string{"Content-Type", "Cache-Control", "X-Request-Id", "X-RateLimit-Remaining"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	w.Header().Set("X-Gateway", it.Slug)

	// 5.1 相同查询的短缓存：短时间反复解析同一作品会触发上游限频，
	//     命中缓存直接回放，既省上游配额也让重复点击秒回
	ckey := ""
	if !stream && r.Method == http.MethodGet {
		ckey = it.Slug + "|" + r.URL.RawQuery
		if e, ok := gwCacheGet(ckey); ok {
			if ct := e.header.Get("Content-Type"); ct != "" {
				w.Header().Set("Content-Type", ct)
			}
			w.Header().Set("X-Cache", "HIT")
			w.WriteHeader(e.status)
			_, _ = w.Write(e.body)
			s.st.RecordUsage(store.Usage{
				KeyID: key.ID, UserID: key.UserID, Username: key.Username, Slug: it.Slug,
				Path: path, Method: r.Method, Status: e.status,
				Ms: time.Since(start).Milliseconds(), IP: s.clientIP(r), Stream: false,
			})
			return
		}
	}

	if stream {
		w.WriteHeader(resp.StatusCode)
		flusher, _ := w.(http.Flusher)
		buf := make([]byte, 4096)
		for {
			n, rerr := resp.Body.Read(buf)
			if n > 0 {
				if _, werr := w.Write(buf[:n]); werr != nil {
					break
				}
				if flusher != nil {
					flusher.Flush()
				}
			}
			if rerr != nil {
				break
			}
		}
	} else {
		raw, rerr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if rerr != nil {
			w.WriteHeader(resp.StatusCode)
			return
		}
		// 上游限频：转成 429 + 明确提示，避免用户误以为是参数错了
		if resp.StatusCode >= 400 && looksRateLimited(raw) {
			s.st.RecordUsage(store.Usage{
				KeyID: key.ID, UserID: key.UserID, Username: key.Username, Slug: it.Slug,
				Path: path, Method: r.Method, Status: http.StatusTooManyRequests,
				Ms: time.Since(start).Milliseconds(), IP: s.clientIP(r), Stream: false,
			})
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"message": "上游临时限流：同一内容短时间被查询太多次，等 1 分钟左右再点一次即可，不用改参数",
					"type":    "rate_limit_error",
					"code":    "upstream_rate_limited",
				},
			})
			return
		}
		w.Header().Set("X-Cache", "MISS")
		if ckey != "" && resp.StatusCode == 200 && gwJSONCode(raw) == 200 {
			gwCachePut(ckey, resp.StatusCode, resp.Header, raw)
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(raw)
	}

	s.st.RecordUsage(store.Usage{
		KeyID: key.ID, UserID: key.UserID, Username: key.Username, Slug: it.Slug,
		Path: path, Method: r.Method, Status: resp.StatusCode,
		Ms: time.Since(start).Milliseconds(), IP: s.clientIP(r), Stream: stream,
	})
}

// localCapability 本地能力注册表：slug → 处理函数。
// 命中后网关跳过上游转发，由本站自行实现并返回状态码供计量。
func (s *Server) localCapability(slug string) func(w http.ResponseWriter, r *http.Request) int {
	switch slug {
	case "dyzz":
		return s.handleDyzz
	default:
		return nil
	}
}

// upstreamTransport 访问上游专用：显式禁用环境代理，
// 避免服务器上残留的 http_proxy / https_proxy 把请求带偏。
var upstreamTransport = &http.Transport{
	Proxy:                 nil,
	DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	MaxIdleConns:          100,
	IdleConnTimeout:       60 * time.Second,
	TLSHandshakeTimeout:   15 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	ForceAttemptHTTP2:     true,
}

// ---------- 成功结果短缓存 ----------

const (
	gwCacheTTL = 60 * time.Second
	gwCacheMax = 512
)

type gwCacheEntry struct {
	status int
	header http.Header
	body   []byte
	exp    time.Time
}

// gwRespCache 只缓存「上游明确成功」的 GET 结果，防止短时间重复查询同一作品
// 触发上游限频，也让重复点击秒回。
var gwRespCache = struct {
	sync.Mutex
	m map[string]gwCacheEntry
}{m: map[string]gwCacheEntry{}}

func gwCacheGet(key string) (gwCacheEntry, bool) {
	gwRespCache.Lock()
	defer gwRespCache.Unlock()
	e, ok := gwRespCache.m[key]
	if !ok {
		return e, false
	}
	if time.Now().After(e.exp) {
		delete(gwRespCache.m, key)
		return gwCacheEntry{}, false
	}
	return e, true
}

func gwCachePut(key string, status int, header http.Header, body []byte) {
	gwRespCache.Lock()
	defer gwRespCache.Unlock()
	if len(gwRespCache.m) >= gwCacheMax {
		now := time.Now()
		for k, v := range gwRespCache.m {
			if now.After(v.exp) {
				delete(gwRespCache.m, k)
			}
		}
		if len(gwRespCache.m) >= gwCacheMax {
			gwRespCache.m = map[string]gwCacheEntry{}
		}
	}
	gwRespCache.m[key] = gwCacheEntry{
		status: status, header: header.Clone(),
		body: append([]byte(nil), body...), exp: time.Now().Add(gwCacheTTL),
	}
}

// gwJSONCode 从统一 JSON 3.0 里取业务码（兼容数字与字符串两种写法）
func gwJSONCode(body []byte) int {
	var v struct {
		Code json.RawMessage `json:"code"`
	}
	// 注意：要解到结构体上；解到 &v.Code 会把整个响应体当成 RawMessage 的值
	if json.Unmarshal(body, &v) != nil || len(v.Code) == 0 {
		return 0
	}
	n, err := strconv.Atoi(strings.Trim(string(v.Code), `" `))
	if err != nil {
		return 0
	}
	return n
}

// looksRateLimited 识别上游的限频文案（中英文都算）
func looksRateLimited(body []byte) bool {
	s := strings.ToLower(string(body))
	for _, k := range []string{"频繁", "频率", "限流", "稍后再试", "稍后重试", "rate limit", "too many requests"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// gwClient 流式请求用更长的读超时
func (s *Server) gwClient(stream bool) *http.Client {
	return &http.Client{Transport: gwTransport(false), Timeout: gwTimeout(stream)}
}

// gwClientProxy 直连失败时的回退：走系统环境代理（兼容 Clash TUN / fake-ip 这类网络环境）
func (s *Server) gwClientProxy(stream bool) *http.Client {
	return &http.Client{Transport: gwTransport(true), Timeout: gwTimeout(stream)}
}

func gwTimeout(stream bool) time.Duration {
	if stream {
		return 0
	}
	return 180 * time.Second
}

func gwTransport(useProxy bool) *http.Transport {
	var proxy func(*http.Request) (*url.URL, error)
	if useProxy {
		proxy = http.ProxyFromEnvironment
	}
	return &http.Transport{
		Proxy:                 proxy,
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}
}

// retryableNetErr 判断是否值得换一条链路重试（DNS / 连接 / 超时类错误）
func retryableNetErr(err error) bool {
	if err == nil {
		return false
	}
	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, k := range []string{"dial tcp", "connect", "no such host", "timeout", "deadline exceeded", "network is unreachable", "eof"} {
		if strings.Contains(msg, k) {
			return true
		}
	}
	return false
}

// wantsStream 判断客户端是否要求流式返回
func wantsStream(r *http.Request, body []byte) bool {
	if strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream") {
		return true
	}
	if len(body) == 0 {
		return false
	}
	var probe struct {
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &probe); err == nil && probe.Stream {
		return true
	}
	return false
}

// handleUpstreamProbe 后台：用当前配置的上游密钥探测连通性
func (s *Server) handleUpstreamProbe(w http.ResponseWriter, r *http.Request) {
	cfg := s.st.Settings()
	base := strings.TrimRight(strings.TrimSpace(cfg.UpstreamBase), "/")
	if base == "" {
		writeErr(w, http.StatusBadRequest, "no_upstream", "请先填写上游地址")
		return
	}
	key := strings.TrimSpace(cfg.UpstreamKey)
	if key == "" {
		writeErr(w, http.StatusBadRequest, "no_key", "请先填写上游密钥")
		return
	}
	probePath := strings.TrimSpace(r.URL.Query().Get("path"))
	if probePath == "" {
		probePath = "/api/models"
	}
	if !strings.HasPrefix(probePath, "/") {
		probePath = "/" + probePath
	}

	req, err := http.NewRequest(http.MethodGet, base+probePath, nil)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_upstream", "上游地址不合法")
		return
	}
	hdr := strings.TrimSpace(cfg.UpstreamAuthHdr)
	if hdr == "" {
		hdr = "x-api-key"
	}
	val := cfg.UpstreamAuthPfx + key
	if strings.EqualFold(hdr, "Authorization") && cfg.UpstreamAuthPfx == "" {
		val = "Bearer " + key
	}
	req.Header.Set(hdr, val)
	req.Header.Set("Accept", "application/json")

	start := time.Now()
	resp, err := (&http.Client{Transport: upstreamTransport, Timeout: 20 * time.Second}).Do(req)
	ms := time.Since(start).Milliseconds()
	if err != nil {
		writeOK(w, map[string]any{"ok": false, "ms": ms, "error": sanitizeErr(err)})
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	writeOK(w, map[string]any{
		"ok":     resp.StatusCode < 400,
		"status": resp.StatusCode,
		"ms":     ms,
		"path":   probePath,
		"body":   string(raw),
	})
}

// handleAdminSync 从上游接口市场同步接口清单
func (s *Server) handleAdminSync(w http.ResponseWriter, r *http.Request) {
	cfg := s.st.Settings()
	n, warns, err := s.st.SyncFromMarket(cfg.UpstreamBase)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "sync_failed", "同步失败："+err.Error())
		return
	}
	writeOK(w, map[string]any{"synced": n, "warnings": warns})
}

var errNoUpstream = errors.New("上游未配置")
