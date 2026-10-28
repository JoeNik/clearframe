package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"jxivideo/internal/store"
)

// dyzz：抖音作者作品分析（本地能力：网关只做鉴权与计量，不走上游转发）。
//
// 数据链路：
//   主页链接 / 分享文案 / 纯 sec_uid → 302 解析出 sec_uid
//     → 旁挂的抖音数据服务（开源项目 Douyin_TikTok_Download_API）
//     → 逐页拉取作者公开作品（每条自带点赞 / 评论 / 分享 / 收藏）
//     → 聚合为统一 JSON，前端 render.js 的 author 分支渲染表格与汇总。
//
// 兼容两种旁挂形态（首次调用自动探测，本次请求内锁定）：
//   v4 单容器：GET {base}/douyin/web/fetch_user_post_videos?sec_user_id=&max_cursor=&count=
//   v5 全家桶：GET {base}/api/v1/douyin/user/posts?sec_user_id=&cursor=&count=
//
// 数据口径：抖音 web 通道不给真实播放量（恒为 0 占位），列表里只有
// 点赞/评论/分享/收藏；单作品的播放量与逐日趋势请走已有的 dytj 能力。

const (
	dyPageCount  = 20   // 每页条数（上游默认 20，稳定）
	dyMaxPages   = 12   // 最多翻页数，防止失控
	dyTimeBudget = 100 * time.Second
	dyMobileUA   = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"
)

var (
	dySecUIDRe   = regexp.MustCompile(`MS4wLjABAAAA[A-Za-z0-9_-]{20,}`)
	dyUserPathRe = regexp.MustCompile(`/user/([A-Za-z0-9_-]{20,})`)
	dyURLInText  = regexp.MustCompile(`https?://[^\s"'，。、）)】]+`)
)

// ---------- 宽松取值：上游字段类型不稳定，一律按宽松类型处理 ----------

func dyM(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func dyA(v any) []any {
	a, _ := v.([]any)
	return a
}

func dyS(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if m == nil {
			break
		}
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			if s := strings.TrimSpace(t); s != "" && s != "null" {
				return s
			}
		case json.Number:
			return t.String()
		}
	}
	return ""
}

func dyI(m map[string]any, keys ...string) (int64, bool) {
	for _, k := range keys {
		if m == nil {
			break
		}
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case json.Number:
			if n, err := t.Int64(); err == nil {
				return n, true
			}
		case int64:
			return t, true
		case int:
			return int64(t), true
		case float64:
			return int64(t), true
		case string:
			if n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64); err == nil {
				return n, true
			}
		case bool:
			if t {
				return 1, true
			}
			return 0, true
		}
	}
	return 0, false
}

func dyI0(m map[string]any, keys ...string) int64 {
	n, _ := dyI(m, keys...)
	return n
}

func dyB(m map[string]any, keys ...string) bool {
	n, ok := dyI(m, keys...)
	return ok && n != 0
}

// dyF 取比率类字段（0~1 小数），缺失返回 false
func dyF(m map[string]any, keys ...string) (float64, bool) {
	for _, k := range keys {
		if m == nil {
			break
		}
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case json.Number:
			if f, err := t.Float64(); err == nil {
				return f, true
			}
		case float64:
			return t, true
		case int64:
			return float64(t), true
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(t), 64); err == nil {
				return f, true
			}
		}
	}
	return 0, false
}

