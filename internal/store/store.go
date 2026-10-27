package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store 封装 SQLite 访问
type Store struct {
	db       *sql.DB
	mu       sync.RWMutex
	settings Settings
}

// Open 打开（或创建）数据库并执行迁移
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("创建数据目录失败: %w", err)
		}
	}
	dsn := path + "?_pragma=busy_timeout(8000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite 单写者，连接数不宜过大
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(time.Hour)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	if err := s.loadSettings(); err != nil {
		return nil, err
	}
	if err := s.seed(); err != nil {
		return nil, err
	}
	return s, nil
}

// DB 暴露底层连接（供少数场景使用）
func (s *Store) DB() *sql.DB { return s.db }

// Close 关闭数据库
func (s *Store) Close() error { return s.db.Close() }

const schema = `
CREATE TABLE IF NOT EXISTS categories (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  slug TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  icon TEXT NOT NULL DEFAULT '',
  sort INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS interfaces (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  slug TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  category TEXT NOT NULL DEFAULT '',
  method TEXT NOT NULL DEFAULT 'POST',
  path TEXT NOT NULL DEFAULT '',
  upstream_path TEXT NOT NULL DEFAULT '',
  upstream_method TEXT NOT NULL DEFAULT '',
  summary TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  params TEXT NOT NULL DEFAULT '[]',
  body_example TEXT NOT NULL DEFAULT '',
  response_example TEXT NOT NULL DEFAULT '',
  pricing_mode TEXT NOT NULL DEFAULT 'free',
  price REAL NOT NULL DEFAULT 0,
  price_unit TEXT NOT NULL DEFAULT '次',
  free_quota INTEGER NOT NULL DEFAULT 0,
  requires_login INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'online',
  latency_ms INTEGER NOT NULL DEFAULT 0,
  popularity INTEGER NOT NULL DEFAULT 0,
  tags TEXT NOT NULL DEFAULT '',
  sort INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_iface_cat ON interfaces(category);
CREATE INDEX IF NOT EXISTS idx_iface_status ON interfaces(status);

CREATE TABLE IF NOT EXISTS settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL UNIQUE,
  pass_hash TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'user',
  status TEXT NOT NULL DEFAULT 'active',
  note TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  last_login INTEGER
);

CREATE TABLE IF NOT EXISTS sessions (
  token TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL,
  ip TEXT NOT NULL DEFAULT '',
  ua TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);

CREATE TABLE IF NOT EXISTS voices (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  owner TEXT NOT NULL DEFAULT '',
  mime TEXT NOT NULL DEFAULT 'audio/wav',
  size INTEGER NOT NULL DEFAULT 0,
  duration_ms INTEGER NOT NULL DEFAULT 0,
  source TEXT NOT NULL DEFAULT 'upload',
  note TEXT NOT NULL DEFAULT '',
  audio BLOB,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_voices_owner ON voices(owner);

CREATE TABLE IF NOT EXISTS renders (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  owner TEXT NOT NULL DEFAULT '',
  voice_id INTEGER NOT NULL DEFAULT 0,
  voice_name TEXT NOT NULL DEFAULT '',
  text TEXT NOT NULL DEFAULT '',
  style TEXT NOT NULL DEFAULT '',
  format TEXT NOT NULL DEFAULT 'wav',
  size INTEGER NOT NULL DEFAULT 0,
  duration_ms INTEGER NOT NULL DEFAULT 0,
  audio BLOB,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_renders_owner ON renders(owner);

CREATE TABLE IF NOT EXISTS ad_slots (
  slot TEXT PRIMARY KEY,
  name TEXT NOT NULL DEFAULT '',
  html TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  ip TEXT NOT NULL DEFAULT '',
  method TEXT NOT NULL DEFAULT '',
  path TEXT NOT NULL DEFAULT '',
  status INTEGER NOT NULL DEFAULT 0,
  ms INTEGER NOT NULL DEFAULT 0,
  kind TEXT NOT NULL DEFAULT '',
  ua TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_logs_ts ON logs(ts);

CREATE TABLE IF NOT EXISTS counters (
  key TEXT PRIMARY KEY,
  val INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS api_keys (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  name TEXT NOT NULL DEFAULT '',
  prefix TEXT NOT NULL DEFAULT '',
  hash TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL DEFAULT 'active',
  quota INTEGER NOT NULL DEFAULT 0,
  used_today INTEGER NOT NULL DEFAULT 0,
  used_total INTEGER NOT NULL DEFAULT 0,
  day TEXT NOT NULL DEFAULT '',
  last_used_at INTEGER,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_keys_user ON api_keys(user_id);
CREATE INDEX IF NOT EXISTS idx_keys_hash ON api_keys(hash);

CREATE TABLE IF NOT EXISTS usage (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  key_id INTEGER NOT NULL DEFAULT 0,
  user_id INTEGER NOT NULL DEFAULT 0,
  username TEXT NOT NULL DEFAULT '',
  slug TEXT NOT NULL DEFAULT '',
  path TEXT NOT NULL DEFAULT '',
  method TEXT NOT NULL DEFAULT '',
  status INTEGER NOT NULL DEFAULT 0,
  ms INTEGER NOT NULL DEFAULT 0,
  ip TEXT NOT NULL DEFAULT '',
  stream INTEGER NOT NULL DEFAULT 0,
  ts INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_usage_ts ON usage(ts);
CREATE INDEX IF NOT EXISTS idx_usage_user ON usage(user_id);
CREATE INDEX IF NOT EXISTS idx_usage_key ON usage(key_id);
`

