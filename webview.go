//go:build windows
// +build windows

package webview2

import (
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var DisableWebSecurity = true
var UserDataFolder = filepath.Join(os.Getenv("AppData"),
	strings.TrimSuffix(filepath.Base(os.Args[0]), path.Ext(os.Args[0])))
var UserAgent = ""
var toUrl = ""
var evalJs = ""
var (
	ole32               = windows.NewLazySystemDLL("ole32")
	ole32CoInitializeEx = ole32.NewProc("CoInitializeEx")

	kernel32                   = windows.NewLazySystemDLL("kernel32")
	kernel32GetProcessHeap     = kernel32.NewProc("GetProcessHeap")
	kernel32HeapAlloc          = kernel32.NewProc("HeapAlloc")
	kernel32HeapFree           = kernel32.NewProc("HeapFree")
	kernel32GetCurrentThreadID = kernel32.NewProc("GetCurrentThreadId")

	user32                   = windows.NewLazySystemDLL("user32")
	user32LoadImageW         = user32.NewProc("LoadImageW")
	user32GetSystemMetrics   = user32.NewProc("GetSystemMetrics")
	user32RegisterClassExW   = user32.NewProc("RegisterClassExW")
	user32CreateWindowExW    = user32.NewProc("CreateWindowExW")
	user32DestroyWindow      = user32.NewProc("DestroyWindow")
	user32ShowWindow         = user32.NewProc("ShowWindow")
	user32UpdateWindow       = user32.NewProc("UpdateWindow")
	user32SetFocus           = user32.NewProc("SetFocus")
	user32GetMessageW        = user32.NewProc("GetMessageW")
	user32PostMessageW       = user32.NewProc("PostMessageW")
	user32TranslateMessage   = user32.NewProc("TranslateMessage")
	user32DispatchMessageW   = user32.NewProc("DispatchMessageW")
	user32DefWindowProcW     = user32.NewProc("DefWindowProcW")
	user32GetClientRect      = user32.NewProc("GetClientRect")
	user32PostQuitMessage    = user32.NewProc("PostQuitMessage")
	user32SetWindowTextW     = user32.NewProc("SetWindowTextW")
	user32PostThreadMessageW = user32.NewProc("PostThreadMessageW")
	user32GetWindowLongPtrW  = user32.NewProc("GetWindowLongPtrW")
	user32SetWindowLongPtrW  = user32.NewProc("SetWindowLongPtrW")
	user32AdjustWindowRect   = user32.NewProc("AdjustWindowRect")
	user32SetWindowPos       = user32.NewProc("SetWindowPos")
	user32GetDpiForSystem    = user32.NewProc("GetDpiForSystem")

	defaultHeap uintptr
)

var (
	windowContext     = map[uintptr]interface{}{}
	windowContextSync sync.RWMutex
)

func getWindowContext(wnd uintptr) interface{} {
	windowContextSync.RLock()
	defer windowContextSync.RUnlock()
	return windowContext[wnd]
}

func setWindowContext(wnd uintptr, data interface{}) {
	windowContextSync.Lock()
	defer windowContextSync.Unlock()
	windowContext[wnd] = data
}

const (
	_SystemMetricsCxIcon = 11
	_SystemMetricsCyIcon = 12
)

const (
	_SWShow = 5
)

const (
	_SWPNoZOrder     = 0x0004
	_SWPNoActivate   = 0x0010
	_SWPNoMove       = 0x0002
	_SWPFrameChanged = 0x0020
)

const (
	_WMDestroy       = 0x0002
	_WMSize          = 0x0005
	_WMClose         = 0x0010
	_WMQuit          = 0x0012
	_WMNavigate      = 0x2022
	_WMEval          = 0x2023
	_WMGetMinMaxInfo = 0x0024
	_WMApp           = 0x8000
)

const (
	_GWLStyle = -16
)

const (
	_WSOverlapped       = 0x00000000
	_WSMaximizeBox      = 0x00020000
	_WSThickFrame       = 0x00040000
	_WSCaption          = 0x00C00000
	_WSSysMenu          = 0x00080000
	_WSMinimizeBox      = 0x00020000
	_WSOverlappedWindow = (_WSOverlapped | _WSCaption | _WSSysMenu | _WSThickFrame | _WSMinimizeBox | _WSMaximizeBox)
)

type _WndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cnClsExtra    int32
	cbWndExtra    int32
	hInstance     windows.Handle
	hIcon         windows.Handle
	hCursor       windows.Handle
	hbrBackground windows.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       windows.Handle
}

