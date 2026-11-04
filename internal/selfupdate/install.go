package selfupdate

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

func Executable() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(executable)
}

func Writable(directory string) bool {
	file, err := os.CreateTemp(directory, ".codexlane-update-*")
	if err != nil {
		return false
	}
	name := file.Name()
	_ = file.Close()
	_ = os.Remove(name)
	return true
}

func Install(staged, executable string) (string, error) {
	if _, err := os.Stat(staged); err != nil {
		return "", fmt.Errorf("staged update: %w", err)
	}
	directory := filepath.Dir(executable)
	if !Writable(directory) {
		return "", fmt.Errorf("update directory is not writable: %s", directory)
	}
	if err := canStart(staged); err != nil {
		return "", fmt.Errorf("Windows refused to start the new version: %w", err)
	}

	next := fmt.Sprintf("%s.new-%d", executable, os.Getpid())
	_ = removeWithRetry(next)
	if err := copyFile(staged, next); err != nil {
		_ = removeWithRetry(next)
		return "", fmt.Errorf("prepare update: %w", err)
	}
	hideFile(next)
	defer removeWithRetry(next)

	old := oldName(executable)
	if err := moveWithRetry(executable, old); err != nil {
		return "", fmt.Errorf("move running executable aside: %w", err)
	}
	if err := moveWithRetry(next, executable); err != nil {
		if restoreErr := moveWithRetry(old, executable); restoreErr != nil {
			return "", errors.Join(err, fmt.Errorf("restore previous executable: %w", restoreErr))
		}
		return "", fmt.Errorf("put new executable in place: %w", err)
	}
	// The staged copy is hidden while it sits next to the running executable;
	// clear the attribute so the installed file stays visible in Explorer.
	unhideFile(executable)
	if err := writeRecovery(executable, old, os.Getpid()); err != nil {
		return "", errors.Join(fmt.Errorf("create update recovery: %w", err), Rollback(executable, old))
	}
	_ = removeWithRetry(staged)
	return old, nil
}

var canStart = trialStart

func Rollback(executable, old string) error {
	if old == "" {
		return errors.New("no previous executable to restore")
	}
	if _, err := os.Stat(old); err != nil {
		return fmt.Errorf("previous executable unavailable: %w", err)
	}
	if err := removeWithRetry(executable); err != nil && !errors.Is(err, os.ErrNotExist) {
		if renameErr := moveWithRetry(executable, executable+".blocked"); renameErr != nil {
			return fmt.Errorf("remove failed update: %w", err)
		}
	}
	if err := moveWithRetry(old, executable); err != nil {
		return fmt.Errorf("restore previous executable: %w", err)
	}
	unhideFile(executable)
	clearRecovery(executable)
	return nil
}

func CleanupOld(executable string) {
	if _, err := os.Stat(executable + ".update.json"); err == nil {
		return
	}
	removeStaleOld(executable + ".old")
	matches, _ := filepath.Glob(executable + ".old-*")
	for _, match := range matches {
		removeStaleOld(match)
	}
}

// removeStaleOld keeps recent backups so a failed start still has something to
// fall back to; older ones are removed on the next launch.
func removeStaleOld(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if time.Since(info.ModTime()) < 10*time.Minute {
		return
	}
	_ = removeWithRetry(path)
}

// RestorePrevious puts the backup created by Install back in place.
func RestorePrevious(executable string) (string, error) {
	old, ok := latestBackup(executable)
	if !ok {
		return "", errors.New("no previous executable")
	}
	if err := removeWithRetry(executable); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("remove current executable: %w", err)
	}
	if err := moveWithRetry(old, executable); err != nil {
		return "", fmt.Errorf("restore previous executable: %w", err)
	}
	unhideFile(executable)
	clearRecovery(executable)
	return executable, nil
}

// latestBackup returns the newest executable backup written by Install.
func latestBackup(executable string) (string, bool) {
	candidates := []string{executable + ".old"}
	matches, _ := filepath.Glob(executable + ".old-*")
	candidates = append(candidates, matches...)
	var newest string
	var newestTime time.Time
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		if newest == "" || info.ModTime().After(newestTime) {
			newest = candidate
			newestTime = info.ModTime()
		}
	}
	return newest, newest != ""
}

func oldName(executable string) string {
	name := executable + ".old"
	for index := 2; ; index++ {
		if _, err := os.Lstat(name); err != nil {
			return name
		}
		name = fmt.Sprintf("%s.old-%d", executable, index)
	}
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	syncErr := output.Sync()
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

var retryWaits = []time.Duration{
	100 * time.Millisecond,
	200 * time.Millisecond,
	400 * time.Millisecond,
	800 * time.Millisecond,
	1500 * time.Millisecond,
	1500 * time.Millisecond,
	1500 * time.Millisecond,
}

func moveWithRetry(from, to string) error {
	var err error
	for attempt := 0; ; attempt++ {
		err = os.Rename(from, to)
		if err == nil {
			return nil
		}
		if attempt >= len(retryWaits) || !sharingViolation(err) {
			return err
		}
		time.Sleep(retryWaits[attempt])
	}
}

func removeWithRetry(path string) error {
	var err error
	for attempt := 0; ; attempt++ {
		err = os.Remove(path)
		if err == nil || errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if attempt >= len(retryWaits) || !sharingViolation(err) {
			return err
		}
		time.Sleep(retryWaits[attempt])
	}
}
