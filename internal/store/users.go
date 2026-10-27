package store

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// ---------- users ----------

// CreateUser 创建用户
func (s *Store) CreateUser(username, passHash, role, note string) (User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return User{}, errors.New("用户名不能为空")
	}
	now := time.Now()
	res, err := s.db.Exec(`INSERT INTO users(username, pass_hash, role, status, note, created_at)
		VALUES(?,?,?,'active',?,?)`, username, passHash, role, note, now.Unix())
	if err != nil {
		return User{}, err
	}
	id, _ := res.LastInsertId()
	return User{ID: id, Username: username, Role: role, Status: "active", Note: note, CreatedAt: now}, nil
}

// GetUserByName 按用户名查询（含密码哈希）
func (s *Store) GetUserByName(username string) (User, string, error) {
	var u User
	var hash string
	var created int64
	var last sql.NullInt64
	var mustChange int
	err := s.db.QueryRow(`SELECT id, username, pass_hash, role, status, note, created_at, last_login, must_change_pass
		FROM users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &hash, &u.Role, &u.Status, &u.Note, &created, &last, &mustChange)
	if errors.Is(err, sql.ErrNoRows) {
		return u, "", ErrNotFound
	}
	if err != nil {
		return u, "", err
	}
	u.MustChangePass = mustChange != 0
	u.CreatedAt = time.Unix(created, 0)
	if last.Valid {
		t := time.Unix(last.Int64, 0)
		u.LastLogin = &t
	}
	return u, hash, nil
}

// GetUserByID 按 ID 查询
func (s *Store) GetUserByID(id int64) (User, error) {
	var u User
	var created int64
	var last sql.NullInt64
	var mustChange int
	err := s.db.QueryRow(`SELECT id, username, role, status, note, created_at, last_login, must_change_pass
		FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Username, &u.Role, &u.Status, &u.Note, &created, &last, &mustChange)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	if err != nil {
		return u, err
	}
	u.MustChangePass = mustChange != 0
	u.CreatedAt = time.Unix(created, 0)
	if last.Valid {
		t := time.Unix(last.Int64, 0)
		u.LastLogin = &t
	}
	return u, nil
}

// ListUsers 列出用户
func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(`SELECT id, username, role, status, note, created_at, last_login, must_change_pass
		FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		var created int64
		var last sql.NullInt64
		var mustChange int
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.Status, &u.Note, &created, &last, &mustChange); err != nil {
			return nil, err
		}
		u.MustChangePass = mustChange != 0
		u.CreatedAt = time.Unix(created, 0)
		if last.Valid {
			t := time.Unix(last.Int64, 0)
			u.LastLogin = &t
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UpdateUser 更新用户资料
func (s *Store) UpdateUser(id int64, passHash, role, status, note string) error {
	sets := []string{}
	args := []any{}
	if passHash != "" {
		sets = append(sets, "pass_hash = ?")
		args = append(args, passHash)
	}
	if role != "" {
		sets = append(sets, "role = ?")
		args = append(args, role)
	}
	if status != "" {
		sets = append(sets, "status = ?")
		args = append(args, status)
	}
	if note != "" {
		sets = append(sets, "note = ?")
		args = append(args, note)
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, id)
	_, err := s.db.Exec(`UPDATE users SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	return err
}

// DeleteUser 删除用户
func (s *Store) DeleteUser(id int64) error {
	_, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	return err
}

// TouchLogin 记录登录时间
func (s *Store) TouchLogin(id int64) {
	_, _ = s.db.Exec(`UPDATE users SET last_login = ? WHERE id = ?`, time.Now().Unix(), id)
}

