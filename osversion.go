package webview2

import (
	"syscall"
	"unsafe"
)

func RtlGetNtVersionNumbers() (majorVersion, minorVersion, buildNumber uint32) {
	//var majorVersion, minorVersion, buildNumber uint32
	ntdll := syscall.NewLazyDLL("ntdll.dll")
	procRtlGetNtVersionNumbers := ntdll.NewProc("RtlGetNtVersionNumbers")
	//v, vv, err := procRtlGetNtVersionNumbers.Call(
	procRtlGetNtVersionNumbers.Call(
		uintptr(unsafe.Pointer(&majorVersion)),
		uintptr(unsafe.Pointer(&minorVersion)),
		uintptr(unsafe.Pointer(&buildNumber)),
	)
	buildNumber &= 0xffff

	return

}
