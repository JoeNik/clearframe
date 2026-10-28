package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// 关注的抖音作者 + 作品快照。
//
// dy_authors 是「关注关系」，按用户隔离：同一 sec_uid 可以被多个用户各自关注。
// dy_videos 是「作品快照」，按作者（sec_uid）全局共享：公开数据谁刷新过，
// 其他关注同一作者的用户直接读库即可，不必重复爬取。取关只删关注关系，不删快照。

// DyAuthor 一条关注关系
type DyAuthor struct {
	ID            int64  `json:"id"`
	UserID        int64  `json:"userId"`
	SecUID        string `json:"secUid"`
	Nickname      string `json:"nickname"`
	Avatar        string `json:"avatar"`
	Signature     string `json:"signature"`
	FollowerCount int64  `json:"followerCount"`
	AwemeCount    int64  `json:"awemeCount"`
	LastSyncedAt  int64  `json:"lastSyncedAt"`
	LastNewCount  int64  `json:"lastNewCount"`
	CreatedAt     int64  `json:"createdAt"`
	VideoCount    int64  `json:"videoCount"` // 联查 dy_videos 的快照条数
}

// DyVideoInput 归一化后的作品数据（api 层从上游响应提取，store 层落库）
type DyVideoInput struct {
	AwemeID       string
	Title         string
	Desc          string
	Kind          string
	URL           string
	Cover         string
	CreateTime    int64
	Duration      int
	IsTop         bool
	IsDeleted     bool
	IsPrivate     bool
	DiggCount     int64
	CommentCount  int64
	ShareCount    int64
	CollectCount  int64
}

// DyVideo 一条作品快照（含深查得到的播放量）
type DyVideo struct {
	ID            int64    `json:"id"`
	SecUID        string   `json:"secUid"`
	AwemeID       string   `json:"awemeId"`
	Title         string   `json:"title"`
	Desc          string   `json:"desc"`
	Kind          string   `json:"kind"`
	URL           string   `json:"url"`
	Cover         string   `json:"cover"`
	CreateTime    int64    `json:"createTime"`
	Duration      int      `json:"duration"`
	IsTop         bool     `json:"isTop"`
	IsDeleted     bool     `json:"isDeleted"`
	IsPrivate     bool     `json:"isPrivate"`
	DiggCount     int64    `json:"diggCount"`
	CommentCount  int64    `json:"commentCount"`
	ShareCount    int64    `json:"shareCount"`
	CollectCount  int64    `json:"collectCount"`
	PlayCount     *int64   `json:"playCount"`
	InteractRate  *float64 `json:"interactRate"`
	StatsUpdatedAt int64   `json:"statsUpdatedAt"`
	FirstSeenAt   int64    `json:"firstSeenAt"`
	LastSeenAt    int64    `json:"lastSeenAt"`
}

const dyAuthorCols = `id, user_id, sec_uid, nickname, avatar, signature, follower_count,
	aweme_count, last_synced_at, last_new_count, created_at`

func scanDyAuthor(sc interface{ Scan(...any) error }) (DyAuthor, error) {
	var a DyAuthor
	err := sc.Scan(&a.ID, &a.UserID, &a.SecUID, &a.Nickname, &a.Avatar, &a.Signature,
		&a.FollowerCount, &a.AwemeCount, &a.LastSyncedAt, &a.LastNewCount, &a.CreatedAt)
	return a, err
}

// ---------- 关注关系 ----------

