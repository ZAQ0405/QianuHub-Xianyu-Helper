//go:build windows

// portable 是不依赖 Go、Node、Docker 或本机 Chrome 的 Windows 启动器。
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

const (
	listenAddress = "127.0.0.1:59188"
	webURL        = "http://127.0.0.1:59188"
)

func main() {
	if err := run(); err != nil {
		showError(fmt.Sprintf("QianuHub 启动失败：%v", err))
		return
	}
}

// showError 在无控制台的便携版启动失败时显示可读的系统提示。
func showError(message string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBox := user32.NewProc("MessageBoxW")
	text, _ := syscall.UTF16PtrFromString(message)
	title, _ := syscall.UTF16PtrFromString("QianuHub 闲鱼助手")
	_, _, _ = messageBox.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}

func run() error {
	appPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取程序路径失败：%w", err)
	}
	appDir := filepath.Dir(appPath)
	dataDir := filepath.Join(appDir, "data")
	runtimeDir := filepath.Join(appDir, "playwright-runtime")
	logDir := filepath.Join(appDir, "logs")
	for _, dir := range []string{dataDir, runtimeDir, logDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建目录 %s 失败：%w", dir, err)
		}
	}

	if !healthy() {
		serverPath := filepath.Join(appDir, "qianuhub-server.exe")
		if _, err := os.Stat(serverPath); err != nil {
			return fmt.Errorf("未找到 %s：请保持便携版目录完整", serverPath)
		}
		if err := startServer(serverPath, appDir, dataDir, runtimeDir, logDir); err != nil {
			return err
		}
		if err := waitForServer(60 * time.Second); err != nil {
			return err
		}
	}

	return openBrowser(webURL)
}

func healthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, webURL+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices
}

func startServer(serverPath, appDir, dataDir, runtimeDir, logDir string) error {
	outFile, err := os.OpenFile(filepath.Join(logDir, "server.out.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("打开标准输出日志失败：%w", err)
	}
	errFile, err := os.OpenFile(filepath.Join(logDir, "server.err.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		_ = outFile.Close()
		return fmt.Errorf("打开错误日志失败：%w", err)
	}

	cmd := exec.Command(serverPath,
		"-addr", listenAddress,
		"-workdir", dataDir,
		"-data-key-file", filepath.Join(dataDir, "data-key"),
		"-playwright-runtime-root", runtimeDir,
	)
	cmd.Dir = appDir
	cmd.Stdout = outFile
	cmd.Stderr = errFile
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		_ = outFile.Close()
		_ = errFile.Close()
		return fmt.Errorf("启动服务失败：%w", err)
	}
	_ = outFile.Close()
	_ = errFile.Close()
	return nil
}

func waitForServer(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if healthy() {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("服务启动超时，请查看 logs/server.err.log")
}

func openBrowser(url string) error {
	cmd := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("打开浏览器失败：请手动访问 %s：%w", url, err)
	}
	return nil
}
