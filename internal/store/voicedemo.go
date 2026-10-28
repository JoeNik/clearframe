package store

import "time"

// 参考音色试听音频缓存：首次播放时合成，之后直接回缓存。

// GetVoiceDemo 读取试听音频，未命中返回 false
func (s *Store) GetVoiceDemo(key string) ([]byte, string, bool) {
	var audio []byte
	var mime string
	err := s.db.QueryRow(`SELECT audio, mime FROM voice_demos WHERE key = ?`, key).Scan(&audio, &mime)
	if err != nil || len(audio) == 0 {
		return nil, "", false
	}
	return audio, mime, true
}

// SaveVoiceDemo 保存试听音频
func (s *Store) SaveVoiceDemo(key string, audio []byte, mime string) {
	_, _ = s.db.Exec(`INSERT INTO voice_demos(key, audio, mime, created_at) VALUES(?,?,?,?)
		ON CONFLICT(key) DO UPDATE SET audio = excluded.audio, mime = excluded.mime, created_at = excluded.created_at`,
		key, audio, mime, time.Now().Unix())
}

// HasVoiceDemo 是否已缓存试听
func (s *Store) HasVoiceDemo(key string) bool {
	var n int
	_ = s.db.QueryRow(`SELECT 1 FROM voice_demos WHERE key = ?`, key).Scan(&n)
	return n > 0
}
