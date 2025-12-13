package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Qbit struct {
	password string
	uds      string
}

func NewQbit() *Qbit {
	return &Qbit{}
}

func (q *Qbit) GetPassword() (string, error) {
	if err := q.refresh(); err != nil {
		return "", fmt.Errorf("refresh qbittorrent parameters: %w", err)
	}
	return q.password, nil
}

func (q *Qbit) GetUds() (string, error) {
	// uds should not be changed once set
	if q.uds != "" {
		return q.uds, nil
	}
	if err := q.refresh(); err != nil {
		return "", fmt.Errorf("refresh qbittorrent parameters: %w", err)
	}
	return q.uds, nil
}

// refresh checks if the qbittorrent-nox process is running and updates password and uds path
func (q *Qbit) refresh() error {
	fmt.Printf("refreshing qbittorrent-nox parameters\n")
	cmdline, err := findQbittorrentProcess()
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}

	if q.password, err = getCmdParams(cmdline, "webui-password"); err != nil {
		return fmt.Errorf("get webui-password: %w", err)
	}
	if q.uds, err = getCmdParams(cmdline, "webui-sock-path"); err != nil {
		return fmt.Errorf("get webui-sock-path: %w", err)
	}
	return nil
}

func findQbittorrentProcess() (string, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		content, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil {
			continue
		}
		// cmdline arguments are separated by null bytes
		cmdline := strings.ReplaceAll(string(content), "\x00", " ")
		if strings.Contains(cmdline, "trim-qbittorrent-nox") {
			return cmdline, nil
		}
	}
	return "", fmt.Errorf("qbittorrent-nox process not found")
}

func getCmdParams(cmd string, parameter string) (string, error) {
	// parse output(likes --webui-password=xxx) to get
	re := regexp.MustCompile(fmt.Sprintf(`--%s=(\S+)`, parameter))
	matches := re.FindStringSubmatch(cmd)
	if len(matches) > 1 {
		return matches[1], nil
	}

	return "", fmt.Errorf("no qbittorrent-nox process found")
}
