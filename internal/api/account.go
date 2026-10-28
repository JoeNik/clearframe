package api

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"jxivideo/internal/auth"
	"jxivideo/internal/mailer"
	"jxivideo/internal/store"
)

// 邮箱验证码：忘记密码（reset）与个人中心绑定邮箱（bind）共用一套存储与频控。
// 找回密码走登录失败同一套 IP 封禁守卫，防止对 6 位码的暴力枚举。

var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func (s *Server) smtpConfig() mailer.Config {
	cfg := s.st.Settings()
	return mailer.Config{
		Host: cfg.SMTPHost, Port: cfg.SMTPPort,
		User: cfg.SMTPUser, Pass: cfg.SMTPPass,
		From: cfg.SMTPFrom, TLS: cfg.SMTPTLS,
	}
}

// maskEmail 收件人提示用：ab***@qq.com
func maskEmail(email string) string {
	at := strings.Index(email, "@")
	if at <= 0 {
		return "***"
	}
	local := email[:at]
	keep := 1
	if len(local) < keep {
		keep = len(local)
	}
	return local[:keep] + "***" + email[at:]
}

func validEmail(email string) bool {
	return emailRe.MatchString(strings.TrimSpace(email))
}

// sendEmailCode 生成验证码并发送；返回打码后的收件地址
func (s *Server) sendEmailCode(email, purpose string) (string, error) {
	cfg := s.st.Settings()
	if !s.smtpConfig().Enabled() {
		return "", errors.New("邮件服务未配置，请联系管理员在后台「全局设置 → 邮件服务」中填写")
	}
	code, err := s.st.NewEmailCode(email, purpose)
	if err != nil {
		return "", err
	}
	site := cfg.SiteName
	if site == "" {
		site = "ClearFrame"
	}
	subject := purpose
	if purpose == "reset" {
		subject = "找回密码"
	} else {
		subject = "绑定邮箱"
	}
	body := mailer.CodeHTML(site, code, subject, int(store.EmailCodeTTL.Minutes()))
	if err := mailer.Send(s.smtpConfig(), email, site+" · "+subject+"验证码", body); err != nil {
		return "", errors.New("邮件发送失败：" + err.Error())
	}
	return maskEmail(email), nil
}

// POST /api/auth/forgot-code  {username}
// 用户名对应的账号必须已绑定邮箱；验证码发到绑定邮箱
func (s *Server) handleForgotCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式有误")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "请输入用户名")
		return
	}
	u, _, err := s.st.GetUserByName(req.Username)
	if err != nil {
		// 不暴露用户名是否存在
		writeOK(w, map[string]any{"sent": true, "masked": "***"})
		return
	}
	if strings.TrimSpace(u.Email) == "" {
		writeErr(w, http.StatusBadRequest, "no_email", "该账号还没有绑定邮箱，请联系管理员重置密码")
		return
	}
	if u.Status != "active" {
		writeErr(w, http.StatusForbidden, "disabled", "账号已被停用")
		return
	}
	masked, err := s.sendEmailCode(u.Email, "reset")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "send_failed", err.Error())
		return
	}
	writeOK(w, map[string]any{"sent": true, "masked": masked})
}

// POST /api/auth/reset-password  {username, code, password}
// 校验通过后重置密码并踢掉该账号全部会话
func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	ip := s.clientIP(r)
	cfg := s.st.Settings()
	ban := time.Duration(cfg.BanMinutes) * time.Minute
	if ban <= 0 {
		ban = 15 * time.Minute
	}
	if s.guard.blocked(ip) {
		writeErr(w, http.StatusTooManyRequests, "locked", "尝试次数过多，请稍后再试")
		return
	}
	var req struct {
		Username string `json:"username"`
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式有误")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Code = strings.TrimSpace(req.Code)
	if req.Username == "" || req.Code == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "请填写用户名与验证码")
		return
	}
	if len(req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "weak_password", "新密码至少 8 位")
		return
	}
	u, _, err := s.st.GetUserByName(req.Username)
	if err != nil || strings.TrimSpace(u.Email) == "" {
		s.guard.fail(ip, ban)
		writeErr(w, http.StatusBadRequest, "invalid", "账号不存在或未绑定邮箱")
		return
	}
	if u.Status != "active" {
		writeErr(w, http.StatusForbidden, "disabled", "账号已被停用")
		return
	}
	if err := s.st.VerifyEmailCode(u.Email, "reset", req.Code); err != nil {
		s.guard.fail(ip, ban)
		writeErr(w, http.StatusBadRequest, "invalid_code", err.Error())
		return
	}
	s.guard.reset(ip)
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash", "密码加密失败")
		return
	}
	if err := s.st.UpdateUser(u.ID, hash, "", "", ""); err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "密码保存失败")
		return
	}
	// 密码已重置：该账号全部登录态作废，需重新登录
	s.st.DeleteSessionsByUser(u.ID)
	writeOK(w, map[string]any{"ok": true})
}

// POST /api/account/email-code  {email}（需登录）
// 给当前登录用户发送绑定邮箱验证码
func (s *Server) handleAccountEmailCode(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "login_required", "请先登录")
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式有误")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !validEmail(req.Email) {
		writeErr(w, http.StatusBadRequest, "bad_request", "邮箱格式不正确")
		return
	}
	if taken, _ := s.st.EmailTaken(req.Email, u.ID); taken {
		writeErr(w, http.StatusConflict, "email_taken", "该邮箱已被其他账号绑定")
		return
	}
	masked, err := s.sendEmailCode(req.Email, "bind")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "send_failed", err.Error())
		return
	}
	writeOK(w, map[string]any{"sent": true, "masked": masked})
}

// POST /api/account/bind-email  {email, code}（需登录）
func (s *Server) handleBindEmail(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "login_required", "请先登录")
		return
	}
	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式有误")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Code = strings.TrimSpace(req.Code)
	if !validEmail(req.Email) || req.Code == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "请填写邮箱与验证码")
		return
	}
	if taken, _ := s.st.EmailTaken(req.Email, u.ID); taken {
		writeErr(w, http.StatusConflict, "email_taken", "该邮箱已被其他账号绑定")
		return
	}
	if err := s.st.VerifyEmailCode(req.Email, "bind", req.Code); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_code", err.Error())
		return
	}
	if err := s.st.SetUserEmail(u.ID, req.Email); err != nil {
		writeErr(w, http.StatusInternalServerError, "db", "保存失败")
		return
	}
	writeOK(w, map[string]any{"ok": true, "email": req.Email})
}

// POST /api/admin/email-test  {to}（管理员）
// 用当前保存的 SMTP 配置发一封测试邮件
func (s *Server) handleAdminEmailTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		To string `json:"to"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求格式有误")
		return
	}
	req.To = strings.ToLower(strings.TrimSpace(req.To))
	if !validEmail(req.To) {
		writeErr(w, http.StatusBadRequest, "bad_request", "请填写有效的测试收件邮箱")
		return
	}
	cfg := s.st.Settings()
	site := cfg.SiteName
	if site == "" {
		site = "ClearFrame"
	}
	body := mailer.CodeHTML(site, "TEST", "SMTP 配置测试", 1)
	if err := mailer.Send(s.smtpConfig(), req.To, site+" · SMTP 配置测试邮件", body); err != nil {
		writeErr(w, http.StatusBadRequest, "send_failed", err.Error())
		return
	}
	writeOK(w, map[string]any{"ok": true})
}
