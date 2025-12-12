package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

type QbMonitor struct {
	process  *os.Process
	password string
	uds      string
}

// thread-unsafe, but it's ok.
func NewQbMonitor() *QbMonitor {
	return &QbMonitor{}
}

func (q *QbMonitor) check() error {
	if !processExists(q.process) {
		if err := q.update(); err != nil {
			return fmt.Errorf("update qbittorrent-nox parameters: %w", err)
		}
	}

	return nil
}

func processExists(p *os.Process) bool {
	if p == nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func (q *QbMonitor) GetPassword() (string, error) {
	if err := q.check(); err != nil {
		return "", err
	}
	return q.password, nil
}

func (q *QbMonitor) GetUds() (string, error) {
	if err := q.check(); err != nil {
		return "", err
	}
	return q.uds, nil
}

func (q *QbMonitor) update() error {
	fmt.Printf("updating qbittorrent-nox parameters\n")
	pid, cmdline, err := findQbittorrentProcess()
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}
	q.process, err = os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process by pid %d: %w", pid, err)
	}

	if q.password, err = getParameter(cmdline, "webui-password"); err != nil {
		return fmt.Errorf("get webui-password: %w", err)
	}
	if q.uds, err = getParameter(cmdline, "webui-sock-path"); err != nil {
		return fmt.Errorf("get webui-sock-path: %w", err)
	}
	return nil
}

// Linux only: find qbittorrent-nox process by scanning /proc
func findQbittorrentProcess() (int, string, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, "", err
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
			return pid, cmdline, nil
		}
	}
	return 0, "", fmt.Errorf("qbittorrent-nox process not found")
}

func getParameter(cmd string, parameter string) (string, error) {

	// parse output(likes --webui-password=xxx) to get
	re := regexp.MustCompile(fmt.Sprintf(`--%s=(\S+)`, parameter))
	matches := re.FindStringSubmatch(cmd)
	if len(matches) > 1 {
		return matches[1], nil
	}

	return "", fmt.Errorf("no qbittorrent-nox process found")
}