type _Rect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type _Point struct {
	x, y int32
}

type _Msg struct {
	hwnd     syscall.Handle
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	pt       _Point
	lPrivate uint32
}

type _MinMaxInfo struct {
	ptReserved     _Point
	ptMaxSize      _Point
	ptMaxPosition  _Point
	ptMinTrackSize _Point
	ptMaxTrackSize _Point
}

func init() {
	runtime.LockOSThread()

	r, _, _ := ole32CoInitializeEx.Call(0, 2)
	if r < 0 {
		log.Printf("Warning: CoInitializeEx call failed: E=%08x", r)
	}

	defaultHeap, _, _ = kernel32GetProcessHeap.Call()
}

func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	// Find NUL terminator.
	end := unsafe.Pointer(p)
	n := 0
	for *(*uint16)(end) != 0 {
		end = unsafe.Pointer(uintptr(end) + unsafe.Sizeof(*p))
		n++
	}
	s := (*[(1 << 30) - 1]uint16)(unsafe.Pointer(p))[:n:n]
	return string(utf16.Decode(s))
}

type chromiumedge struct {
	hwnd                uintptr
	controller          *iCoreWebView2Controller
	webview             *iCoreWebView2
	inited              uintptr
	envCompleted        *iCoreWebView2CreateCoreWebView2EnvironmentCompletedHandler
	controllerCompleted *iCoreWebView2CreateCoreWebView2ControllerCompletedHandler
	webMessageReceived  *iCoreWebView2WebMessageReceivedEventHandler
	permissionRequested *iCoreWebView2PermissionRequestedEventHandler
	webResourceRequested  *iCoreWebView2WebResourceRequestedEventHandler
	WebResourceRequestedCallback func(request *ICoreWebView2WebResourceRequest, args *ICoreWebView2WebResourceRequestedEventArgs)
	msgcb               func(string)
}

type browser interface {
	Embed(debug bool, hwnd uintptr) bool
	Resize()
	Navigate(url string)
	Init(script string)
	Eval(script string)
}

type webview struct {
	hwnd       uintptr
	mainthread uintptr
	browser    browser
	maxsz      _Point
	minsz      _Point
}

func newchromiumedge() *chromiumedge {
	e := &chromiumedge{}
	e.envCompleted = newICoreWebView2CreateCoreWebView2EnvironmentCompletedHandler(e)
	e.controllerCompleted = newICoreWebView2CreateCoreWebView2ControllerCompletedHandler(e)
	e.webMessageReceived = newICoreWebView2WebMessageReceivedEventHandler(e)
	e.permissionRequested = newICoreWebView2PermissionRequestedEventHandler(e)
	e.webResourceRequested = newICoreWebView2WebResourceRequestedEventHandler(e)
	return e
}

