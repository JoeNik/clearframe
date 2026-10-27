package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// ---------- API Keys ----------

const keyCols = `id, user_id, name, prefix, status, quota, used_today, used_total, last_used_at, created_at,
	COALESCE((SELECT username FROM users WHERE users.id = api_keys.user_id), '')`

func scanKey(sc interface{ Scan(...any) error }) (APIKey, error) {
	var k APIKey
	var lastUsed sql.NullInt64
	var created int64
	err := sc.Scan(&k.ID, &k.UserID, &k.Name, &k.Prefix, &k.Status, &k.Quota,
		&k.UsedToday, &k.UsedTotal, &lastUsed, &created, &k.Username)
	if err != nil {
		return k, err
	}
	if lastUsed.Valid {
		t := time.Unix(lastUsed.Int64, 0)
		k.LastUsedAt = &t
	}
	k.CreatedAt = time.Unix(created, 0)
	return k, nil
}

func hashKey(plain string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(plain)))
	return hex.EncodeToString(sum[:])
}

// NewKeyString 生成 sk- 开头的随机密钥
func NewKeyString() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "sk-" + hex.EncodeToString([]byte(time.Now().String()))[:40]
	}
	return "sk-" + hex.EncodeToString(b)
}

// CreateAPIKey 为用户签发新密钥，返回明文（仅此一次可见）
func (s *Store) CreateAPIKey(userID int64, name string, quota int) (APIKey, string, error) {
	if userID <= 0 {
		return APIKey{}, "", errors.New("用户不存在")
	}
	if strings.TrimSpace(name) == "" {
		name = "默认密钥"
	}
	plain := NewKeyString()
	prefix := plain
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	now := time.Now()
	res, err := s.db.Exec(`INSERT INTO api_keys(user_id, name, prefix, hash, status, quota, used_today,
		used_total, day, created_at) VALUES(?,?,?,?,'active',?,0,0,?,?)`,
		userID, name, prefix, hashKey(plain), quota, now.Format("2006-01-02"), now.Unix())
	if err != nil {
		return APIKey{}, "", err
	}
	id, _ := res.LastInsertId()
	k, err := s.GetAPIKeyByID(id)
	return k, plain, err
}

// GetAPIKeyByID 按 ID 查询
func (s *Store) GetAPIKeyByID(id int64) (APIKey, error) {
	row := s.db.QueryRow(`SELECT `+keyCols+` FROM api_keys WHERE id = ?`, id)
	k, err := scanKey(row)
	if errors.Is(err, sql.ErrNoRows) {
		return k, ErrNotFound
	}
	return k, err
}

// FindAPIKey 用明文密钥查找（网关鉴权）
func (s *Store) FindAPIKey(plain string) (APIKey, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return APIKey{}, ErrNotFound
	}
	row := s.db.QueryRow(`SELECT `+keyCols+` FROM api_keys WHERE hash = ?`, hashKey(plain))
	k, err := scanKey(row)
	if errors.Is(err, sql.ErrNoRows) {
		return k, ErrNotFound
	}
	return k, err
}

// ListAPIKeys 列出密钥。userID 为 0 时返回全部（后台用）
func (s *Store) ListAPIKeys(userID int64) ([]APIKey, error) {
	q := `SELECT ` + keyCols + ` FROM api_keys`
	args := []any{}
	if userID > 0 {
		q += ` WHERE user_id = ?`
		args = append(args, userID)
	}
	q += ` ORDER BY id DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIKey{}
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// UpdateAPIKey 更新名称/额度/状态。userID>0 时校验归属
func (s *Store) UpdateAPIKey(id, userID int64, name string, quota int, status string) error {
	sets := []string{}
	args := []any{}
	if name != "" {
		sets = append(sets, "name = ?")
		args = append(args, name)
	}
	if quota >= 0 {
		sets = append(sets, "quota = ?")
		args = append(args, quota)
	}
	if status == "active" || status == "disabled" {
		sets = append(sets, "status = ?")
		args = append(args, status)
	}
	if len(sets) == 0 {
		return errors.New("没有可更新的字段")
	}
	args = append(args, id)
	q := `UPDATE api_keys SET ` + strings.Join(sets, ", ") + ` WHERE id = ?`
	if userID > 0 {
		q += ` AND user_id = ?`
		args = append(args, userID)
	}
	_, err := s.db.Exec(q, args...)
	return err
}

// DeleteAPIKey 删除密钥。userID>0 时校验归属
func (s *Store) DeleteAPIKey(id, userID int64) error {
	q := `DELETE FROM api_keys WHERE id = ?`
	args := []any{id}
	if userID > 0 {
		q += ` AND user_id = ?`
		args = append(args, userID)
	}
	_, err := s.db.Exec(q, args...)
	return err
}

// TouchAPIKey 调用计数（跨天自动重置今日用量）
func (s *Store) TouchAPIKey(id int64) {
	day := time.Now().Format("2006-01-02")
	_, _ = s.db.Exec(`UPDATE api_keys SET used_total = used_total + 1,
		used_today = CASE WHEN day = ? THEN used_today + 1 ELSE 1 END,
		day = ?, last_used_at = ? WHERE id = ?`, day, day, time.Now().Unix(), id)
}

// ---------- Usage ----------

// RecordUsage 写入一条调用记录
func (s *Store) RecordUsage(u Usage) {
	_, _ = s.db.Exec(`INSERT INTO usage(key_id, user_id, username, slug, path, method, status, ms, ip, stream, ts)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		u.KeyID, u.UserID, u.Username, u.Slug, u.Path, u.Method, u.Status, u.Ms, u.IP, boolInt(u.Stream), time.Now().Unix())
}

