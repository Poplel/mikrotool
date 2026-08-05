package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"

	"mikrotool/internal/inputcheck"
	"mikrotool/internal/store"
	"mikrotool/internal/winbox"
)

const legacyAppID = "com.wirestar.wiretool"

func migrateLegacyData(configDir, destination string) []error {
	source := filepath.Join(configDir, "Wiretool")
	if source == destination {
		return nil
	}
	sourceInfo, err := os.Lstat(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return []error{fmt.Errorf("inspect legacy data directory: %w", err)}
	} else if !sourceInfo.IsDir() || sourceInfo.Mode()&os.ModeSymlink != 0 {
		return []error{fmt.Errorf("legacy data path is not a safe directory")}
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return []error{fmt.Errorf("create Mikrotool data directory: %w", err)}
	}
	destinationInfo, err := os.Lstat(destination)
	if err != nil || !destinationInfo.IsDir() || destinationInfo.Mode()&os.ModeSymlink != 0 {
		return []error{fmt.Errorf("Mikrotool data path is not a safe directory")}
	}
	var migrationErrors []error
	for _, file := range []struct {
		name string
		max  int64
	}{
		{name: "sites.csv", max: 16 << 20},
		{name: "known_hosts", max: 16 << 20},
		{name: "actions.csv", max: 16 << 20},
	} {
		if err := copyLegacyRegularFile(filepath.Join(source, file.name), filepath.Join(destination, file.name), file.max); err != nil && !errors.Is(err, os.ErrNotExist) {
			migrationErrors = append(migrationErrors, fmt.Errorf("migrate %s: %w", file.name, err))
		}
	}
	if err := migrateLegacyRecovery(source, destination); err != nil {
		migrationErrors = append(migrationErrors, err)
	}
	return migrationErrors
}

func migrateLegacyRecovery(source, destination string) error {
	destinationPath := filepath.Join(destination, "active-session.json")
	if _, err := os.Lstat(destinationPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	value, err := store.NewRecoveryStore(filepath.Join(source, "active-session.json")).Load()
	if err != nil || value == nil {
		if errors.Is(err, os.ErrNotExist) || value == nil {
			return nil
		}
		return fmt.Errorf("read legacy interrupted session: %w", err)
	}
	destinationConfig := filepath.Join(destination, "active", value.TunnelName+".conf")
	legacyConfig := filepath.Join(source, "active", value.TunnelName+".conf")
	if err := copyLegacyRegularFile(legacyConfig, destinationConfig, 64<<10); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("migrate private interrupted-session config: %w", err)
	}
	// A normalized destination path also lets recovery safely recognize a
	// missing legacy config as already removed instead of treating the old path
	// as an arbitrary file supplied by persisted data.
	value.ConfigPath = destinationConfig
	if err := store.NewRecoveryStore(destinationPath).Save(*value); err != nil {
		return fmt.Errorf("migrate interrupted session: %w", err)
	}
	return nil
}

func copyLegacyRegularFile(source, destination string, maximum int64) error {
	if _, err := os.Lstat(destination); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file")
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	openedInfo, err := input.Stat()
	if err != nil || !os.SameFile(info, openedInfo) || !openedInfo.Mode().IsRegular() {
		return fmt.Errorf("source changed while it was being opened")
	}
	data, err := io.ReadAll(io.LimitReader(input, maximum+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > maximum {
		return fmt.Errorf("source exceeds the migration size limit")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".mikrotool-migrate-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, destination); err != nil {
		return err
	}
	committed = true
	return os.Chmod(destination, 0o600)
}

func migrateLegacyPreferences(application fyne.App) error {
	root, err := legacyFynePreferencesRoot()
	if err != nil {
		return err
	}
	path := filepath.Join(root, legacyAppID, "preferences.json")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open legacy preferences: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if err != nil {
		return fmt.Errorf("read legacy preferences: %w", err)
	}
	if len(data) > 64<<10 {
		return fmt.Errorf("legacy preferences exceed 64 KiB")
	}
	var values map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&values); err != nil {
		return fmt.Errorf("decode legacy preferences: %w", err)
	}
	preferences := application.Preferences()
	if preferences.String(prefUsername) == "" {
		if value, ok := legacyString(values, prefUsername); ok {
			if username, validationErr := inputcheck.Username(value); validationErr == nil {
				preferences.SetString(prefUsername, username)
			}
		}
	}
	if preferences.String(prefSSHPort) == "" {
		if value, ok := legacyString(values, prefSSHPort); ok {
			port, parseErr := strconv.ParseUint(strings.TrimSpace(value), 10, 16)
			if parseErr == nil && port > 0 {
				preferences.SetString(prefSSHPort, strconv.FormatUint(port, 10))
			}
		}
	}
	if runtime.GOOS == "windows" && preferences.String(prefWinBoxPath) == "" {
		if value, ok := legacyString(values, prefWinBoxPath); ok && winbox.ValidateExecutablePath(value) == nil {
			preferences.SetString(prefWinBoxPath, strings.TrimSpace(value))
		}
	}
	return nil
}

func legacyString(values map[string]json.RawMessage, key string) (string, bool) {
	value, ok := values[key]
	if !ok {
		return "", false
	}
	var decoded string
	if err := json.Unmarshal(value, &decoded); err != nil {
		return "", false
	}
	return decoded, true
}

func legacyFynePreferencesRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory for preference migration: %w", err)
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Preferences", "fyne"), nil
	case "windows":
		return filepath.Join(home, "AppData", "Roaming", "fyne"), nil
	default:
		config, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(config, "fyne"), nil
	}
}