func (e *chromiumedge) Embed(debug bool, hwnd uintptr) bool {
	e.hwnd = hwnd
	if DisableWebSecurity {
		os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "--disable-web-security --autoplay-policy=no-user-gesture-required --allow-file-access-from-files")
	}
	res, err := createCoreWebView2EnvironmentWithOptions(nil, windows.StringToUTF16Ptr(UserDataFolder), 0, e.envCompleted)
	if err != nil {
		log.Printf("Error calling Webview2Loader: %v", err)
		return false
	} else if res != 0 {
		log.Printf("Result: %08x", res)
		return false
	}
	var msg _Msg
	for {
		if atomic.LoadUintptr(&e.inited) != 0 {
			break
		}
		r, _, _ := user32GetMessageW.Call(
			uintptr(unsafe.Pointer(&msg)),
			0,
			0,
			0,
		)
		if r == 0 {
			break
		}
		user32TranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		user32DispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	e.Init("window.external={invoke:s=>window.chrome.webview.postMessage(s)}")

	var settings *iCoreWebView2Settings3
	e.webview.vtbl.GetSettings.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(&settings)),
	)
	if !debug {
		// Disable drag and drop to open files
		e.Init(`window.addEventListener('dragover',function(e){e.preventDefault();},false);` +
			`window.addEventListener('drop',function(e){e.preventDefault();},false);`)
		// 0 -> false (Windows)
		settings.vtbl.putAreDevToolsEnabled.Call(
			uintptr(unsafe.Pointer(settings)),
			uintptr(0),
		)
		settings.vtbl.putAreDefaultContextMenusEnabled.Call(
			uintptr(unsafe.Pointer(settings)),
			uintptr(0),
		)
		settings.vtbl.putIsBuiltInErrorPageEnabled.Call(
			uintptr(unsafe.Pointer(settings)),
			uintptr(0),
		)
		settings.vtbl.putIsStatusBarEnabled.Call(
			uintptr(unsafe.Pointer(settings)),
			uintptr(0),
		)
		settings.vtbl.putIsZoomControlEnabled.Call(
			uintptr(unsafe.Pointer(settings)),
			uintptr(0),
		)
	}
	var ua *uint16
	settings.vtbl.getUserAgent.Call(
		uintptr(unsafe.Pointer(settings)),
		uintptr(unsafe.Pointer(&ua)),
	)
	var s = windows.UTF16PtrToString(ua)
	if UserAgent != "" {
		settings.vtbl.putUserAgent.Call(
			uintptr(unsafe.Pointer(settings)),
			uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(s+" "+UserAgent))),
		)
	}
	return true
}
func (e *chromiumedge) WebResourceRequested(sender *iCoreWebView2, args *ICoreWebView2WebResourceRequestedEventArgs) uintptr {
	req, err := args.GetRequest()
	if err != nil {
		log.Fatal(err)
	}
	if e.WebResourceRequestedCallback != nil {
		e.WebResourceRequestedCallback(req, args)
	}
	return 0
}
func (e *chromiumedge) Navigate(url string) {
	e.webview.vtbl.Navigate.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(url))),
	)
}

func (e *chromiumedge) Init(script string) {
	e.webview.vtbl.AddScriptToExecuteOnDocumentCreated.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(script))),
		0,
	)
}

func (e *chromiumedge) Eval(script string) {
	e.webview.vtbl.ExecuteScript.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(script))),
		0,
	)
}

func (e *chromiumedge) QueryInterface(refiid, object uintptr) uintptr {
	return 0
}

func (e *chromiumedge) AddRef() uintptr {
	return 1
}

func (e *chromiumedge) Release() uintptr {
	return 1
}

func (e *chromiumedge) EnvironmentCompleted(res uintptr, env *iCoreWebView2Environment) uintptr {
	if int64(res) < 0 {
		log.Fatalf("Creating environment failed with %08x", res)
	}
	env.vtbl.CreateCoreWebView2Controller.Call(
		uintptr(unsafe.Pointer(env)),
		e.hwnd,
		uintptr(unsafe.Pointer(e.controllerCompleted)),
	)
	return 0
}

func (e *chromiumedge) ControllerCompleted(res uintptr, controller *iCoreWebView2Controller) uintptr {
	if int64(res) < 0 {
		log.Fatalf("Creating controller failed with %08x", res)
	}
	controller.vtbl.AddRef.Call(uintptr(unsafe.Pointer(controller)))
	e.controller = controller

	var token _EventRegistrationToken
	controller.vtbl.GetCoreWebView2.Call(
		uintptr(unsafe.Pointer(controller)),
		uintptr(unsafe.Pointer(&e.webview)),
	)
	e.webview.vtbl.AddRef.Call(
		uintptr(unsafe.Pointer(e.webview)),
	)
	e.webview.vtbl.AddWebMessageReceived.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(e.webMessageReceived)),
		uintptr(unsafe.Pointer(&token)),
	)
	e.webview.vtbl.AddPermissionRequested.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(e.permissionRequested)),
		uintptr(unsafe.Pointer(&token)),
	)
	e.webview.vtbl.AddWebResourceRequested.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(e.webResourceRequested)),
		uintptr(unsafe.Pointer(&token)),
	)

	atomic.StoreUintptr(&e.inited, 1)

	return 0
}

