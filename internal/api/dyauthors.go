package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"jxivideo/internal/store"
)

// 我的作者：登录用户保存关注的抖音作者，作品快照落库，支持增量刷新与批量深查。
// 深查（播放量/互动率）走站内端点直接调上游 dytj，不占 /v1 网关的游客额度。

const (
	dyFollowMaxVideos  = 200 // 关注时最多落库条数
	dyRefreshMaxVideos = 100 // 单次增量刷新最多拉取条数
	dyRefreshMaxPages  = 5   // 增量刷新最多翻页数（遇已知作品提前停）
	dyStatsBatchMax    = 20  // 单批深查条数上限（前端分批调用）
	dyStatsInterval    = 400 * time.Millisecond
)

// ---------- 关注管理 ----------

// handleDyAuthorList GET /api/dy/authors
func (s *Server) handleDyAuthorList(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	list, err := s.st.ListDyAuthors(u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "读取关注列表失败")
		return
	}
	writeOK(w, map[string]any{"items": list})
}

// dyAuthorPayload 关注请求体：要么带能力页结果（author+videos，零额外爬取），
// 要么只带链接/sec_uid 由服务端自己拉取。
type dyAuthorPayload struct {
	URL    string         `json:"url"`
	SecUID string         `json:"secUid"`
	Author map[string]any `json:"author"`
	Videos []map[string]any `json:"videos"`
}

// handleDyAuthorAdd POST /api/dy/authors
func (s *Server) handleDyAuthorAdd(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	var p dyAuthorPayload
	if err := readJSON(r, &p); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式有误")
		return
	}

	input := strings.TrimSpace(p.SecUID)
	if input == "" {
		input = strings.TrimSpace(p.URL)
	}
	if input == "" {
		writeErr(w, http.StatusBadRequest, "missing_url", "请提供作者主页链接或 sec_uid")
		return
	}
	secUID, err := dyResolveSecUID(input)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_url", err.Error())
		return
	}

	var (
		nickname, avatar, signature string
		follower, awemeCount        int64
		inputs                      []store.DyVideoInput
	)
	// 客户端带来的快照数据（来自 /v1/dyzz 结果页、且 sec_uid 与解析结果一致）优先采用，
	// 避免重复爬取；没带就由服务端自己全量拉一次。
	if len(p.Videos) > 0 && strings.TrimSpace(p.SecUID) == secUID {
		for _, v := range p.Videos {
			if len(inputs) >= dyFollowMaxVideos {
				break
			}
			inputs = append(inputs, dyVideoInput(v))
		}
		nickname = dyS(p.Author, "nickname")
		avatar = truncateRunes(dyS(p.Author, "avatar"), 400)
		signature = truncateRunes(dyS(p.Author, "signature"), 300)
		follower = dyI0(p.Author, "follower_count")
		awemeCount = dyI0(p.Author, "aweme_count")
	} else {
		sc, serr := s.dySidecar()
		if serr != nil {
			writeErr(w, http.StatusServiceUnavailable, "no_sidecar", serr.Error())
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), dyTimeBudget)
		defer cancel()
		res, ferr := dyFetchAll(ctx, sc, secUID, dyFollowMaxVideos)
		if ferr != nil {
			writeErr(w, http.StatusBadGateway, "sidecar_error", "抖音数据服务调用失败："+ferr.Error())
			return
		}
		for _, v := range res.videos {
			inputs = append(inputs, dyVideoInput(v))
		}
		prof := sc.profile(ctx, secUID)
		au := dyAuthorJSON(secUID, prof, res.fbAuthor)
		nickname, _ = au["nickname"].(string)
		avatar, _ = au["avatar"].(string)
		signature, _ = au["signature"].(string)
		if v, ok := au["follower_count"].(int64); ok {
			follower = v
		}
		if v, ok := au["aweme_count"].(int64); ok {
			awemeCount = v
		}
	}

	if _, err := s.st.UpsertDyVideos(secUID, inputs); err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "保存作品快照失败")
		return
	}
	au, err := s.st.UpsertDyAuthor(u.ID, secUID, nickname, avatar, signature, follower, awemeCount)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "保存关注失败")
		return
	}
	_ = s.st.TouchDyAuthorSync(u.ID, au.ID, follower, awemeCount, int64(len(inputs)))
	au, _ = s.st.GetDyAuthor(u.ID, au.ID)
	writeOK(w, map[string]any{"author": au, "saved": len(inputs)})
}

// handleDyAuthorDelete DELETE /api/dy/authors/{id}
func (s *Server) handleDyAuthorDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	if err := s.st.DeleteDyAuthor(u.ID, pathID(r)); err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "关注不存在")
		return
	}
	writeOK(w, map[string]any{"ok": true})
}