// dyFirstURL 从 {url_list:[...]} / {url:...} 这类结构里取第一个可用地址
func dyFirstURL(maps ...map[string]any) string {
	for _, m := range maps {
		if m == nil {
			continue
		}
		if s := dyS(m, "url"); s != "" {
			return s
		}
		if lst := dyA(m["url_list"]); len(lst) > 0 {
			if s, ok := lst[0].(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// dyTitle 抖音只有 desc 一个文本字段，标题取第一行非空内容
func dyTitle(desc string) string {
	for _, line := range strings.Split(desc, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			return s
		}
	}
	if desc == "" {
		return "未命名作品"
	}
	return desc
}

// ---------- sec_uid 解析 ----------

func dyExtractUserPath(u string) string {
	if m := dyUserPathRe.FindStringSubmatch(u); m != nil && dySecUIDRe.MatchString(m[1]) {
		return m[1]
	}
	return ""
}

func dyVideoish(u string) bool {
	return strings.Contains(u, "/video/") || strings.Contains(u, "/note/") ||
		strings.Contains(u, "/share/slides") || strings.Contains(u, "modal_id=")
}

// dyResolveSecUID 把用户输入归一成 sec_uid。
// 接受：纯 sec_uid、www.douyin.com/user/{sec_uid}、v.douyin.com 短链（含分享文案）。
func dyResolveSecUID(input string) (string, error) {
	cur := strings.TrimSpace(input)
	if cur == "" {
		return "", fmt.Errorf("请粘贴作者主页链接")
	}
	if m := dyURLInText.FindString(cur); m != "" {
		cur = m
	}
	// 纯 sec_uid（不含路径分隔符，避免把视频链接里的 did 设备号误认成作者）
	if !strings.Contains(cur, "/") && dySecUIDRe.MatchString(cur) {
		return dySecUIDRe.FindString(cur), nil
	}
	if sec := dyExtractUserPath(cur); sec != "" {
		return sec, nil
	}

	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: upstreamTransport,
		// 不自动跟随：手动逐跳检查 Location，够用且不会被挑战页拖死
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	for hop := 0; hop < 4; hop++ {
		req, err := http.NewRequest(http.MethodGet, cur, nil)
		if err != nil {
			return "", fmt.Errorf("链接格式不正确")
		}
		req.Header.Set("User-Agent", dyMobileUA)
		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("解析分享链接失败：%v", err)
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		resp.Body.Close()

		loc := resp.Header.Get("Location")
		final := ""
		if resp.Request != nil && resp.Request.URL != nil {
			final = resp.Request.URL.String()
		}
		if c := resp.StatusCode; c >= 300 && c < 400 && loc != "" {
			base, _ := url.Parse(cur)
			next := loc
			if base != nil {
				if abs, aerr := base.Parse(loc); aerr == nil {
					next = abs.String()
				}
			}
			if sec := dyExtractUserPath(next); sec != "" {
				return sec, nil
			}
			if dyVideoish(next) {
				return "", fmt.Errorf("这是作品链接，不是作者主页链接；请在抖音 App 进入作者主页 → 分享 → 复制主页链接")
			}
			cur = next
			continue
		}
		if sec := dyExtractUserPath(final); sec != "" {
			return sec, nil
		}
		break
	}
	return "", fmt.Errorf("链接里没有找到作者标识（sec_uid），请确认复制的是「作者主页」的分享链接")
}

// ---------- 旁挂服务客户端 ----------

type dySidecar struct {
	base   string
	key    string
	mode   string // "" 未探测；"v4" / "v5" 探测后锁定
	client *http.Client
}

func (s *Server) dySidecar() (*dySidecar, error) {
	cfg := s.st.Settings()
	base := strings.TrimSpace(cfg.DouyinSidecar)
	if base == "" {
		base = strings.TrimSpace(os.Getenv("DOUYIN_SIDECAR"))
	}
	if base == "" {
		return nil, fmt.Errorf("抖音数据服务未配置：docker compose 部署会在环境变量 DOUYIN_SIDECAR 里自动带上；手动部署请在后台「全局设置 → 上游接入」填写旁挂服务地址")
	}
	key := strings.TrimSpace(cfg.DouyinSidecarKey)
	if key == "" {
		key = strings.TrimSpace(os.Getenv("DOUYIN_SIDECAR_KEY"))
	}
	return &dySidecar{
		base:   strings.TrimRight(base, "/"),
		key:    key,
		client: &http.Client{Transport: upstreamTransport, Timeout: 30 * time.Second},
	}, nil
}

func (sc *dySidecar) getJSON(ctx context.Context, path string, q url.Values) (map[string]any, int, error) {
	u := sc.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ClearFrame-Dyzz/1.0")
	if sc.key != "" {
		req.Header.Set("Authorization", "Bearer "+sc.key)
		req.Header.Set("X-API-Key", sc.key)
	}
	resp, err := sc.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	var m map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // aweme_id 是 19 位数字，绝不能过 float64
	if err := dec.Decode(&m); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("响应不是 JSON（HTTP %d）", resp.StatusCode)
	}
	return m, resp.StatusCode, nil
}

type dyPage struct {
	items   []any
	next    string
	hasMore bool
}

// postsPage 取一页作者作品：优先 v4 单容器形态，不通自动落到 v5
func (sc *dySidecar) postsPage(ctx context.Context, secUID, cursor string) (*dyPage, error) {
	var err4 error
	if sc.mode != "v5" {
		var p *dyPage
		p, err4 = sc.postsPageV4(ctx, secUID, cursor)
		if err4 == nil {
			sc.mode = "v4"
			return p, nil
		}
		if sc.mode == "v4" {
			return nil, err4
		}
	}
	p, err5 := sc.postsPageV5(ctx, secUID, cursor)
	if err5 != nil {
		if sc.mode == "" && err4 != nil {
			return nil, fmt.Errorf("v4：%v；v5：%v", err4, err5)
		}
		return nil, err5
	}
	sc.mode = "v5"
	return p, nil
}

func (sc *dySidecar) postsPageV4(ctx context.Context, secUID, cursor string) (*dyPage, error) {
	q := url.Values{}
	q.Set("sec_user_id", secUID)
	q.Set("count", strconv.Itoa(dyPageCount))
	if cursor == "" {
		cursor = "0"
	}
	q.Set("max_cursor", cursor)
	m, status, err := sc.getJSON(ctx, "/douyin/web/fetch_user_post_videos", q)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("HTTP %d", status)
	}
	data := dyM(m["data"])
	items := dyA(data["aweme_list"])
	if data == nil || items == nil {
		if msg := dyS(m, "msg"); msg != "" {
			return nil, fmt.Errorf("%s", msg)
		}
		return nil, fmt.Errorf("响应缺少 aweme_list（HTTP %d）", status)
	}
	hasMore := dyB(data, "has_more")
	next := ""
	if hasMore {
		if n, ok := dyI(data, "max_cursor"); ok {
			next = strconv.FormatInt(n, 10)
		}
	}
	return &dyPage{items: items, next: next, hasMore: hasMore}, nil
}

func (sc *dySidecar) postsPageV5(ctx context.Context, secUID, cursor string) (*dyPage, error) {
	q := url.Values{}
	q.Set("sec_user_id", secUID)
	q.Set("count", strconv.Itoa(dyPageCount))
	q.Set("wait", "30")
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	m, status, err := sc.getJSON(ctx, "/api/v1/douyin/user/posts", q)
	if err != nil {
		return nil, err
	}
	if status == http.StatusAccepted {
		return nil, fmt.Errorf("服务返回异步任务（202），请稍后重试")
	}
	if status >= 400 {
		return nil, fmt.Errorf("HTTP %d", status)
	}
	if !dyB(m, "success") {
		if e := dyM(m["error"]); e != nil {
			return nil, fmt.Errorf("%s", dyS(e, "message", "code"))
		}
		return nil, fmt.Errorf("服务返回失败")
	}
	data := dyM(m["data"])
	items := dyA(data["items"])
	if data == nil || items == nil {
		return nil, fmt.Errorf("响应缺少 items")
	}
	hasMore := dyB(data, "has_more")
	return &dyPage{items: items, next: dyS(data, "cursor"), hasMore: hasMore}, nil
}

// profile 作者资料（尽力而为；失败不影响作品列表）
func (sc *dySidecar) profile(ctx context.Context, secUID string) map[string]any {
	q := url.Values{}
	q.Set("sec_user_id", secUID)
	if sc.mode != "v4" {
		if m, st, err := sc.getJSON(ctx, "/api/v1/douyin/user", q); err == nil && st < 400 && dyB(m, "success") {
			if d := dyM(m["data"]); d != nil {
				return d
			}
		}
	}
	if sc.mode != "v5" {
		if m, st, err := sc.getJSON(ctx, "/douyin/web/handler_user_profile", q); err == nil && st < 400 {
			if code, _ := dyI(m, "code"); code == 200 || code == 0 {
				if d := dyM(m["data"]); d != nil {
					if u := dyM(d["user"]); u != nil {
						return u
					}
					return d
				}
			}
		}
	}
	return nil
}

// ---------- 归一化 ----------

func dyStatsJSON(st map[string]any) map[string]any {
	out := map[string]any{}
	// play_count 恒 0 占位：只有 >0 才输出，避免「播放 0」误导
	if v := dyI0(st, "play_count"); v > 0 {
		out["play_count"] = v
	}
	if v, ok := dyI(st, "digg_count"); ok {
		out["digg_count"] = v
	}
	if v, ok := dyI(st, "comment_count"); ok {
		out["comment_count"] = v
	}
	if v, ok := dyI(st, "share_count"); ok {
		out["share_count"] = v
	}
	if v, ok := dyI(st, "collect_count"); ok {
		out["collect_count"] = v
	}
	return out
}

// dyVideoFromV4 原始 aweme 节点 → 统一作品对象
func dyVideoFromV4(n map[string]any) map[string]any {
	id := dyS(n, "aweme_id")
	if id == "" {
		return nil
	}
	desc := dyS(n, "desc")
	images := dyA(n["images"])
	kind := "video"
	if len(images) > 0 {
		kind = "image"
	}
	vid := dyM(n["video"])
	cover := dyFirstURL(dyM(vid["origin_cover"]), dyM(vid["cover"]), dyM(vid["dynamic_cover"]))
	if cover == "" && kind == "image" && len(images) > 0 {
		cover = dyFirstURL(dyM(images[0]))
	}
	webURL := "https://www.douyin.com/video/" + id
	if kind == "image" {
		webURL = "https://www.douyin.com/note/" + id
	}
	ct, _ := dyI(n, "create_time")
	durMs := dyI0(vid, "duration")
	status := dyM(n["status"])
	out := map[string]any{
		"aweme_id":    id,
		"title":       dyTitle(desc),
		"desc":        desc,
		"kind":        kind,
		"url":         webURL,
		"create_time": ct,
		"duration":    durMs / 1000,
		"stats":       dyStatsJSON(dyM(n["statistics"])),
	}
	if cover != "" {
		out["cover"] = cover
	}
	if dyB(n, "is_top") {
		out["is_top"] = true
	}
	if dyB(status, "is_delete") {
		out["is_deleted"] = true
	}
	if ps, ok := dyI(status, "private_status"); ok && ps != 0 {
		out["is_private"] = true
	}
	return out
}

// dyVideoFromV5 归一化 Content 节点 → 统一作品对象
func dyVideoFromV5(n map[string]any) map[string]any {
	id := dyS(n, "content_id")
	if id == "" {
		return nil
	}
	kind := "video"
	if k := dyS(n, "kind"); strings.Contains(k, "image") {
		kind = "image"
	}
	media := dyM(n["media"])
	var cover string
	if covers := dyA(media["covers"]); len(covers) > 0 {
		cover = dyFirstURL(dyM(covers[0]))
	}
	if cover == "" {
		cover = dyFirstURL(dyM(media["cover"]))
	}
	if cover == "" && kind == "image" {
		if imgs := dyA(media["images"]); len(imgs) > 0 {
			cover = dyFirstURL(dyM(imgs[0]))
		}
	}
	createTime := int64(0)
	if s := dyS(n, "created_at"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			createTime = t.Unix()
		}
	}
	out := map[string]any{
		"aweme_id":    id,
		"title":       dyTitle(dyS(n, "title", "description")),
		"desc":        dyS(n, "description", "title"),
		"kind":        kind,
		"url":         dyS(n, "web_url"),
		"create_time": createTime,
		"duration":    dyI0(n, "duration_ms") / 1000,
		"stats":       dyStatsJSON(dyM(n["stats"])),
	}
	if cover != "" {
		out["cover"] = cover
	}
	if dyB(n, "is_deleted") {
		out["is_deleted"] = true
	}
	if dyB(n, "is_private") {
		out["is_private"] = true
	}
	return out
}