// CountUsers 用户总数
func (s *Store) CountUsers() int {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(1) FROM users`).Scan(&n)
	return n
}

// SetMustChangePass 设置/清除「必须修改密码」标记
func (s *Store) SetMustChangePass(id int64, must bool) error {
	v := 0
	if must {
		v = 1
	}
	_, err := s.db.Exec(`UPDATE users SET must_change_pass = ? WHERE id = ?`, v, id)
	return err
}

// ---------- sessions ----------

// NewSession 创建会话
func (s *Store) NewSession(userID int64, ip, ua string, ttl time.Duration) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf)
	now := time.Now()
	_, err := s.db.Exec(`INSERT INTO sessions(token, user_id, ip, ua, created_at, expires_at)
		VALUES(?,?,?,?,?,?)`, token, userID, ip, truncate(ua, 200), now.Unix(), now.Add(ttl).Unix())
	return token, err
}

// SessionUser 校验会话并返回用户
func (s *Store) SessionUser(token string) (User, bool) {
	if token == "" {
		return User{}, false
	}
	var userID int64
	var expires int64
	err := s.db.QueryRow(`SELECT user_id, expires_at FROM sessions WHERE token = ?`, token).
		Scan(&userID, &expires)
	if err != nil || time.Now().Unix() > expires {
		if err == nil {
			_, _ = s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
		}
		return User{}, false
	}
	u, err := s.GetUserByID(userID)
	if err != nil || u.Status != "active" {
		return User{}, false
	}
	return u, true
}

// DeleteSession 注销会话
func (s *Store) DeleteSession(token string) {
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
}

// PurgeSessions 清理过期会话
func (s *Store) PurgeSessions() {
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().Unix())
}

// ---------- voices ----------

// CreateVoice 保存音色样本
func (s *Store) CreateVoice(v Voice, audio []byte) (Voice, error) {
	now := time.Now()
	res, err := s.db.Exec(`INSERT INTO voices(name, owner, mime, size, duration_ms, source, note, audio, created_at)
		VALUES(?,?,?,?,?,?,?,?,?)`, v.Name, v.Owner, v.Mime, len(audio), v.DurationMs, v.Source, v.Note, audio, now.Unix())
	if err != nil {
		return v, err
	}
	v.ID, _ = res.LastInsertId()
	v.Size = int64(len(audio))
	v.CreatedAt = now
	v.HasAudio = true
	return v, nil
}

// ListVoices 列出音色（owner 为空表示全部）
func (s *Store) ListVoices(owner string, limit int) ([]Voice, error) {
	q := `SELECT id, name, owner, mime, size, duration_ms, source, note, created_at,
		CASE WHEN audio IS NULL OR length(audio) = 0 THEN 0 ELSE 1 END FROM voices`
	args := []any{}
	if owner != "" {
		q += ` WHERE owner = ?`
		args = append(args, owner)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	if limit <= 0 {
		limit = 300
	}
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Voice{}
	for rows.Next() {
		var v Voice
		var created int64
		var has int
		if err := rows.Scan(&v.ID, &v.Name, &v.Owner, &v.Mime, &v.Size, &v.DurationMs,
			&v.Source, &v.Note, &created, &has); err != nil {
			return nil, err
		}
		v.CreatedAt = time.Unix(created, 0)
		v.HasAudio = has == 1
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetVoice 获取音色元信息
func (s *Store) GetVoice(id int64) (Voice, error) {
	var v Voice
	var created int64
	var has int
	err := s.db.QueryRow(`SELECT id, name, owner, mime, size, duration_ms, source, note, created_at,
		CASE WHEN audio IS NULL OR length(audio) = 0 THEN 0 ELSE 1 END FROM voices WHERE id = ?`, id).
		Scan(&v.ID, &v.Name, &v.Owner, &v.Mime, &v.Size, &v.DurationMs, &v.Source, &v.Note, &created, &has)
	if errors.Is(err, sql.ErrNoRows) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	v.CreatedAt = time.Unix(created, 0)
	v.HasAudio = has == 1
	return v, nil
}

// VoiceAudio 读取音色原始音频
func (s *Store) VoiceAudio(id int64) ([]byte, string, error) {
	var data []byte
	var mime string
	err := s.db.QueryRow(`SELECT audio, mime FROM voices WHERE id = ?`, id).Scan(&data, &mime)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	return data, mime, err
}

// RenameVoice 重命名音色
func (s *Store) RenameVoice(id int64, name string) error {
	_, err := s.db.Exec(`UPDATE voices SET name = ? WHERE id = ?`, name, id)
	return err
}

// DeleteVoices 批量删除音色
func (s *Store) DeleteVoices(ids []int64, owner string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var total int64
	for _, id := range ids {
		q := `DELETE FROM voices WHERE id = ?`
		args := []any{id}
		if owner != "" {
			q += ` AND owner = ?`
			args = append(args, owner)
		}
		res, err := s.db.Exec(q, args...)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// ---------- renders ----------

// CreateRender 保存合成结果
func (s *Store) CreateRender(r Render, audio []byte) (Render, error) {
	now := time.Now()
	res, err := s.db.Exec(`INSERT INTO renders(owner, voice_id, voice_name, text, style, format, size, duration_ms, audio, created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, r.Owner, r.VoiceID, r.VoiceName, r.Text, r.Style,
		r.Format, len(audio), r.DurationMs, audio, now.Unix())
	if err != nil {
		return r, err
	}
	r.ID, _ = res.LastInsertId()
	r.Size = int64(len(audio))
	r.CreatedAt = now
	return r, nil
}

