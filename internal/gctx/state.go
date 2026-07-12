package gctx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	adcFilename        = "application_default_credentials.json"
	stateFilename      = ".gctx-state.json"
	credentialFileMode = 0o600
)

type previousState struct {
	Previous string `json:"previous"`
}

func readPreviousState(directory string) (string, error) {
	path := filepath.Join(directory, stateFilename)
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", errors.New("no previous gcloud configuration is recorded; switch contexts once before using gctx -")
	}
	if err != nil {
		return "", fmt.Errorf("open previous-context state: %w", err)
	}
	var state previousState
	decodeErr := json.NewDecoder(file).Decode(&state)
	closeErr := file.Close()
	if decodeErr != nil {
		return "", fmt.Errorf("decode previous-context state: %w", decodeErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close previous-context state: %w", closeErr)
	}
	if state.Previous == "" {
		return "", errors.New("previous-context state does not contain a configuration name")
	}
	return state.Previous, nil
}

type stagedFile struct {
	temporary string
	final     string
}

func stagePreviousState(directory, previous string) (*stagedFile, error) {
	file, err := os.CreateTemp(directory, ".gctx-state-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("stage previous-context state: %w", err)
	}
	temporary := file.Name()
	keep := false
	defer func() {
		if !keep {
			discardTemporary(file, temporary)
		}
	}()
	if err := file.Chmod(credentialFileMode); err != nil {
		return nil, fmt.Errorf("secure previous-context state: %w", err)
	}
	if err := json.NewEncoder(file).Encode(previousState{Previous: previous}); err != nil {
		return nil, fmt.Errorf("encode previous-context state: %w", err)
	}
	if err := file.Sync(); err != nil {
		return nil, fmt.Errorf("sync previous-context state: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close previous-context state: %w", err)
	}
	keep = true
	return &stagedFile{temporary: temporary, final: filepath.Join(directory, stateFilename)}, nil
}

func (state *stagedFile) commit() error {
	if state == nil {
		return nil
	}
	if err := os.Rename(state.temporary, state.final); err != nil {
		return fmt.Errorf("commit previous-context state: %w", err)
	}
	state.temporary = ""
	return syncDirectory(filepath.Dir(state.final))
}

func (state *stagedFile) remove() {
	if state != nil && state.temporary != "" {
		if err := os.Remove(state.temporary); err != nil && !errors.Is(err, os.ErrNotExist) {
			return
		}
	}
}

type adcBackup struct {
	adcPath   string
	temporary string
	existed   bool
}

func backupADC(directory string) (*adcBackup, error) {
	adcPath := filepath.Join(directory, adcFilename)
	source, err := os.Open(adcPath)
	if errors.Is(err, os.ErrNotExist) {
		return &adcBackup{adcPath: adcPath}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open ADC for backup: %w", err)
	}
	defer closeChecked(source)
	destination, err := os.CreateTemp(directory, ".gctx-adc-*.tmp")
	if err != nil {
		if closeErr := source.Close(); closeErr != nil {
			return nil, errors.Join(
				fmt.Errorf("create ADC backup: %w", err),
				fmt.Errorf("close ADC source: %w", closeErr),
			)
		}
		return nil, fmt.Errorf("create ADC backup: %w", err)
	}
	temporary := destination.Name()
	keep := false
	defer func() {
		if !keep {
			discardTemporary(destination, temporary)
		}
	}()
	if err := destination.Chmod(credentialFileMode); err != nil {
		return nil, fmt.Errorf("secure ADC backup: %w", err)
	}
	if _, err := io.Copy(destination, source); err != nil {
		return nil, fmt.Errorf("copy ADC backup: %w", err)
	}
	if err := destination.Sync(); err != nil {
		return nil, fmt.Errorf("sync ADC backup: %w", err)
	}
	if err := destination.Close(); err != nil {
		return nil, fmt.Errorf("close ADC backup: %w", err)
	}
	if err := source.Close(); err != nil {
		return nil, fmt.Errorf("close ADC source: %w", err)
	}
	keep = true
	return &adcBackup{adcPath: adcPath, temporary: temporary, existed: true}, nil
}

func (backup *adcBackup) restore() error {
	if backup.existed {
		if err := os.Rename(backup.temporary, backup.adcPath); err != nil {
			return fmt.Errorf("restore ADC from %s: %w", backup.temporary, err)
		}
		backup.temporary = ""
		if err := os.Chmod(backup.adcPath, credentialFileMode); err != nil {
			return fmt.Errorf("secure restored ADC: %w", err)
		}
		return syncDirectory(filepath.Dir(backup.adcPath))
	}
	if err := os.Remove(backup.adcPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove newly created ADC: %w", err)
	}
	return syncDirectory(filepath.Dir(backup.adcPath))
}

func (backup *adcBackup) remove() error {
	if backup.temporary == "" {
		return nil
	}
	err := os.Remove(backup.temporary)
	if err == nil {
		backup.temporary = ""
		return nil
	}
	return fmt.Errorf("remove ADC backup: %w", err)
}

func syncDirectory(directory string) error {
	file, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open directory for sync: %w", err)
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if syncErr != nil {
		return fmt.Errorf("sync directory: %w", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close synced directory: %w", closeErr)
	}
	return nil
}

func discardTemporary(file *os.File, path string) {
	closeErr := file.Close()
	removeErr := os.Remove(path)
	if closeErr != nil || (removeErr != nil && !errors.Is(removeErr, os.ErrNotExist)) {
		return
	}
}

func closeChecked(file *os.File) {
	if err := file.Close(); err != nil {
		return
	}
}