func (sc *dySidecar) videoOf(n map[string]any) map[string]any {
	if sc.mode == "v5" {
		return dyVideoFromV5(n)
	}
	return dyVideoFromV4(n)
}

// dyAuthorJSON 作者信息：资料接口结果优先，作品里的作者节点兜底
func dyAuthorJSON(secUID string, prof, fallback map[string]any) map[string]any {
	av := dyFirstURL(dyM(prof["avatar_thumb"]), dyM(prof["avatar"]), dyM(prof["picture"]))
	if av == "" && fallback != nil {
		av = dyFirstURL(dyM(fallback["avatar_thumb"]), dyM(fallback["avatar"]))
	}
	nick := dyS(prof, "nickname", "display_name", "name")
	if nick == "" && fallback != nil {
		nick = dyS(fallback, "nickname")
	}
	out := map[string]any{
		"sec_uid":  secUID,
		"nickname": nick,
		"avatar":   av,
	}
	if prof != nil {
		st := dyM(prof["stats"])
		get := func(keys ...string) (int64, bool) {
			if n, ok := dyI(prof, keys...); ok {
				return n, true
			}
			return dyI(st, keys...)
		}
		if v, ok := get("follower_count"); ok {
			out["follower_count"] = v
		}
		if v, ok := get("following_count"); ok {
			out["following_count"] = v
		}
		if v, ok := dyI(prof, "aweme_count"); ok {
			out["aweme_count"] = v
		} else if v, ok := get("content_count"); ok {
			out["aweme_count"] = v
		}
		if v, ok := get("total_favorited", "total_digg"); ok {
			out["total_favorited"] = v
		}
		if sig := dyS(prof, "signature"); sig != "" {
			out["signature"] = sig
		}
	}
	return out
}

