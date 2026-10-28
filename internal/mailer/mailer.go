// Package mailer 提供基于 net/smtp 的邮件发送，支持 SSL(465)、STARTTLS(587) 与明文三种模式，
// 覆盖常见的国内邮箱服务商（QQ/163/企业邮等），不引入第三方依赖。
package mailer

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Config SMTP 发信配置（来自后台「全局设置 → 邮件服务」）
type Config struct {
	Host string // SMTP 服务器，如 smtp.qq.com
	Port int    // 465(SSL) / 587(STARTTLS) / 25(none)
	User string // 登录账号，一般为邮箱地址
	Pass string // 登录密码 / 授权码
	From string // 发件人显示地址，留空取 User
	TLS  string // ssl | starttls | none
}

// Enabled 是否已配置完整
func (c Config) Enabled() bool {
	return strings.TrimSpace(c.Host) != "" && c.Port > 0 && strings.TrimSpace(c.User) != ""
}

func (c Config) from() string {
	if f := strings.TrimSpace(c.From); f != "" {
		return f
	}
	return strings.TrimSpace(c.User)
}

func (c Config) mode() string {
	switch strings.ToLower(strings.TrimSpace(c.TLS)) {
	case "starttls":
		return "starttls"
	case "none":
		return "none"
	default:
		return "ssl"
	}
}

// addr 返回 host:port
func (c Config) addr() string {
	return fmt.Sprintf("%s:%d", strings.TrimSuffix(c.Host, ":"), c.Port)
}

// Send 发送一封 HTML 邮件
func Send(cfg Config, to, subject, htmlBody string) error {
	if !cfg.Enabled() {
		return fmt.Errorf("SMTP 未配置：请在后台「全局设置 → 邮件服务」填写服务器与账号")
	}
	from := cfg.from()
	to = strings.TrimSpace(to)
	if to == "" {
		return fmt.Errorf("收件邮箱不能为空")
	}

	headers := map[string]string{
		"From":         fmt.Sprintf("ClearFrame <%s>", from),
		"To":           to,
		"Subject":      subject,
		"MIME-Version": "1.0",
		"Content-Type": `text/html; charset="utf-8"`,
		"Date":         time.Now().Format(time.RFC1123Z),
	}
	var msg strings.Builder
	for k, v := range headers {
		fmt.Fprintf(&msg, "%s: %s\r\n", k, v)
	}
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)
	body := []byte(msg.String())

	host, _, _ := net.SplitHostPort(cfg.addr())
	mode := cfg.mode()
	auth := smtp.PlainAuth("", cfg.User, cfg.Pass, host)
	timeout := 15 * time.Second

	if mode == "ssl" {
		conn, err := tls.Dial("tcp", cfg.addr(), &tls.Config{ServerName: host})
		if err != nil {
			return fmt.Errorf("SMTP 连接失败: %w", err)
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(timeout))
		cli, err := smtp.NewClient(conn, host)
		if err != nil {
			return fmt.Errorf("SMTP 会话失败: %w", err)
		}
		defer cli.Close()
		if ok, _ := cli.Extension("AUTH"); ok {
			if err = cli.Auth(auth); err != nil {
				return fmt.Errorf("SMTP 认证失败（请检查账号与授权码）: %w", err)
			}
		}
		if err = cli.Mail(from); err != nil {
			return fmt.Errorf("SMTP MAIL FROM 失败: %w", err)
		}
		if err = cli.Rcpt(to); err != nil {
			return fmt.Errorf("SMTP RCPT TO 失败: %w", err)
		}
		w, err := cli.Data()
		if err != nil {
			return fmt.Errorf("SMTP DATA 失败: %w", err)
		}
		if _, err = w.Write(body); err != nil {
			return fmt.Errorf("SMTP 写入失败: %w", err)
		}
		if err = w.Close(); err != nil {
			return fmt.Errorf("SMTP 发送失败: %w", err)
		}
		return cli.Quit()
	}

	conn, err := net.DialTimeout("tcp", cfg.addr(), timeout)
	if err != nil {
		return fmt.Errorf("SMTP 连接失败: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))
	cli, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("SMTP 会话失败: %w", err)
	}
	defer cli.Close()
	if mode == "starttls" {
		if ok, _ := cli.Extension("STARTTLS"); !ok {
			return fmt.Errorf("服务器不支持 STARTTLS，请把加密方式改为 SSL 或不加密")
		}
		if err = cli.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return fmt.Errorf("STARTTLS 失败: %w", err)
		}
	}
	if ok, _ := cli.Extension("AUTH"); ok {
		if err = cli.Auth(auth); err != nil {
			return fmt.Errorf("SMTP 认证失败（请检查账号与授权码）: %w", err)
		}
	}
	if err = cli.Mail(from); err != nil {
		return fmt.Errorf("SMTP MAIL FROM 失败: %w", err)
	}
	if err = cli.Rcpt(to); err != nil {
		return fmt.Errorf("SMTP RCPT TO 失败: %w", err)
	}
	w, err := cli.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA 失败: %w", err)
	}
	if _, err = w.Write(body); err != nil {
		return fmt.Errorf("SMTP 写入失败: %w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("SMTP 发送失败: %w", err)
	}
	return cli.Quit()
}

// CodeHTML 生成验证码邮件正文
func CodeHTML(siteName, code, purpose string, minutes int) string {
	return fmt.Sprintf(`<!DOCTYPE html><html lang="zh-CN"><body style="margin:0;padding:24px;background:#f7f5f0;font-family:-apple-system,'PingFang SC','Microsoft YaHei',sans-serif;color:#1b1814">
<div style="max-width:520px;margin:0 auto;background:#fffefb;border:1px solid rgba(27,24,20,0.12);border-radius:12px;padding:28px">
<div style="font-size:13px;letter-spacing:0.16em;color:#8f887b;text-transform:uppercase;margin-bottom:14px">%s</div>
<h2 style="margin:0 0 10px;font-size:20px">%s验证码</h2>
<p style="margin:0 0 18px;font-size:14px;line-height:1.7;color:#5d574d">请在 <b>%d 分钟内</b> 输入以下验证码完成操作。验证码单次有效，若非本人操作请忽略本邮件。</p>
<div style="font-family:ui-monospace,Consolas,monospace;font-size:32px;font-weight:700;letter-spacing:0.3em;background:rgba(194,65,12,0.07);border:1px solid rgba(194,65,12,0.3);border-radius:10px;padding:16px;text-align:center;color:#c2410c">%s</div>
<p style="margin:18px 0 0;font-size:12px;color:#8f887b;line-height:1.7">本邮件由系统自动发送，请勿回复。</p>
</div></body></html>`, siteName, purpose, minutes, code)
}