func (e *chromiumedge) MessageReceived(sender *iCoreWebView2, args *iCoreWebView2WebMessageReceivedEventArgs) uintptr {
	var message *uint16
	args.vtbl.TryGetWebMessageAsString.Call(
		uintptr(unsafe.Pointer(args)),
		uintptr(unsafe.Pointer(&message)),
	)
	e.msgcb(utf16PtrToString(message))
	sender.vtbl.PostWebMessageAsString.Call(
		uintptr(unsafe.Pointer(sender)),
		uintptr(unsafe.Pointer(message)),
	)
	windows.CoTaskMemFree(unsafe.Pointer(message))
	return 0
}

func (e *chromiumedge) PermissionRequested(sender *iCoreWebView2, args *iCoreWebView2PermissionRequestedEventArgs) uintptr {
	var kind _CoreWebView2PermissionKind
	args.vtbl.GetPermissionKind.Call(
		uintptr(unsafe.Pointer(args)),
		uintptr(unsafe.Pointer(&kind)),
	)
	if kind == _CoreWebView2PermissionKindClipboardRead || kind == _CoreWebView2PermissionKindMicrophone || kind == _CoreWebView2PermissionKindCamera {
		args.vtbl.PutState.Call(
			uintptr(unsafe.Pointer(args)),
			uintptr(_CoreWebView2PermissionStateAllow),
		)
	}
	return 0
}

// New creates a new webview in a new window.
func New(debug bool, window unsafe.Pointer) WebView { return NewWindow(debug, window) }

// NewWindow creates a new webview using an existing window.
func NewWindow(debug bool, window unsafe.Pointer) WebView {
	w := &webview{}
	w.browser = newchromiumedge()
	w.mainthread, _, _ = kernel32GetCurrentThreadID.Call()
	if !w.Create(debug, window) {
		return nil
	}
	return w
}

func wndproc(hwnd, msg, wp, lp uintptr) uintptr {
	if w, ok := getWindowContext(hwnd).(*webview); ok {
		switch msg {
		case _WMSize:
			w.browser.Resize()
		case _WMClose:
			user32DestroyWindow.Call(hwnd)
		case _WMDestroy:
			w.Terminate()
		case _WMGetMinMaxInfo:
			lpmmi := (*_MinMaxInfo)(unsafe.Pointer(lp))
			if w.maxsz.x > 0 && w.maxsz.y > 0 {
				lpmmi.ptMaxSize = w.maxsz
				lpmmi.ptMaxTrackSize = w.maxsz
			}
			if w.minsz.x > 0 && w.minsz.y > 0 {
				lpmmi.ptMinTrackSize = w.minsz
			}
		default:
			r, _, _ := user32DefWindowProcW.Call(hwnd, msg, wp, lp)
			return r
		}
		return 0
	}
	r, _, _ := user32DefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

func (w *webview) Create(debug bool, window unsafe.Pointer) bool {
	if window != nil {
		if !w.browser.Embed(debug, uintptr(window)) {
			return false
		}
		w.browser.Resize()
	}
	var hinstance windows.Handle
	windows.GetModuleHandleEx(0, nil, &hinstance)

	icow, _, _ := user32GetSystemMetrics.Call(_SystemMetricsCxIcon)
	icoh, _, _ := user32GetSystemMetrics.Call(_SystemMetricsCyIcon)

	icon, _, _ := user32LoadImageW.Call(uintptr(hinstance), 32512, icow, icoh, 0)
	wc := _WndClassExW{
		style:         35, /* CS_HREDRAW | CS_VREDRAW | CS_OWNDC */
		cbSize:        uint32(unsafe.Sizeof(_WndClassExW{})),
		hInstance:     hinstance,
		lpszClassName: windows.StringToUTF16Ptr("webview"),
		hIcon:         windows.Handle(icon),
		hIconSm:       windows.Handle(icon),
		lpfnWndProc:   windows.NewCallback(wndproc),
	}
	var dpi = getDpi()
	var width = uintptr(dpi * 590)
	var height = uintptr(dpi * 800)
	var mw, _, _ = user32GetSystemMetrics.Call(0)
	var mh, _, _ = user32GetSystemMetrics.Call(1)
	if (height > mh) {
		height = mh
	}
	user32RegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	w.hwnd, _, _ = user32CreateWindowExW.Call(
		35, /* CS_HREDRAW | CS_VREDRAW | CS_OWNDC */
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("webview"))),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(""))),
		0x80000, // WS_OVERLAPPEDWINDOW 0xCF0000
		mw-width, // CW_USEDEFAULT
		0,        // CW_USEDEFAULT
		width,
		height,
		0,
		0,
		uintptr(hinstance),
		0,
	)
	setWindowContext(w.hwnd, w)

	user32ShowWindow.Call(w.hwnd, _SWShow)
	user32UpdateWindow.Call(w.hwnd)
	user32SetFocus.Call(w.hwnd)

	if !w.browser.Embed(debug, w.hwnd) {
		return false
	}
	w.browser.Resize()
	return true
}

