//go:build windows
// +build windows

package webview2

import (
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"syscall"

	"github.com/gen2brain/dlgs"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func httpGet(params, body string) {

	resp, _ := http.Get("https://www.live.nestsound.cn/api/error_report" + params)
	// TODO: check err
	defer resp.Body.Close()
}
func GetWebview2Runtime() error {
	willDownload, err := dlgs.Question(`系统组件缺失`,
		`请下载最新组件库`, false)
	if err != nil {
		return err
	}
	if willDownload {
		cmd := exec.Command(`cmd`, `/c`, `start`, `https://go.microsoft.com/fwlink/p/?LinkId=2124703`)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		return cmd.Start()
	}
	return err
}

func checkRuntime(err error, err2 error) {
	if err == nil || err2 == nil {
		return
	}
	// p := url.Values{}
	// p.Set("action", "webview2")
	var p string = ""
	if err != registry.ErrNotExist && err2 != registry.ErrNotExist {
		// p.Set("msg", "install runtime Exist"+err.Error())
		p += "?action=webview2&msg=install_runtime_Exist" + err.Error()
		dlgs.Error(`Microsoft Webview2 Runtime`, `Webview2 Runtime Error: `+err.Error())
	} else {
		if err := GetWebview2Runtime(); err != nil {
			p += "?action=webview2&msg=Get_Webview2_Runtime_Error" + err.Error()
			// p.Set("msg", "Get Webview2 Runtime Error"+err.Error())
			dlgs.Error(`Microsoft Webview2 Runtime`, `Get Webview2 Runtime Error: `+err.Error())
		}
	}
	// httpGet(p, "")
	os.Exit(1)
}

func init() {
	// Enable High Dpi Support
	var major, _, _ = RtlGetNtVersionNumbers()
	if major > 6 {
		windows.NewLazySystemDLL("Shcore").NewProc("SetProcessDpiAwareness").Call(1)
	}
	var key registry.Key
	var key2 registry.Key
	var err error = nil
	var err2 error = nil
	switch runtime.GOARCH {
	case "amd64":
		key, err = registry.OpenKey(registry.LOCAL_MACHINE,
			`SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`,
			registry.READ)
		key2, err2 = registry.OpenKey(registry.CURRENT_USER,
			`Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`,
			registry.READ)
	case "386":
		key, err = registry.OpenKey(registry.LOCAL_MACHINE,
			`SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`,
			registry.READ)
		key2, err2 = registry.OpenKey(registry.CURRENT_USER,
			`Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`,
			registry.READ)
	default:
		return
	}
	defer key.Close()
	checkRuntime(err, err2)
	_, _, err = key.GetStringValue(`pv`)
	_, _, err2 = key2.GetStringValue(`pv`)
	checkRuntime(err, err2)
}
