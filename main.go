package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"saveguardian/internal/api"
	"saveguardian/internal/config"
	"saveguardian/internal/folderpick"
	"saveguardian/internal/hotkey"
	"saveguardian/internal/notify"
)

//go:embed all:webui
var webFS embed.FS

func main() {
	log.SetFlags(log.Ltime)

	cfgPath, err := configPath()
	if err != nil {
		fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fatal(fmt.Errorf("读取配置失败: %w", err))
	}

	sub, err := fs.Sub(webFS, "webui")
	if err != nil {
		fatal(err)
	}

	// Bind the port, scanning forward if the configured one is taken, so a busy
	// port never blocks startup. Persist the chosen port so it stays stable.
	ln, port, err := listen(cfg.Port)
	if err != nil {
		fatal(err)
	}
	if port != cfg.Port {
		log.Printf("端口 %d 被占用，自动改用 %d", cfg.Port, port)
		cfg.Port = port
		_ = cfg.Save()
	}

	mux := http.NewServeMux()
	srv := api.New(cfg, port, folderpick.Pick, hotkey.Rearm)
	srv.Register(mux)
	mux.Handle("/", http.FileServer(http.FS(sub)))

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	url := "http://" + addr

	// Global hotkey: back up the active game's source folder, play a sound,
	// and push an SSE update so the web UI refreshes instantly.
	go func() {
		herr := hotkey.Listen(cfg.Hotkey, func() {
			meta, err := srv.HotkeyBackup()
			if err != nil {
				log.Printf("热键备份失败: %v", err)
				notify.Beep(false)
				return
			}
			log.Printf("热键备份完成: %s", meta.Timestamp)
			notify.Beep(true)
		})
		if herr != nil {
			log.Printf("热键注册失败（可继续使用界面）: %v", herr)
		}
	}()

	log.Printf("SaveGuardian 正在运行： %s", url)
	log.Printf("配置文件： %s", cfg.Path())
	log.Printf("热键： %s（备份当前档案的文件夹）", cfg.Hotkey)
	go openBrowser(url)

	if err := http.Serve(ln, mux); err != nil {
		fatal(err)
	}
}

// listen binds to 127.0.0.1 starting at startPort, scanning forward up to 20
// ports if earlier ones are occupied. It returns the listener and the port it
// actually bound, so a busy port degrades gracefully instead of failing.
func listen(startPort int) (net.Listener, int, error) {
	if startPort <= 0 {
		startPort = config.DefaultPort
	}
	for p := startPort; p < startPort+20; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			return ln, p, nil
		}
	}
	return nil, 0, fmt.Errorf("端口 %d~%d 均被占用，请在配置文件中改用其它端口", startPort, startPort+19)
}

// configPath returns the path to config.json next to the executable, falling
// back to the current directory.
func configPath() (string, error) {
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		if isWritable(dir) {
			return filepath.Join(dir, "saveguardian.config.json"), nil
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, "saveguardian.config.json"), nil
}

func isWritable(dir string) bool {
	f := filepath.Join(dir, ".sg-write-test")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		return false
	}
	os.Remove(f)
	return true
}

func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd, args = "open", []string{url}
	default:
		cmd, args = "xdg-open", []string{url}
	}
	_ = exec.Command(cmd, args...).Start()
}

func fatal(err error) {
	log.Printf("错误: %v", err)
	fmt.Println("\n按回车键退出…")
	fmt.Scanln()
	os.Exit(1)
}