// ---------- 增量刷新 ----------

// handleDyAuthorRefresh POST /api/dy/authors/{id}/refresh
//
// 只拉新作品：逐页取列表，新作品入库、已知作品顺带刷新统计，
// 一整页没有任何新作品就停（新作品按时间倒序只会出现在前部）。
func (s *Server) handleDyAuthorRefresh(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	au, err := s.st.GetDyAuthor(u.ID, pathID(r))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found", "关注不存在")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "读取关注失败")
		return
	}

	sc, serr := s.dySidecar()
	if serr != nil {
		writeErr(w, http.StatusServiceUnavailable, "no_sidecar", serr.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dyTimeBudget)
	defer cancel()

	known, err := s.st.DyKnownAwemeIDs(au.SecUID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "读取快照失败")
		return
	}

	cursor := ""
	newN, updN, pages := 0, 0, 0
	var prof map[string]any
	var fbAuthor map[string]any
	for pages < dyRefreshMaxPages {
		p, perr := sc.postsPage(ctx, au.SecUID, cursor)
		if perr != nil {
			writeErr(w, http.StatusBadGateway, "sidecar_error", "抖音数据服务调用失败："+perr.Error())
			return
		}
		pages++
		batch := []store.DyVideoInput{}
		for _, it := range p.items {
			n := dyM(it)
			if n == nil {
				continue
			}
			if fbAuthor == nil {
				fbAuthor = dyM(n["author"])
			}
			if len(batch) >= dyRefreshMaxVideos {
				break
			}
			// 上游返回的是原始节点，先归一化成统一作品对象再落库
			if v := sc.videoOf(n); v != nil {
				batch = append(batch, dyVideoInput(v))
			}
		}
		fresh, uerr := s.upsertCounting(au.SecUID, known, batch)
		if uerr != nil {
			writeErr(w, http.StatusInternalServerError, "db", "保存快照失败")
			return
		}
		newN += fresh
		updN += len(batch) - fresh
		if fresh == 0 || !p.hasMore || p.next == "" || p.next == cursor ||
			len(batch) >= dyRefreshMaxVideos || pages >= dyRefreshMaxPages ||
			time.Now().After(ctxDeadline(ctx)) {
			break
		}
		cursor = p.next
		time.Sleep(300 * time.Millisecond)
	}

	prof = sc.profile(ctx, au.SecUID)
	pau := dyAuthorJSON(au.SecUID, prof, fbAuthor)
	follower := dyI0(pau, "follower_count")
	awemeCount := dyI0(pau, "aweme_count")
	_ = s.st.TouchDyAuthorSync(u.ID, au.ID, follower, awemeCount, int64(newN))

	updated, _ := s.st.GetDyAuthor(u.ID, au.ID)
	writeOK(w, map[string]any{
		"author":     updated,
		"new_count":  newN,
		"updated":    updN,
		"pages":      pages,
	})
}

// upsertCounting 落库并按「库里是否本来就有」精确统计新增数
func (s *Server) upsertCounting(secUID string, known map[string]bool, batch []store.DyVideoInput) (int, error) {
	fresh := 0
	for _, v := range batch {
		if v.AwemeID == "" {
			continue
		}
		if !known[v.AwemeID] {
			fresh++
			known[v.AwemeID] = true
		}
	}
	_, err := s.st.UpsertDyVideos(secUID, batch)
	return fresh, err
}

// ---------- 作品快照 ----------

// handleDyAuthorVideos GET /api/dy/authors/{id}/videos?page=&size=
func (s *Server) handleDyAuthorVideos(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	au, err := s.st.GetDyAuthor(u.ID, pathID(r))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found", "关注不存在")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "读取关注失败")
		return
	}
	page := queryInt(r, "page", 1)
	if page < 1 {
		page = 1
	}
	size := queryInt(r, "size", 50)
	if size < 1 {
		size = 50
	}
	if size > 200 {
		size = 200
	}
	items, total, err := s.st.ListDyVideos(au.SecUID, size, (page-1)*size)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "读取作品失败")
		return
	}
	writeOK(w, map[string]any{"items": items, "total": total, "author": au})
}

// ---------- 批量深查（播放量等，来自上游 dytj） ----------

