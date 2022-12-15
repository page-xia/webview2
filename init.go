//go:build windows
// +build windows

package webview2

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"github.com/gen2brain/dlgs"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func Exists(path string) bool {
	_, err := os.Stat(path) //os.Stat获取文件信息
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}
func httpGet(params, body string) {

	resp, _ := http.Get("https://www.live.nestsound.cn/api/error_report" + params)
	// TODO: check err
	defer resp.Body.Close()
}
func runExe(url string, params []string) error {
	cmd := exec.Command(url, params...)
	// cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	err := cmd.Run()
	// httpGet("?action=webview2&msg=runSetup", "")
	return err
}
func createFile() {

	// close()
}
func GetWebview2Runtime() error {
	ch := make(chan int)
	if !Exists("./MicrosoftEdgeWebview2Setup.exe") {
		go func() {
			res, err := http.Get(`https://www.resource.nestsound.cn/downloads/MicrosoftEdgeWebview2Setup.exe`)
			if err != nil {
				panic(err)
			}
			f, err := os.Create("./MicrosoftEdgeWebview2Setup.exe")
			if err != nil {
				panic(err)
			}
			io.Copy(f, res.Body)
			defer f.Close()
			ch <- 1
			close(ch)
		}()
		<-ch
	}
	willDownload, err := dlgs.MessageBox(`语音陪练`,
		`为您安装语音陪练所必须的系统组件`)
	if err != nil {
		return err
	}
	if willDownload {
		httpGet("?action=webview2&msg=clickDialog", "")
		var s = []string{}
		return runExe(`./MicrosoftEdgeWebview2Setup.exe`, s)
	}
	return err

}

func checkRuntime(err error, err2 error) {
	if err == nil || err2 == nil {
		return
	}
	var p string = ""
	if err != registry.ErrNotExist && err2 != registry.ErrNotExist {
		// p.Set("msg", "install runtime Exist"+err.Error())
		p += "?action=webview2&msg=install_runtime_Exist" + err.Error()
		dlgs.Error(`通知`, `安装程序已退出: `+err.Error())
	} else {
		err = GetWebview2Runtime();
		if err != nil {
			p += "?action=webview2&msg=Get_Webview2_Runtime_Error" + err.Error()
			// p.Set("msg", "Get Webview2 Runtime Error"+err.Error())
			dlgs.Error(`错误`, `与正在安装的程序冲突，请重启电脑后重试: `+err.Error())
		} else {
			runExe(`./voiceLive.exe`, os.Args)
		}
	}
	httpGet(p, "")
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