func dySummary(videos []map[string]any, pages int, truncated bool) map[string]any {
	var totalDigg, totalComment, totalShare, totalCollect int64
	var latest, earliest int64
	var topID, topTitle string
	var topDigg int64
	for _, v := range videos {
		st := dyM(v["stats"])
		d := dyI0(st, "digg_count")
		totalDigg += d
		totalComment += dyI0(st, "comment_count")
		totalShare += dyI0(st, "share_count")
		totalCollect += dyI0(st, "collect_count")
		if ct, _ := dyI(v, "create_time"); ct > 0 {
			if latest == 0 || ct > latest {
				latest = ct
			}
			if earliest == 0 || ct < earliest {
				earliest = ct
			}
		}
		if d >= topDigg {
			topDigg = d
			topID = dyS(v, "aweme_id")
			topTitle = dyS(v, "title")
		}
	}
	n := int64(len(videos))
	avg := func(a int64) int64 {
		if n > 0 {
			return a / n
		}
		return 0
	}
	out := map[string]any{
		"video_count":   n,
		"total_digg":    totalDigg,
		"total_comment": totalComment,
		"total_share":   totalShare,
		"total_collect": totalCollect,
		"avg_digg":      avg(totalDigg),
		"avg_comment":   avg(totalComment),
		"avg_share":     avg(totalShare),
		"avg_collect":   avg(totalCollect),
		"latest_time":   latest,
		"earliest_time": earliest,
		"pages":         pages,
		"truncated":     truncated,
	}
	if topID != "" {
		out["top_aweme_id"] = topID
		out["top_title"] = topTitle
		out["top_digg"] = topDigg
	}
	return out
}