// ListUsage 调用记录。userID 为 0 时返回全部
func (s *Store) ListUsage(userID int64, limit int) ([]Usage, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT id, key_id, user_id, username, slug, path, method, status, ms, ip, stream, ts FROM usage`
	args := []any{}
	if userID > 0 {
		q += ` WHERE user_id = ?`
		args = append(args, userID)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Usage{}
	for rows.Next() {
		var u Usage
		var stream, ts int64
		if err := rows.Scan(&u.ID, &u.KeyID, &u.UserID, &u.Username, &u.Slug, &u.Path,
			&u.Method, &u.Status, &u.Ms, &u.IP, &stream, &ts); err != nil {
			return nil, err
		}
		u.Stream = stream != 0
		u.TS = time.Unix(ts, 0)
		out = append(out, u)
	}
	return out, rows.Err()
}

// UsageSummary 用量汇总
type UsageSummary struct {
	Total   int64            `json:"total"`
	Today   int64            `json:"today"`
	Success int64            `json:"success"`
	Failed  int64            `json:"failed"`
	AvgMs   int64            `json:"avgMs"`
	ByDay   map[string]int64 `json:"byDay"`
	TopPath []PathCount      `json:"topPath"`
}

// PathCount 路径调用次数
type PathCount struct {
	Path  string `json:"path"`
	Count int64  `json:"count"`
}

// UsageStats 统计用量。userID 为 0 时统计全站
func (s *Store) UsageStats(userID int64) UsageSummary {
	sum := UsageSummary{ByDay: map[string]int64{}, TopPath: []PathCount{}}
	where := ""
	args := []any{}
	if userID > 0 {
		where = " WHERE user_id = ?"
		args = append(args, userID)
	}
	_ = s.db.QueryRow(`SELECT COUNT(1) FROM usage`+where, args...).Scan(&sum.Total)
	_ = s.db.QueryRow(`SELECT COUNT(1) FROM usage`+where+andWhere(where, "ts >= ?"),
		append(args, startOfToday())...).Scan(&sum.Today)
	_ = s.db.QueryRow(`SELECT COUNT(1) FROM usage`+where+andWhere(where, "status < 400"), args...).Scan(&sum.Success)
	_ = s.db.QueryRow(`SELECT COUNT(1) FROM usage`+where+andWhere(where, "status >= 400"), args...).Scan(&sum.Failed)
	_ = s.db.QueryRow(`SELECT COALESCE(CAST(AVG(ms) AS INTEGER),0) FROM usage`+where, args...).Scan(&sum.AvgMs)

	rows, err := s.db.Query(`SELECT path, COUNT(1) c FROM usage`+where+` GROUP BY path ORDER BY c DESC LIMIT 8`, args...)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var p PathCount
			if rows.Scan(&p.Path, &p.Count) == nil {
				sum.TopPath = append(sum.TopPath, p)
			}
		}
	}
	since := time.Now().AddDate(0, 0, -6)
	rows2, err := s.db.Query(`SELECT ts FROM usage`+where+andWhere(where, "ts >= ?"),
		append(args, since.Unix())...)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var ts int64
			if rows2.Scan(&ts) == nil {
				sum.ByDay[time.Unix(ts, 0).Format("01-02")]++
			}
		}
	}
	return sum
}

// PurgeOldUsage 清理旧调用记录
func (s *Store) PurgeOldUsage(days int) int64 {
	if days <= 0 {
		days = 30
	}
	cut := time.Now().AddDate(0, 0, -days).Unix()
	res, err := s.db.Exec(`DELETE FROM usage WHERE ts < ?`, cut)
	if err != nil {
		return 0
	}
	n, _ := res.RowsAffected()
	return n
}

func andWhere(where, cond string) string {
	if where == "" {
		return " WHERE " + cond
	}
	return " AND " + cond
}

func startOfToday() int64 {
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location()).Unix()
}
