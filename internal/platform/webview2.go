package platform

import "strings"

type WebView2Status struct {
	Installed bool
	Version   string
	Error     string
}

const webView2ClientGUID = "{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"
const webView2DownloadURL = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"
const webView2DownloadPage = "https://developer.microsoft.com/microsoft-edge/webview2/"

func validWebView2Version(version string) bool {
	version = strings.TrimSpace(version)
	return version != "" && version != "0.0.0.0"
}