// ---------- 拉取与归一化（/v1/dyzz 与关注作者共用） ----------

// dyFetchResult 一次完整拉取的产物
type dyFetchResult struct {
	videos    []map[string]any
	fbAuthor  map[string]any
	pages     int
	truncated bool
}

// dyFetchAll 从旁挂服务翻页拉取作者的公开作品（最多 limit 条）。
// 用于 /v1/dyzz 即时查询，以及「关注作者」时的首次全量同步。
func dyFetchAll(ctx context.Context, sc *dySidecar, secUID string, limit int) (*dyFetchResult, error) {
	res := &dyFetchResult{videos: []map[string]any{}}
	cursor := ""
	for {
		p, perr := sc.postsPage(ctx, secUID, cursor)
		if perr != nil {
			return nil, perr
		}
		res.pages++
		for _, it := range p.items {
			n := dyM(it)
			if n == nil {
				continue
			}
			if res.fbAuthor == nil {
				res.fbAuthor = dyM(n["author"])
			}
			if len(res.videos) >= limit {
				res.truncated = true
				break
			}
			if v := sc.videoOf(n); v != nil {
				res.videos = append(res.videos, v)
			}
		}
		if res.truncated || !p.hasMore || p.next == "" || p.next == cursor ||
			res.pages >= dyMaxPages || time.Now().After(ctxDeadline(ctx)) {
			break
		}
		cursor = p.next
		time.Sleep(300 * time.Millisecond)
	}
	return res, nil
}