// ListRenders 列出合成记录
func (s *Store) ListRenders(owner string, limit int) ([]Render, error) {
	q := `SELECT id, owner, voice_id, voice_name, text, style, format, size, duration_ms, created_at
		FROM renders`
	args := []any{}
	if owner != "" {
		q += ` WHERE owner = ?`
		args = append(args, owner)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	if limit <= 0 {
		limit = 100
	}
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Render{}
	for rows.Next() {
		var r Render
		var created int64
		if err := rows.Scan(&r.ID, &r.Owner, &r.VoiceID, &r.VoiceName, &r.Text, &r.Style,
			&r.Format, &r.Size, &r.DurationMs, &created); err != nil {
			return nil, err
		}
		r.CreatedAt = time.Unix(created, 0)
		out = append(out, r)
	}
	return out, rows.Err()
}

// RenderAudio 读取合成音频
func (s *Store) RenderAudio(id int64) ([]byte, string, string, error) {
	var data []byte
	var format, text string
	err := s.db.QueryRow(`SELECT audio, format, text FROM renders WHERE id = ?`, id).Scan(&data, &format, &text)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", "", ErrNotFound
	}
	return data, format, text, err
}

// DeleteRenders 批量删除合成记录
func (s *Store) DeleteRenders(ids []int64, owner string) (int64, error) {
	var total int64
	for _, id := range ids {
		q := `DELETE FROM renders WHERE id = ?`
		args := []any{id}
		if owner != "" {
			q += ` AND owner = ?`
			args = append(args, owner)
		}
		res, err := s.db.Exec(q, args...)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// ---------- ad slots ----------

// AdSlots 返回全部广告位
func (s *Store) AdSlots() ([]AdSlot, error) {
	rows, err := s.db.Query(`SELECT slot, name, html, enabled FROM ad_slots ORDER BY slot`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdSlot{}
	for rows.Next() {
		var a AdSlot
		var en int
		if err := rows.Scan(&a.Slot, &a.Name, &a.HTML, &en); err != nil {
			return nil, err
		}
		a.Enabled = en == 1
		out = append(out, a)
	}
	return out, rows.Err()
}

// SaveAdSlot 保存广告位
func (s *Store) SaveAdSlot(a AdSlot) error {
	_, err := s.db.Exec(`INSERT INTO ad_slots(slot, name, html, enabled) VALUES(?,?,?,?)
		ON CONFLICT(slot) DO UPDATE SET name = excluded.name, html = excluded.html, enabled = excluded.enabled`,
		a.Slot, a.Name, a.HTML, boolInt(a.Enabled))
	return err
}

// PublicAds 返回启用的广告位
func (s *Store) PublicAds() map[string]string {
	out := map[string]string{}
	rows, err := s.db.Query(`SELECT slot, html FROM ad_slots WHERE enabled = 1 AND html <> ''`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) == nil {
			out[k] = v
		}
	}
	return out
}

// ---------- logs ----------

// AddLog 写入访问日志
func (s *Store) AddLog(e LogEntry) {
	_, _ = s.db.Exec(`INSERT INTO logs(ts, ip, method, path, status, ms, kind, ua)
		VALUES(?,?,?,?,?,?,?,?)`, time.Now().Unix(), e.IP, e.Method, truncate(e.Path, 300),
		e.Status, e.Ms, e.Kind, truncate(e.UA, 200))
}

// ListLogs 分页读取日志
func (s *Store) ListLogs(limit, offset int) ([]LogEntry, int, error) {
	var total int
	_ = s.db.QueryRow(`SELECT COUNT(1) FROM logs`).Scan(&total)
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id, ts, ip, method, path, status, ms, kind, ua
		FROM logs ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []LogEntry{}
	for rows.Next() {
		var e LogEntry
		var ts int64
		if err := rows.Scan(&e.ID, &ts, &e.IP, &e.Method, &e.Path, &e.Status, &e.Ms, &e.Kind, &e.UA); err != nil {
			return nil, 0, err
		}
		e.TS = time.Unix(ts, 0)
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// TrimLogs 只保留最近 n 条
func (s *Store) TrimLogs(n int) {
	_, _ = s.db.Exec(`DELETE FROM logs WHERE id NOT IN (SELECT id FROM logs ORDER BY id DESC LIMIT ?)`, n)
}

// PurgeOldRenders 清理过期合成记录
func (s *Store) PurgeOldRenders(days int) {
	if days <= 0 {
		return
	}
	cut := time.Now().AddDate(0, 0, -days).Unix()
	_, _ = s.db.Exec(`DELETE FROM renders WHERE created_at < ?`, cut)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// ConstantTimeEqual 固定时间比较，避免时序侧信道
func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
