//go:build linux

package processes

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func listNative(uid int) ([]Process, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrUnsupported
		}
		return nil, err
	}

	procs := make([]Process, 0, len(entries))
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		meta, err := readProcMetadata(entry.Name())
		if err != nil || meta == nil || meta.uid != uid {
			continue
		}

		cwd, err := os.Readlink(filepath.Join("/proc", entry.Name(), "cwd"))
		if err != nil || cwd == "" {
			continue
		}
		cwd = strings.TrimSuffix(cwd, " (deleted)")

		command, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
		if err != nil || len(command) == 0 {
			command, err = readFirstLine(filepath.Join("/proc", entry.Name(), "cmdline"))
			if err != nil {
				continue
			}
		}
		cmd := strings.TrimSpace(string(command))
		cmd = sanitizeCommand(cmd, pid)

		procs = append(procs, Process{
			PID:     pid,
			PPID:    meta.ppid,
			Command: cmd,
			CWD:     cwd,
		})
	}

	return procs, nil
}

type procMetadata struct {
	uid     int
	ppid    int
	hasUID  bool
	hasPPID bool
}

func readProcMetadata(pid string) (*procMetadata, error) {
	statusPath := filepath.Join("/proc", pid, "status")
	file, err := os.Open(statusPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var meta procMetadata
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "Uid:"):
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if uid, err := strconv.Atoi(fields[1]); err == nil {
					meta.uid = uid
					meta.hasUID = true
				}
			}
		case strings.HasPrefix(line, "PPid:"):
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if ppid, err := strconv.Atoi(fields[1]); err == nil {
					meta.ppid = ppid
					meta.hasPPID = true
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !meta.hasUID {
		return nil, fmt.Errorf("missing metadata")
	}
	return &meta, nil
}

func readFirstLine(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("empty command line")
	}
	parts := strings.Split(string(data), "\x00")
	if len(parts) == 0 || parts[0] == "" {
		return nil, errors.New("empty command line")
	}
	return []byte(parts[0]), nil
}