func (w *webview) Destroy() {
	user32PostQuitMessage.Call(0)
}

func (w *webview) Run() {
	var msg _Msg
	for {
		user32GetMessageW.Call(
			uintptr(unsafe.Pointer(&msg)),
			0,
			0,
			0,
		)
		if msg.message == _WMNavigate {
			w.browser.Navigate(toUrl)
		}
		if msg.message == _WMEval {
			w.browser.Eval(evalJs)
		}
		if msg.message == _WMApp {

		} else if msg.message == _WMQuit {
			user32PostQuitMessage.Call(0)
			return
		}
		user32TranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		user32DispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func (w *webview) Terminate() {
	user32PostMessageW.Call(w.hwnd, _WMQuit)
	user32PostQuitMessage.Call(0)
}

func (w *webview) Window() unsafe.Pointer {
	return unsafe.Pointer(w.hwnd)
}

func (w *webview) Navigate(url string, isMsg bool) {
	toUrl = url
	if isMsg {
		user32PostMessageW.Call(w.hwnd, _WMNavigate)
	} else {
		w.browser.Navigate(url)
	}
}

func (w *webview) SetTitle(title string) {
	user32SetWindowTextW.Call(w.hwnd, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(title))))
}

func (w *webview) SetSize(width int, height int, hints Hint) {
	index := _GWLStyle
	style, _, _ := user32GetWindowLongPtrW.Call(w.hwnd, uintptr(index))
	if hints == HintFixed {
		style &^= (_WSThickFrame | _WSMaximizeBox)
	} else {
		style |= (_WSThickFrame | _WSMaximizeBox)
	}
	user32SetWindowLongPtrW.Call(w.hwnd, uintptr(index), style)
	var dpi = getDpi()
	width = dpi * width
	height = dpi * height
	if hints == HintMax {
		w.maxsz.x = int32(width)
		w.maxsz.y = int32(height)
	} else if hints == HintMin {
		w.minsz.x = int32(width)
		w.minsz.y = int32(height)
	} else {
		r := _Rect{}
		r.Left = 0
		r.Top = 0
		r.Right = int32(width)
		r.Bottom = int32(height)
		user32AdjustWindowRect.Call(uintptr(unsafe.Pointer(&r)), _WSOverlappedWindow, 0)
		user32SetWindowPos.Call(
			w.hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top),
			_SWPNoZOrder|_SWPNoActivate|_SWPNoMove|_SWPFrameChanged)
		w.browser.Resize()
	}
}

func (w *webview) Init(js string) {
	w.browser.Init(js)
}

func (w *webview) Eval(js string, isMsg bool) {
	evalJs = js
	if isMsg {
		user32PostMessageW.Call(w.hwnd, _WMEval)
	} else {
		w.browser.Eval(js)
	}
}

func (w *webview) Dispatch(f func()) {
	// TODO
}

func (w *webview) Bind(name string, f interface{}) error {
	// TODO
	return nil
}
func getDpi() int {
	var major, _, _ = RtlGetNtVersionNumbers()
	var dpi uintptr
	if major >= 10 {
		dpi, _, _ = user32GetDpiForSystem.Call()
	}
	if int(dpi) < 96 {
		dpi = 96
	}
	return int(dpi) / 96
}
