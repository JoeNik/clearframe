package store

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"
)

// 邮件验证码：找回密码（reset）与绑定邮箱（bind）共用。
// 规则：6 位数字，10 分钟有效，单次使用；同一邮箱同一用途 60 秒内只允许发一次，
// 新码生成时作废旧码，避免多码并存导致误用。

const (
	EmailCodeTTL      = 10 * time.Minute
	EmailCodeCooldown = 60 * time.Second
)

var (
	ErrEmailCodeCooldown = errors.New("发送过于频繁，请 1 分钟后再试")
	ErrEmailCodeInvalid  = errors.New("验证码错误或已过期")
)

// NewEmailCode 生成并落库一个验证码，返回明文（调用方负责通过邮件下发）
func (s *Store) NewEmailCode(email, purpose string) (string, error) {
	email = normalizeEmail(email)
	if email == "" {
		return "", errors.New("邮箱不能为空")
	}
	now := time.Now()
	var lastCreated int64
	err := s.db.QueryRow(`SELECT created_at FROM email_codes
		WHERE email = ? AND purpose = ? ORDER BY id DESC LIMIT 1`, email, purpose).Scan(&lastCreated)
	if err == nil && now.Unix()-lastCreated < int64(EmailCodeCooldown.Seconds()) {
		return "", ErrEmailCodeCooldown
	}
	// 生成 6 位数字码（crypto/rand，取模偏置对 6 位验证码可忽略）
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	n := int(buf[0])<<24 | int(buf[1])<<16 | int(buf[2])<<8 | int(buf[3])
	code := fmt.Sprintf("%06d", n%1000000)
	// 作废同邮箱同用途的旧未用码
	_, _ = s.db.Exec(`UPDATE email_codes SET used = 1 WHERE email = ? AND purpose = ? AND used = 0`, email, purpose)
	_, err = s.db.Exec(`INSERT INTO email_codes(email, purpose, code, expires_at, used, created_at)
		VALUES(?,?,?,?,0,?)`, email, purpose, code, now.Add(EmailCodeTTL).Unix(), now.Unix())
	if err != nil {
		return "", err
	}
	return code, nil
}

// VerifyEmailCode 校验并消费验证码（单次有效）
func (s *Store) VerifyEmailCode(email, purpose, code string) error {
	email = normalizeEmail(email)
	if email == "" || code == "" {
		return ErrEmailCodeInvalid
	}
	res, err := s.db.Exec(`UPDATE email_codes SET used = 1
		WHERE email = ? AND purpose = ? AND code = ? AND used = 0 AND expires_at > ?`,
		email, purpose, code, time.Now().Unix())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrEmailCodeInvalid
	}
	return nil
}

// PurgeEmailCodes 清理过期验证码（挂到后台清理任务）
func (s *Store) PurgeEmailCodes() {
	_, _ = s.db.Exec(`DELETE FROM email_codes WHERE expires_at < ? OR used = 1`, time.Now().Add(-time.Hour).Unix())
}

func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
