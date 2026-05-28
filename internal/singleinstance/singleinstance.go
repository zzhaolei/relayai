package singleinstance

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// LockFile 尝试获取单实例锁，返回 unlock 函数和 error
// 如果已有实例在运行，返回 error
func LockFile() (unlock func(), err error) {
	lockPath, err := lockFilePath()
	if err != nil {
		return nil, fmt.Errorf("获取锁文件路径失败: %w", err)
	}

	// 检查是否已有实例在运行
	if data, err := os.ReadFile(lockPath); err == nil {
		pidStr := strings.TrimSpace(string(data))
		if pid, err := strconv.Atoi(pidStr); err == nil {
			if isProcessAlive(pid) {
				return nil, fmt.Errorf("RelayAI 已在运行中 (PID: %d)", pid)
			}
		}
	}

	// 写入当前 PID
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return nil, fmt.Errorf("创建锁目录失败: %w", err)
	}
	if err := os.WriteFile(lockPath, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		return nil, fmt.Errorf("写入锁文件失败: %w", err)
	}

	unlock = func() {
		os.Remove(lockPath)
	}
	return unlock, nil
}

func lockFilePath() (string, error) {
	var dir string
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, "Library", "Application Support", "RelayAI")
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			home, _ := os.UserHomeDir()
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		dir = filepath.Join(appData, "RelayAI")
	default: // linux
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".relayai")
	}
	return filepath.Join(dir, "relayai.lock"), nil
}

func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