// dyVideoInput 统一作品对象 → 落库结构（字段白名单，数值强转）
func dyVideoInput(m map[string]any) store.DyVideoInput {
	st := dyM(m["stats"])
	in := store.DyVideoInput{
		AwemeID:      dyS(m, "aweme_id"),
		Title:        truncateRunes(dyS(m, "title"), 120),
		Desc:         truncateRunes(dyS(m, "desc"), 600),
		Kind:         dyS(m, "kind"),
		URL:          truncateRunes(dyS(m, "url"), 200),
		Cover:        truncateRunes(dyS(m, "cover"), 400),
		CreateTime:   dyI0(m, "create_time"),
		Duration:     int(dyI0(m, "duration")),
		IsTop:        dyB(m, "is_top"),
		IsDeleted:    dyB(m, "is_deleted"),
		IsPrivate:    dyB(m, "is_private"),
		DiggCount:    dyI0(st, "digg_count"),
		CommentCount: dyI0(st, "comment_count"),
		ShareCount:   dyI0(st, "share_count"),
		CollectCount: dyI0(st, "collect_count"),
	}
	if in.Kind != "image" {
		in.Kind = "video"
	}
	if in.URL == "" && in.AwemeID != "" {
		if in.Kind == "image" {
			in.URL = "https://www.douyin.com/note/" + in.AwemeID
		} else {
			in.URL = "https://www.douyin.com/video/" + in.AwemeID
		}
	}
	return in
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// ---------- 入口 ----------

// handleDyzz 本地能力入口；返回写出的 HTTP 状态码供网关计量。
func (s *Server) handleDyzz(w http.ResponseWriter, r *http.Request) int {
	q := r.URL.Query()
	input := strings.TrimSpace(q.Get("url"))
	if input == "" {
		input = strings.TrimSpace(q.Get("sec_user_id"))
	}
	limit := queryInt(r, "limit", 50)
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if input == "" {
		gwError(w, http.StatusBadRequest, "missing_url", "请填写作者主页链接、主页分享文案或 sec_uid")
		return http.StatusBadRequest
	}

	secUID, err := dyResolveSecUID(input)
	if err != nil {
		gwError(w, http.StatusBadRequest, "bad_url", err.Error())
		return http.StatusBadRequest
	}

	// 短缓存：同样的作者 60 秒内直接回放，防手抖重复打爆旁挂服务
	ckey := "dyzz|" + secUID + "|" + strconv.Itoa(limit)
	if e, ok := gwCacheGet(ckey); ok {
		if ct := e.header.Get("Content-Type"); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		w.Header().Set("X-Cache", "HIT")
		w.Header().Set("X-Gateway", "dyzz")
		w.WriteHeader(e.status)
		_, _ = w.Write(e.body)
		return e.status
	}

	sc, serr := s.dySidecar()
	if serr != nil {
		gwError(w, http.StatusServiceUnavailable, "no_sidecar", serr.Error())
		return http.StatusServiceUnavailable
	}

	ctx, cancel := context.WithTimeout(r.Context(), dyTimeBudget)
	defer cancel()

	res, ferr := dyFetchAll(ctx, sc, secUID, limit)
	if ferr != nil {
		gwError(w, http.StatusBadGateway, "sidecar_error", "抖音数据服务调用失败："+ferr.Error())
		return http.StatusBadGateway
	}
	if len(res.videos) == 0 {
		gwError(w, http.StatusNotFound, "no_videos",
			"没有获取到该作者的公开作品：可能账号没有作品、作品全部私密，或数据服务临时被平台风控，稍后再试")
		return http.StatusNotFound
	}

	prof := sc.profile(ctx, secUID)
	resp := map[string]any{
		"code": 200,
		"msg":  "查询成功",
		"data": map[string]any{
			"sec_uid": secUID,
			"author":  dyAuthorJSON(secUID, prof, res.fbAuthor),
			"videos":  res.videos,
			"summary": dySummary(res.videos, res.pages, res.truncated),
			"source":  sc.mode,
		},
	}
	body, _ := json.Marshal(resp)
	hdr := http.Header{}
	hdr.Set("Content-Type", "application/json; charset=utf-8")
	gwCachePut(ckey, http.StatusOK, hdr, body)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Gateway", "dyzz")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	return http.StatusOK
}

// ctxDeadline 翻页循环的时间闸门（到点就收，保证整体响应时间可控）
func ctxDeadline(ctx context.Context) time.Time {
	d, ok := ctx.Deadline()
	if !ok {
		return time.Now().Add(time.Hour)
	}
	return d.Add(-2 * time.Second)
}
