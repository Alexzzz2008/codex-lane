package appservice

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/model"
)

const (
	RepositoryURL = "https://github.com/Alexzzz2008/codex-lane"
	ReleasesURL   = RepositoryURL + "/releases"
	releaseFeed   = ReleasesURL + ".atom"
)

type releaseFeedXML struct {
	Entries []releaseEntryXML `xml:"entry"`
}

type releaseEntryXML struct {
	Updated string `xml:"updated"`
	Link    struct {
		Href string `xml:"href,attr"`
	} `xml:"link"`
}

func (s *AppService) CheckForUpdates() model.UpdateStatus {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	status := checkForUpdates(ctx, Version, releaseFeed)
	s.mu.Lock()
	s.updateStatus = status
	s.mu.Unlock()
	if status.Available {
		s.addEvent("info", "发现新版本", "v"+status.LatestVersion+" 可通过 GitHub Releases 获取")
	}
	return status
}

func checkForUpdates(ctx context.Context, currentVersion, feedURL string) model.UpdateStatus {
	status := model.UpdateStatus{
		CurrentVersion: strings.TrimPrefix(currentVersion, "v"),
		CheckedAt:      time.Now().Format(time.RFC3339),
		Status:         "error",
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	request.Header.Set("Accept", "application/atom+xml")
	request.Header.Set("User-Agent", "CodexLane/"+status.CurrentVersion)

	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		status.Error = "无法连接 GitHub Releases"
		return status
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		status.Status = "unavailable"
		status.Error = "仓库未公开或暂时没有 Release"
		return status
	}
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 8*1024))
		status.Error = fmt.Sprintf("GitHub Releases 返回 HTTP %d", response.StatusCode)
		return status
	}

	var feed releaseFeedXML
	if err := xml.NewDecoder(io.LimitReader(response.Body, 512*1024)).Decode(&feed); err != nil {
		status.Error = "无法解析 GitHub Release 信息"
		return status
	}
	if len(feed.Entries) == 0 {
		status.Status = "unreleased"
		status.Error = "仓库暂时没有 Release"
		return status
	}

	latest := strings.TrimPrefix(versionFromReleaseURL(feed.Entries[0].Link.Href), "v")
	if latest == "" || !isVersion(latest) {
		status.Error = "无法识别最新 Release 版本"
		return status
	}
	status.LatestVersion = latest
	status.ReleaseURL = feed.Entries[0].Link.Href
	status.PublishedAt = feed.Entries[0].Updated
	status.Available = compareVersions(latest, status.CurrentVersion) > 0
	if status.Available {
		status.Status = "available"
	} else {
		status.Status = "current"
	}
	return status
}

func versionFromReleaseURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 || parts[len(parts)-2] != "tag" {
		return ""
	}
	return strings.TrimSpace(parts[len(parts)-1])
}

func isVersion(value string) bool {
	parts := strings.SplitN(strings.TrimPrefix(strings.TrimSpace(value), "v"), "-", 2)
	core := strings.Split(parts[0], ".")
	if len(core) == 0 {
		return false
	}
	for _, part := range core {
		if part == "" {
			return false
		}
		if _, err := strconv.Atoi(part); err != nil {
			return false
		}
	}
	return true
}

func compareVersions(left, right string) int {
	leftCore, leftPre := versionParts(left)
	rightCore, rightPre := versionParts(right)
	length := max(len(leftCore), len(rightCore))
	for index := 0; index < length; index++ {
		var a, b int
		if index < len(leftCore) {
			a = leftCore[index]
		}
		if index < len(rightCore) {
			b = rightCore[index]
		}
		if a > b {
			return 1
		}
		if a < b {
			return -1
		}
	}
	switch {
	case leftPre == "" && rightPre != "":
		return 1
	case leftPre != "" && rightPre == "":
		return -1
	case leftPre > rightPre:
		return 1
	case leftPre < rightPre:
		return -1
	default:
		return 0
	}
}

func versionParts(value string) ([]int, string) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	value = strings.SplitN(value, "+", 2)[0]
	core := value
	pre := ""
	if index := strings.IndexByte(value, '-'); index >= 0 {
		core = value[:index]
		pre = value[index+1:]
	}
	parts := strings.Split(core, ".")
	numbers := make([]int, 0, len(parts))
	for _, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil {
			number = 0
		}
		numbers = append(numbers, number)
	}
	return numbers, pre
}