// handleDyAuthorStats POST /api/dy/authors/{id}/stats  body: {aweme_ids: [...]}
func (s *Server) handleDyAuthorStats(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	au, err := s.st.GetDyAuthor(u.ID, pathID(r))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found", "关注不存在")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "读取关注失败")
		return
	}

	var p struct {
		AwemeIDs []string `json:"aweme_ids"`
	}
	if err := readJSON(r, &p); err != nil || len(p.AwemeIDs) == 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "请提供要查询的作品 ID 列表")
		return
	}
	if len(p.AwemeIDs) > dyStatsBatchMax {
		writeErr(w, http.StatusBadRequest, "too_many", "每批最多查询 "+strconv.Itoa(dyStatsBatchMax)+" 个作品，请分批")
		return
	}

	results := []store.DyVideo{}
	errs := []map[string]string{}
	for i, id := range p.AwemeIDs {
		id = strings.TrimSpace(id)
		if id == "" || !dyAwemeIDRe.MatchString(id) {
			errs = append(errs, map[string]string{"aweme_id": id, "error": "作品 ID 格式不正确"})
			continue
		}
		if err := s.dyDeepQuery(au.SecUID, id); err != nil {
			errs = append(errs, map[string]string{"aweme_id": id, "error": err.Error()})
			continue
		}
		if i < len(p.AwemeIDs)-1 {
			time.Sleep(dyStatsInterval)
		}
	}
	if len(errs) == 0 {
		results, err = s.st.GetDyVideosByIDs(au.SecUID, p.AwemeIDs)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "db", "读取查询结果失败")
			return
		}
	} else {
		// 有失败也把成功的部分带回来
		okIDs := []string{}
		failed := map[string]bool{}
		for _, e := range errs {
			failed[e["aweme_id"]] = true
		}
		for _, id := range p.AwemeIDs {
			if !failed[strings.TrimSpace(id)] {
				okIDs = append(okIDs, strings.TrimSpace(id))
			}
		}
		if len(okIDs) > 0 {
			results, _ = s.st.GetDyVideosByIDs(au.SecUID, okIDs)
		}
	}
	writeOK(w, map[string]any{"items": results, "errors": errs})
}

// dyAwemeIDRe 作品 ID：15-22 位、非 0 开头的十进制数字串
var dyAwemeIDRe = regexp.MustCompile(`^[1-9][0-9]{14,21}$`)

// dyDeepQuery 调上游 dytj 查单个作品并把结果落库
func (s *Server) dyDeepQuery(secUID, awemeID string) error {
	cfg := s.st.Settings()
	base := strings.TrimRight(strings.TrimSpace(cfg.UpstreamBase), "/")
	if base == "" {
		return errors.New("上游未配置，无法查询作品统计")
	}
	upPath := "/api/dytj"
	if it, err := s.st.GetInterface("dytj"); err == nil && strings.TrimSpace(it.UpstreamPath) != "" {
		upPath = strings.TrimSpace(it.UpstreamPath)
	}
	target := base + upPath + "?id=" + url.QueryEscape(awemeID)
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return errors.New("构造请求失败")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ClearFrame-Gateway/1.0")
	if key := strings.TrimSpace(cfg.UpstreamKey); key != "" {
		hdr := strings.TrimSpace(cfg.UpstreamAuthHdr)
		if hdr == "" {
			hdr = "x-api-key"
		}
		val := cfg.UpstreamAuthPfx + key
		if strings.EqualFold(hdr, "Authorization") && cfg.UpstreamAuthPfx == "" {
			val = "Bearer " + key
		}
		req.Header.Set(hdr, val)
	}

	client := &http.Client{Transport: upstreamTransport, Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("上游请求失败")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return errors.New("读取上游响应失败")
	}
	var out struct {
		Code    json.Number `json:"code"`
		Msg     string      `json:"msg"`
		Data    struct {
			Extra struct {
				Statistics   map[string]any `json:"statistics"`
				StatisticsTrend map[string]any `json:"statistics_trend"`
			} `json:"extra"`
		} `json:"data"`
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return errors.New("上游响应不是 JSON")
	}
	if n, _ := out.Code.Int64(); n != 200 {
		msg := strings.TrimSpace(out.Msg)
		if msg == "" {
			msg = "查询失败"
		}
		return fmt.Errorf("%s", msg)
	}
	st := out.Data.Extra.Statistics
	var play *int64
	var rate *float64
	if v, ok := dyI(st, "play_count"); ok && v > 0 {
		play = &v
	}
	if v, ok := dyF(st, "interact_rate"); ok {
		rate = &v
	}
	if err := s.st.UpdateDyVideoStats(secUID, awemeID,
		dyI0(st, "digg_count"), dyI0(st, "comment_count"),
		dyI0(st, "share_count"), dyI0(st, "collect_count"), play, rate); err != nil {
		log.Printf("dyzz: 保存深查结果失败 aweme=%s: %v", awemeID, err)
		return errors.New("保存查询结果失败")
	}
	return nil
}
