package main

import (
	"github.com/page-xia/webview2"
	_ "github.com/page-xia/webview2/manifest"
)

func main() {
	w := webview2.New(false, nil)
	if w == nil {
		println("Failed to load webview.")
		return
	}
	defer w.Destroy()
	w.SetTitle(`Minimal webview example`)
	w.SetSize(1280, 720, webview2.HintNone)
	w.Navigate(`https://www.google.com`, false)
	w.Run()
}
