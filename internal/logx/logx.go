package logx

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// 轻量轮转文件日志：单文件超过 maxBytes 后按 clearframe.log.1 → .5 依次顺移，
// 保留 backupCount 份备份。零外部依赖，配合 log.SetOutput 使用。

const (
	maxBytes    = 10 << 20 // 单文件 10MB
	backupCount = 5
)

type rotatingWriter struct {
	mu   sync.Mutex
	dir  string
	name string
	f    *os.File
	size int64
}

// Init 建立日志目录并打开日志文件；返回的 Writer 同时写 stdout 与文件。
// 目录创建或文件打开失败时返回错误，由调用方决定降级策略。
func Init(dir string) (io.Writer, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}
	w := &rotatingWriter{dir: dir, name: "clearframe.log"}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingWriter) open() error {
	path := filepath.Join(w.dir, w.name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	w.f = f
	w.size = info.Size()
	return nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size+int64(len(p)) > maxBytes {
		w.rotate()
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *rotatingWriter) rotate() {
	w.f.Close()
	for i := backupCount - 1; i >= 1; i-- {
		src := filepath.Join(w.dir, fmt.Sprintf("%s.%d", w.name, i))
		dst := filepath.Join(w.dir, fmt.Sprintf("%s.%d", w.name, i+1))
		os.Rename(src, dst)
	}
	os.Rename(filepath.Join(w.dir, w.name), filepath.Join(w.dir, w.name+".1"))
	if err := w.open(); err != nil {
		// 磁盘异常时兜底回 stderr，进程不能因为没有日志文件起不来
		w.f = os.Stderr
		w.size = 0
	}
}
