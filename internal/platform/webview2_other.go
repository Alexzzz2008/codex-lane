//go:build !windows

package platform

import "context"

func CheckWebView2() WebView2Status {
	return WebView2Status{Installed: true}
}

func EnsureWebView2() error {
	return nil
}

func InstallWebView2(context.Context) error {
	return nil
}
