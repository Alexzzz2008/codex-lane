package selfupdate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type recoveryRecord struct {
	Backup       string `json:"backup"`
	Marker       string `json:"marker"`
	InstallerPID int    `json:"installerPid"`
}

func writeRecovery(executable, backup string, installerPID int) error {
	file, err := os.CreateTemp(filepath.Dir(executable), ".codexlane-ready-*")
	if err != nil {
		return err
	}
	marker := file.Name()
	_ = file.Close()
	_ = os.Remove(marker)
	data, err := json.Marshal(recoveryRecord{Backup: backup, Marker: marker, InstallerPID: installerPID})
	if err != nil {
		return err
	}
	return os.WriteFile(executable+".update.json", data, 0o600)
}

func readRecovery(executable string) (recoveryRecord, error) {
	var record recoveryRecord
	data, err := os.ReadFile(executable + ".update.json")
	if err != nil {
		return record, err
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return record, err
	}
	if filepath.Dir(record.Backup) != filepath.Dir(executable) ||
		!strings.HasPrefix(record.Backup, executable+".old") ||
		filepath.Dir(record.Marker) != filepath.Dir(executable) ||
		!strings.HasPrefix(filepath.Base(record.Marker), ".codexlane-ready-") {
		return record, errors.New("invalid update recovery record")
	}
	return record, nil
}

func clearRecovery(executable string) {
	if record, err := readRecovery(executable); err == nil {
		_ = os.Remove(record.Marker)
	}
	_ = os.Remove(executable + ".update.json")
}

func ArmRecovery(executable string) error {
	if _, err := readRecovery(executable); errors.Is(err, os.ErrNotExist) {
		backup, exists := latestBackup(executable)
		if !exists || PredecessorPID() == 0 {
			return nil
		}
		if err := writeRecovery(executable, backup, PredecessorPID()); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	command := exec.Command(executable, "--update-guard", executable, strconv.Itoa(os.Getpid()))
	command.Env = cleanReplacementEnv()
	detach(command)
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}

func ReportReady(executable string) error {
	record, err := readRecovery(executable)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return os.WriteFile(record.Marker, []byte(strconv.Itoa(os.Getpid())), 0o600)
}

func waitReady(marker string, pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if data, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(data)) == strconv.Itoa(pid) {
			return nil
		}
		if !processAlive(pid) {
			return errors.New("更新进程在界面就绪前退出")
		}
		if time.Now().After(deadline) {
			return errors.New("更新后界面未能及时就绪")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func GuardUpdate(executable string, pid int, timeout time.Duration) error {
	if pid <= 0 || pid == os.Getpid() {
		return errors.New("invalid update process")
	}
	record, err := readRecovery(executable)
	if err != nil {
		return err
	}
	if err := waitReady(record.Marker, pid, timeout); err == nil {
		clearRecovery(executable)
		CleanupOld(executable)
		return nil
	}
	for deadline := time.Now().Add(25 * time.Second); record.InstallerPID != pid && processAlive(record.InstallerPID) && time.Now().Before(deadline); {
		if _, err := os.Stat(executable + ".update.json"); errors.Is(err, os.ErrNotExist) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := os.Stat(record.Backup); errors.Is(err, os.ErrNotExist) {
		clearRecovery(executable)
		return nil
	}
	if processAlive(pid) {
		process, err := os.FindProcess(pid)
		if err != nil {
			return err
		}
		killErr := process.Kill()
		_ = process.Release()
		if killErr != nil {
			return killErr
		}
		for limit := time.Now().Add(5 * time.Second); processAlive(pid) && time.Now().Before(limit); {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if err := Rollback(executable, record.Backup); err != nil {
		return fmt.Errorf("更新恢复失败：%w", err)
	}
	return StartDetached(executable)
}

func cleanReplacementEnv() []string {
	var env []string
	for _, value := range os.Environ() {
		key := strings.SplitN(value, "=", 2)[0]
		if strings.EqualFold(key, predecessorEnv) || strings.EqualFold(key, startedEnv) {
			continue
		}
		env = append(env, value)
	}
	return env
}