// ListDyAuthors 用户的关注列表（附快照条数）
func (s *Store) ListDyAuthors(userID int64) ([]DyAuthor, error) {
	q := `SELECT ` + dyAuthorCols + `,
		(SELECT COUNT(1) FROM dy_videos v WHERE v.sec_uid = a.sec_uid) AS video_count
		FROM dy_authors a WHERE a.user_id = ? ORDER BY a.id DESC`
	rows, err := s.db.Query(q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DyAuthor{}
	for rows.Next() {
		var a DyAuthor
		if err := rows.Scan(&a.ID, &a.UserID, &a.SecUID, &a.Nickname, &a.Avatar, &a.Signature,
			&a.FollowerCount, &a.AwemeCount, &a.LastSyncedAt, &a.LastNewCount, &a.CreatedAt,
			&a.VideoCount); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetDyAuthor 按归属查关注关系
func (s *Store) GetDyAuthor(userID, id int64) (DyAuthor, error) {
	row := s.db.QueryRow(`SELECT `+dyAuthorCols+`,
		(SELECT COUNT(1) FROM dy_videos v WHERE v.sec_uid = a.sec_uid)
		FROM dy_authors a WHERE a.id = ? AND a.user_id = ?`, id, userID)
	var a DyAuthor
	err := row.Scan(&a.ID, &a.UserID, &a.SecUID, &a.Nickname, &a.Avatar, &a.Signature,
		&a.FollowerCount, &a.AwemeCount, &a.LastSyncedAt, &a.LastNewCount, &a.CreatedAt,
		&a.VideoCount)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// GetDyAuthorBySec 按归属 + sec_uid 查（关注去重）
func (s *Store) GetDyAuthorBySec(userID int64, secUID string) (DyAuthor, error) {
	row := s.db.QueryRow(`SELECT `+dyAuthorCols+`,
		(SELECT COUNT(1) FROM dy_videos v WHERE v.sec_uid = a.sec_uid)
		FROM dy_authors a WHERE a.user_id = ? AND a.sec_uid = ?`, userID, secUID)
	var a DyAuthor
	err := row.Scan(&a.ID, &a.UserID, &a.SecUID, &a.Nickname, &a.Avatar, &a.Signature,
		&a.FollowerCount, &a.AwemeCount, &a.LastSyncedAt, &a.LastNewCount, &a.CreatedAt,
		&a.VideoCount)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// UpsertDyAuthor 关注（或刷新资料）。已有同 sec_uid 关注时只更新资料字段。
func (s *Store) UpsertDyAuthor(userID int64, secUID, nickname, avatar, signature string, follower, awemeCount int64) (DyAuthor, error) {
	now := time.Now().Unix()
	_, err := s.db.Exec(`INSERT INTO dy_authors(user_id, sec_uid, nickname, avatar, signature,
		follower_count, aweme_count, last_synced_at, last_new_count, created_at)
		VALUES(?,?,?,?,?,?,?,0,0,?)
		ON CONFLICT(user_id, sec_uid) DO UPDATE SET
		nickname = CASE WHEN excluded.nickname != '' THEN excluded.nickname ELSE dy_authors.nickname END,
		avatar = CASE WHEN excluded.avatar != '' THEN excluded.avatar ELSE dy_authors.avatar END,
		signature = excluded.signature,
		follower_count = excluded.follower_count,
		aweme_count = excluded.aweme_count`,
		userID, secUID, nickname, avatar, signature, follower, awemeCount, now)
	if err != nil {
		return DyAuthor{}, err
	}
	return s.GetDyAuthorBySec(userID, secUID)
}

// TouchDyAuthorSync 记录一次同步（全量或增量）的结果
func (s *Store) TouchDyAuthorSync(userID, id int64, follower, awemeCount, newCount int64) error {
	_, err := s.db.Exec(`UPDATE dy_authors SET last_synced_at = ?, last_new_count = ?,
		follower_count = CASE WHEN ? > 0 THEN ? ELSE follower_count END,
		aweme_count = CASE WHEN ? > 0 THEN ? ELSE aweme_count END
		WHERE id = ? AND user_id = ?`,
		time.Now().Unix(), newCount, follower, follower, awemeCount, awemeCount, id, userID)
	return err
}

// DeleteDyAuthor 取关（不动作品快照）
func (s *Store) DeleteDyAuthor(userID, id int64) error {
	res, err := s.db.Exec(`DELETE FROM dy_authors WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- 作品快照 ----------

// UpsertDyVideos 批量落库：新作品插入，已知作品更新统计与 last_seen。
// 返回本次新插入条数。
func (s *Store) UpsertDyVideos(secUID string, list []DyVideoInput) (int, error) {
	if len(list) == 0 {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	newN := 0
	for _, v := range list {
		if strings.TrimSpace(v.AwemeID) == "" {
			continue
		}
		res, err := tx.Exec(`INSERT INTO dy_videos(sec_uid, aweme_id, title, descr, kind, url, cover,
			create_time, duration, is_top, is_deleted, is_private,
			digg_count, comment_count, share_count, collect_count, first_seen_at, last_seen_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(sec_uid, aweme_id) DO UPDATE SET
			title = CASE WHEN excluded.title != '' THEN excluded.title ELSE dy_videos.title END,
			descr = CASE WHEN excluded.descr != '' THEN excluded.descr ELSE dy_videos.descr END,
			kind = excluded.kind,
			url = CASE WHEN excluded.url != '' THEN excluded.url ELSE dy_videos.url END,
			cover = CASE WHEN excluded.cover != '' THEN excluded.cover ELSE dy_videos.cover END,
			create_time = CASE WHEN excluded.create_time > 0 THEN excluded.create_time ELSE dy_videos.create_time END,
			duration = CASE WHEN excluded.duration > 0 THEN excluded.duration ELSE dy_videos.duration END,
			is_top = excluded.is_top,
			is_deleted = excluded.is_deleted, is_private = excluded.is_private,
			digg_count = excluded.digg_count, comment_count = excluded.comment_count,
			share_count = excluded.share_count, collect_count = excluded.collect_count,
			last_seen_at = excluded.last_seen_at`,
			secUID, v.AwemeID, v.Title, v.Desc, v.Kind, v.URL, v.Cover,
			v.CreateTime, v.Duration, boolInt(v.IsTop), boolInt(v.IsDeleted), boolInt(v.IsPrivate),
			v.DiggCount, v.CommentCount, v.ShareCount, v.CollectCount, now, now)
		if err != nil {
			return newN, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			// RowsAffected 对 upsert 的 insert 与 update 都返回 1，
			// 用变化前是否已存在来判断新增：查一次总比误报好。
			var exist int
			_ = tx.QueryRow(`SELECT 1 FROM dy_videos WHERE sec_uid = ? AND aweme_id = ? AND first_seen_at < ?`,
				secUID, v.AwemeID, now).Scan(&exist)
			if exist == 0 {
				newN++
			}
		}
	}
	return newN, tx.Commit()
}

// DyKnownAwemeIDs 作者已知作品 ID 集合（增量刷新用）
func (s *Store) DyKnownAwemeIDs(secUID string) (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT aweme_id FROM dy_videos WHERE sec_uid = ?`, secUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			out[id] = true
		}
	}
	return out, rows.Err()
}

const dyVideoCols = `id, sec_uid, aweme_id, title, descr, kind, url, cover,
	create_time, duration, is_top, is_deleted, is_private,
	digg_count, comment_count, share_count, collect_count,
	play_count, interact_rate, stats_updated_at, first_seen_at, last_seen_at`

func scanDyVideo(sc interface{ Scan(...any) error }) (DyVideo, error) {
	var v DyVideo
	var isTop, isDeleted, isPrivate int
	var play sql.NullInt64
	var rate sql.NullFloat64
	err := sc.Scan(&v.ID, &v.SecUID, &v.AwemeID, &v.Title, &v.Desc, &v.Kind, &v.URL, &v.Cover,
		&v.CreateTime, &v.Duration, &isTop, &isDeleted, &isPrivate,
		&v.DiggCount, &v.CommentCount, &v.ShareCount, &v.CollectCount,
		&play, &rate, &v.StatsUpdatedAt, &v.FirstSeenAt, &v.LastSeenAt)
	if err != nil {
		return v, err
	}
	v.IsTop = isTop != 0
	v.IsDeleted = isDeleted != 0
	v.IsPrivate = isPrivate != 0
	if play.Valid {
		p := play.Int64
		v.PlayCount = &p
	}
	if rate.Valid {
		r := rate.Float64
		v.InteractRate = &r
	}
	return v, nil
}

// ListDyVideos 作品快照分页（按发布时间倒序，置顶在前）
func (s *Store) ListDyVideos(secUID string, limit, offset int) ([]DyVideo, int, error) {
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM dy_videos WHERE sec_uid = ?`, secUID).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT `+dyVideoCols+` FROM dy_videos WHERE sec_uid = ?
		ORDER BY is_top DESC, create_time DESC, id DESC LIMIT ? OFFSET ?`, secUID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []DyVideo{}
	for rows.Next() {
		v, err := scanDyVideo(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

// GetDyVideosByIDs 取指定作品（深查后回读用）
func (s *Store) GetDyVideosByIDs(secUID string, ids []string) ([]DyVideo, error) {
	out := []DyVideo{}
	if len(ids) == 0 {
		return out, nil
	}
	args := []any{secUID}
	ph := []string{}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			continue
		}
		args = append(args, id)
		ph = append(ph, "?")
	}
	if len(ph) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(`SELECT `+dyVideoCols+` FROM dy_videos WHERE sec_uid = ? AND aweme_id IN (`+
		strings.Join(ph, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		v, err := scanDyVideo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// UpdateDyVideoStats 深查结果落库（播放量等累计统计）
func (s *Store) UpdateDyVideoStats(secUID, awemeID string, digg, comment, share, collect int64,
	play *int64, rate *float64) error {
	_, err := s.db.Exec(`UPDATE dy_videos SET digg_count = ?, comment_count = ?, share_count = ?,
		collect_count = ?, play_count = ?, interact_rate = ?, stats_updated_at = ?
		WHERE sec_uid = ? AND aweme_id = ?`,
		digg, comment, share, collect, play, rate, time.Now().Unix(), secUID, awemeID)
	return err
}
