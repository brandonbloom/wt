package processes

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

var (
	ErrUnsupported         = errors.New("process detection unsupported")
	testDataInlineEnv      = "WT_PROCESS_TEST_DATA"
	testDataFileEnv        = "WT_PROCESS_TEST_DATA_FILE"
	minimumCommandFallback = "process"
)

type Process struct {
	PID     int    `json:"pid"`
	Command string `json:"command"`
	CWD     string `json:"cwd"`
	PPID    int    `json:"ppid"`
}

func List() ([]Process, error) {
	if procs, ok, err := fromTestData(); err != nil || ok {
		return procs, err
	}
	return listNative(os.Getuid())
}

// TestDataFilePath reports the file path used for WT_PROCESS_TEST_DATA_FILE, if any.
func TestDataFilePath() string {
	return os.Getenv(testDataFileEnv)
}

func fromTestData() ([]Process, bool, error) {
	if path := os.Getenv(testDataFileEnv); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, true, fmt.Errorf("read %s: %w", path, err)
		}
		procs, err := decodeTestData(data)
		return procs, true, err
	}
	if data := os.Getenv(testDataInlineEnv); data != "" {
		procs, err := decodeTestData([]byte(data))
		return procs, true, err
	}
	return nil, false, nil
}

func decodeTestData(data []byte) ([]Process, error) {
	var procs []Process
	if err := json.Unmarshal(data, &procs); err != nil {
		return nil, fmt.Errorf("parse WT process test data: %w", err)
	}
	return procs, nil
}

func sanitizeCommand(cmd string, pid int) string {
	if cmd != "" {
		return cmd
	}
	return fmt.Sprintf("%s-%d", minimumCommandFallback, pid)
}
