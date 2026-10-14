package storage

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	persistentLogMu      sync.Mutex
	persistentLogEnabled bool
	persistentLogDays    = 7
	persistentLogFile    *os.File
	persistentLogBuf     *bufio.Writer
	persistentLogPath    string
	persistentLogFlushT  *time.Timer
	secretLogPattern     = regexp.MustCompile(`(?i)(accessToken|refreshToken|clientSecret|device_code|user_code|regCode|workflowState|code|验证码)(\s*[:=]\s*)([^\s,}]+)`)
)

// ConfigurePersistentLogs updates file logging and removes expired files.
func ConfigurePersistentLogs(enabled bool, retentionDays int) {
	persistentLogMu.Lock()
	defer persistentLogMu.Unlock()
	persistentLogEnabled = enabled
	if !enabled {
		closePersistentLogLocked()
	}
	if retentionDays >= 1 && retentionDays <= 90 {
		persistentLogDays = retentionDays
	}
	pruneLogsLocked(time.Now())
}

// AppendPersistentLog writes a redacted copy. UI logs remain unchanged in memory.
func AppendPersistentLog(message string) {
	persistentLogMu.Lock()
	defer persistentLogMu.Unlock()
	if !persistentLogEnabled {
		return
	}
	path := filepath.Join(GetLogsDir(), time.Now().Format("2006-01-02")+".log")
	if persistentLogFile == nil || persistentLogPath != path {
		closePersistentLogLocked()
		_ = os.MkdirAll(GetLogsDir(), 0o755)
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return
		}
		persistentLogFile = file
		persistentLogBuf = bufio.NewWriterSize(file, 32*1024)
		persistentLogPath = path
	}
	redacted := secretLogPattern.ReplaceAllString(message, "$1$2<redacted>")
	_, _ = persistentLogBuf.WriteString(redacted)
	// 缓冲写入，1 秒内合并落盘
	if persistentLogFlushT == nil {
		persistentLogFlushT = time.AfterFunc(time.Second, func() {
			persistentLogMu.Lock()
			defer persistentLogMu.Unlock()
			persistentLogFlushT = nil
			if persistentLogBuf != nil {
				_ = persistentLogBuf.Flush()
			}
		})
	}
}

// closePersistentLogLocked 刷新并关闭当前日志文件句柄 (需持有 persistentLogMu)
func closePersistentLogLocked() {
	if persistentLogFlushT != nil {
		persistentLogFlushT.Stop()
		persistentLogFlushT = nil
	}
	if persistentLogBuf != nil {
		_ = persistentLogBuf.Flush()
		persistentLogBuf = nil
	}
	if persistentLogFile != nil {
		_ = persistentLogFile.Close()
		persistentLogFile = nil
	}
	persistentLogPath = ""
}

// FlushPersistentLog 立即将缓冲日志写入磁盘
func FlushPersistentLog() {
	persistentLogMu.Lock()
	defer persistentLogMu.Unlock()
	if persistentLogBuf != nil {
		_ = persistentLogBuf.Flush()
	}
}

// ClosePersistentLog 刷新并关闭日志文件 (应用退出时调用)
func ClosePersistentLog() {
	persistentLogMu.Lock()
	defer persistentLogMu.Unlock()
	closePersistentLogLocked()
}

func pruneLogsLocked(now time.Time) {
	entries, err := os.ReadDir(GetLogsDir())
	if err != nil {
		return
	}
	cutoff := now.AddDate(0, 0, -persistentLogDays)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(GetLogsDir(), entry.Name()))
		}
	}
}

// ClearLogs removes only KiroX .log files.
func ClearLogs() error {
	persistentLogMu.Lock()
	defer persistentLogMu.Unlock()
	closePersistentLogLocked()
	entries, err := os.ReadDir(GetLogsDir())
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".log") {
			if err := os.Remove(filepath.Join(GetLogsDir(), entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}
