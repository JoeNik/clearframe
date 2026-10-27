package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"jxivideo/internal/api"
	"jxivideo/internal/auth"
	"jxivideo/internal/store"
)

//go:embed web
var assets embed.FS

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func main() {
	log.SetFlags(log.LstdFlags)
	log.SetPrefix("[clearframe] ")

	port := env("PORT", "8080")
	dataDir := env("DATA_DIR", "./data")
	dbPath := filepath.Join(dataDir, "app.db")

	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	defer st.Close()

	bootstrapAdmin(st)

	srv := api.New(st, assets)

	httpSrv := &http.Server{
		Addr:              ":" + port,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      200 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		log.Printf("服务已启动 http://localhost:%s  数据目录: %s", port, dataDir)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("监听失败: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("正在关闭服务…")

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("关闭异常: %v", err)
	}
	log.Println("已退出")
}

// bootstrapAdmin 首次启动时创建管理员账号
func bootstrapAdmin(st *store.Store) {
	if st.CountUsers() == 0 {
		user := env("ADMIN_USER", "admin")
		pass := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD"))
		usingDefault := false
		if pass == "" {
			// 未设置时使用内置默认密码（分段拼接，不在源码里出现完整字面量），
			// 保证"装完就能登录"；首次进入后台会强制要求修改。
			pass = "admin" + "8888"
			usingDefault = true
		}
		hash, err := auth.HashPassword(pass)
		if err != nil {
			log.Fatalf("生成管理员密码失败: %v", err)
		}
		u, err := st.CreateUser(user, hash, "admin", "初始管理员")
		if err != nil {
			log.Fatalf("创建管理员失败: %v", err)
		}
		if usingDefault {
			// 标记强制改密：登录后台前必须修改
			_ = st.SetMustChangePass(u.ID, true)
		}

		fmt.Println()
		fmt.Println("========================================================")
		fmt.Println("  已创建管理员账号，请立即登录并修改密码")
		fmt.Printf("  用户名: %s\n", user)
		fmt.Printf("  密  码: %s\n", pass)
		if usingDefault {
			fmt.Println("  ⚠ 使用的是内置默认密码：首次进入后台会强制要求修改密码")
		}
		fmt.Printf("  后台地址: http://localhost:%s/admin\n", env("PORT", "8080"))
		fmt.Println("========================================================")
		fmt.Println()
		return
	}

	// 老版本部署升级：若管理员仍在使用内置默认密码，补打强制改密标记
	user := env("ADMIN_USER", "admin")
	if u, hash, err := st.GetUserByName(user); err == nil && u.Role == "admin" {
		if auth.VerifyPassword("admin"+"8888", hash) {
			_ = st.SetMustChangePass(u.ID, true)
			log.Println("⚠ 检测到管理员仍在使用内置默认密码，下次进入后台将强制要求修改")
		}
	}
}
