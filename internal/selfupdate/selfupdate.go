package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxDownloadSize = int64(512 << 20)

type Asset struct {
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	Version     string           `json:"version"`
	Notes       string           `json:"notes"`
	ReleaseURL  string           `json:"releaseUrl"`
	PublishedAt string           `json:"publishedAt"`
	Assets      map[string]Asset `json:"assets"`
}

type HTTPError struct {
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("update feed returned HTTP %d", e.StatusCode)
}

func FetchManifest(ctx context.Context, client *http.Client, feedURL string) (Manifest, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return Manifest{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Cache-Control", "no-cache")
	response, err := client.Do(request)
	if err != nil {
		return Manifest{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 8<<10))
		return Manifest{}, &HTTPError{StatusCode: response.StatusCode}
	}

	var manifest Manifest
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode update feed: %w", err)
	}
	manifest.Version = strings.TrimPrefix(strings.TrimSpace(manifest.Version), "v")
	if manifest.Version == "" {
		return Manifest{}, errors.New("update feed has no version")
	}
	return manifest, nil
}

func (m Manifest) Asset(key string) (Asset, error) {
	asset, ok := m.Assets[key]
	if !ok {
		return Asset{}, fmt.Errorf("release %s has no %s asset", m.Version, key)
	}
	if strings.TrimSpace(asset.URL) == "" {
		return Asset{}, fmt.Errorf("release %s has no download URL for %s", m.Version, key)
	}
	parsed, err := url.Parse(asset.URL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return Asset{}, fmt.Errorf("release %s has an invalid download URL", m.Version)
	}
	if asset.Size <= 0 || asset.Size > maxDownloadSize {
		return Asset{}, fmt.Errorf("release %s has an invalid download size", m.Version)
	}
	if len(asset.SHA256) != 64 {
		return Asset{}, fmt.Errorf("release %s has an invalid SHA-256", m.Version)
	}
	if _, err := hex.DecodeString(asset.SHA256); err != nil {
		return Asset{}, fmt.Errorf("release %s has an invalid SHA-256", m.Version)
	}
	return asset, nil
}

type Progress func(downloaded, total int64)

func Verify(path string, asset Asset) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	digest := sha256.New()
	size, err := io.Copy(digest, io.LimitReader(file, asset.Size+1))
	if err != nil {
		return err
	}
	if size != asset.Size || !strings.EqualFold(hex.EncodeToString(digest.Sum(nil)), asset.SHA256) {
		return errors.New("更新文件校验失败，请重新下载")
	}
	return nil
}

func Download(ctx context.Context, client *http.Client, asset Asset, destination string, progress Progress) error {
	if Verify(destination, asset) == nil {
		if progress != nil {
			progress(asset.Size, asset.Size)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	temporary := destination + ".part"
	_ = removeWithRetry(temporary)

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		lastErr = downloadOnce(ctx, client, asset, temporary, progress)
		if lastErr == nil {
			break
		}
		_ = removeWithRetry(temporary)
	}
	if lastErr != nil {
		return lastErr
	}
	if err := removeWithRetry(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func downloadOnce(ctx context.Context, client *http.Client, asset Asset, destination string, progress Progress) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("Cache-Control", "no-cache")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 8<<10))
		return fmt.Errorf("download update: HTTP %d", response.StatusCode)
	}

	file, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	digest := sha256.New()
	total := asset.Size
	if response.ContentLength > 0 {
		total = response.ContentLength
	}
	reader := io.LimitReader(response.Body, asset.Size+1)
	counter := &progressReader{reader: reader, total: total, progress: progress}
	written, copyErr := io.Copy(io.MultiWriter(file, digest), counter)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != asset.Size {
		return fmt.Errorf("download update: got %d bytes, expected %d", written, asset.Size)
	}
	sum := hex.EncodeToString(digest.Sum(nil))
	if !strings.EqualFold(sum, asset.SHA256) {
		return errors.New("download update: SHA-256 mismatch")
	}
	return nil
}

type progressReader struct {
	reader   io.Reader
	done     int64
	total    int64
	progress Progress
	last     time.Time
}

func (r *progressReader) Read(buffer []byte) (int, error) {
	n, err := r.reader.Read(buffer)
	r.done += int64(n)
	now := time.Now()
	if r.progress != nil && (r.last.IsZero() || now.Sub(r.last) >= 150*time.Millisecond || err == io.EOF) {
		r.last = now
		r.progress(r.done, r.total)
	}
	return n, err
}