func (s *Store) migrate() error {
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("初始化表结构失败: %w", err)
	}
	// 老库补列（SQLite 不支持 IF NOT EXISTS 加列，忽略重复错误即可）
	for _, col := range []struct{ table, name, def string }{
		{"interfaces", "upstream_path", "TEXT NOT NULL DEFAULT ''"},
		{"interfaces", "upstream_method", "TEXT NOT NULL DEFAULT ''"},
		// 上游计费状态快照：由「从上游同步」写入，与本地计费设置（pricing_mode/price/price_unit）互不干扰
		{"interfaces", "upstream_pricing", "TEXT NOT NULL DEFAULT ''"},
		{"interfaces", "upstream_price", "REAL NOT NULL DEFAULT 0"},
		{"interfaces", "upstream_price_unit", "TEXT NOT NULL DEFAULT ''"},
		// 首次登录强制改密标记：使用内置默认密码的管理员账号为 1
		{"users", "must_change_pass", "INTEGER NOT NULL DEFAULT 0"},
	} {
		_, _ = s.db.Exec(`ALTER TABLE ` + col.table + ` ADD COLUMN ` + col.name + ` ` + col.def)
	}
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_iface_path ON interfaces(path)`)
	return nil
}

// ---------- settings ----------

func (s *Store) loadSettings() error {
	var raw string
	err := s.db.QueryRow(`SELECT json FROM settings WHERE id = 1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		s.settings = DefaultSettings()
		return s.persistSettings()
	}
	if err != nil {
		return err
	}
	cfg := DefaultSettings()
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		// 配置损坏时回落到默认值，避免整个服务起不来
		cfg = DefaultSettings()
	}
	// 补齐可能缺失的字段
	def := DefaultSettings()
	if cfg.TTS.MaxUploadMB <= 0 {
		cfg.TTS.MaxUploadMB = def.TTS.MaxUploadMB
	}
	if cfg.TTS.MaxChars <= 0 {
		cfg.TTS.MaxChars = def.TTS.MaxChars
	}
	if cfg.TTS.TimeoutSec <= 0 {
		cfg.TTS.TimeoutSec = def.TTS.TimeoutSec
	}
	if cfg.TTS.BaseURL == "" {
		cfg.TTS.BaseURL = def.TTS.BaseURL
	}
	if cfg.TTS.Model == "" {
		cfg.TTS.Model = def.TTS.Model
	}
	if cfg.RateLimitPerMin <= 0 {
		cfg.RateLimitPerMin = def.RateLimitPerMin
	}
	if cfg.RateLimitBurst <= 0 {
		cfg.RateLimitBurst = def.RateLimitBurst
	}
	if cfg.MaxBodyMB <= 0 {
		cfg.MaxBodyMB = def.MaxBodyMB
	}
	s.settings = cfg
	return nil
}

func (s *Store) persistSettings() error {
	raw, err := json.Marshal(s.settings)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO settings(id, json) VALUES(1, ?)
		ON CONFLICT(id) DO UPDATE SET json = excluded.json`, string(raw))
	return err
}

// Settings 读取配置副本
func (s *Store) Settings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

// SaveSettings 保存配置
func (s *Store) SaveSettings(cfg Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = cfg
	return s.persistSettings()
}

// ---------- counters ----------

// IncrCounter 自增计数器
func (s *Store) IncrCounter(key string, delta int64) error {
	_, err := s.db.Exec(`INSERT INTO counters(key, val) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET val = val + excluded.val`, key, delta)
	return err
}

// Counter 读取计数器
func (s *Store) Counter(key string) int64 {
	var v int64
	_ = s.db.QueryRow(`SELECT val FROM counters WHERE key = ?`, key).Scan(&v)
	return v
}

// Counters 读取全部计数器
func (s *Store) Counters() map[string]int64 {
	out := map[string]int64{}
	rows, err := s.db.Query(`SELECT key, val FROM counters`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var v int64
		if rows.Scan(&k, &v) == nil {
			out[k] = v
		}
	}
	return out
}

func splitTags(s string) []string {
	if strings.TrimSpace(s) == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
