package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
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
	if q.uds != "" {
		return q.uds, nil
	}
	if err := q.refresh(); err != nil {
		return "", fmt.Errorf("refresh qbittorrent parameters: %w", err)
	}
	return q.uds, nil
}

// refresh reads qBittorrent's current socket path and recovers its password.
// Newer fnOS builds mask the password in qBittorrent's argv, so the fallback
// extracts the login body retained by dlcenter in its own memory.
func (q *Qbit) refresh() error {
	pid, cmdline, err := findQbittorrentProcess()
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}
	if q.uds, err = getCmdParams(cmdline, "webui-sock-path"); err != nil {
		return fmt.Errorf("get webui-sock-path: %w", err)
	}
	password, err := getCmdParams(cmdline, "webui-password")
	if err != nil {
		return fmt.Errorf("get webui-password: %w", err)
	}
	if isMasked(password) {
		password, err = findPasswordFromDlcenter(pid)
		if err != nil {
			return fmt.Errorf("recover masked webui-password: %w", err)
		}
	}
	q.password = password
	return nil
}

func findQbittorrentProcess() (int, string, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, "", err
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
		if err != nil || !strings.HasSuffix(exe, "/trim-qbittorrent-nox") {
			continue
		}
		content, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil {
			continue
		}
		return pid, strings.ReplaceAll(string(content), "\x00", " "), nil
	}
	return 0, "", fmt.Errorf("qbittorrent-nox process not found")
}

func getCmdParams(cmd string, parameter string) (string, error) {
	re := regexp.MustCompile(fmt.Sprintf(`--%s=(\S+)`, parameter))
	matches := re.FindStringSubmatch(cmd)
	if len(matches) > 1 {
		return matches[1], nil
	}
	return "", fmt.Errorf("parameter --%s not found", parameter)
}

const (
	loginBodyPrefix = "username=admin&password="
	maxPasswordLen  = 64
	maxMapSize      = 512 << 20
	scanChunk       = 1 << 20
	passwordWait    = 5 * time.Second
)

func isMasked(s string) bool {
	return len(s) > 0 && strings.Trim(s, "*") == ""
}

func findPasswordFromDlcenter(qbPID int) (string, error) {
	pid, err := findProcessByExe("dlcenter")
	if err != nil {
		return "", err
	}
	deadline := time.Now().Add(passwordWait)
	for {
		passwords, err := scanProcessMemory(pid, loginBodyPrefix)
		if err != nil {
			return "", err
		}
		for _, password := range passwords {
			if len(password) >= 4 && len(password) <= maxPasswordLen && !isMasked(password) {
				return password, nil
			}
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("no login request found in dlcenter (pid %d) for qBittorrent pid %d; run as root", pid, qbPID)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func findProcessByExe(name string) (int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, err
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
		if err == nil && strings.HasSuffix(exe, "/"+name) {
			return pid, nil
		}
	}
	return 0, fmt.Errorf("%s process not found", name)
}

func scanProcessMemory(pid int, prefix string) ([]string, error) {
	mapsData, err := os.ReadFile(fmt.Sprintf("/proc/%d/maps", pid))
	if err != nil {
		return nil, fmt.Errorf("read maps: %w", err)
	}
	mem, err := os.Open(fmt.Sprintf("/proc/%d/mem", pid))
	if err != nil {
		return nil, fmt.Errorf("open memory: %w", err)
	}
	defer mem.Close()

	needle := []byte(prefix)
	var passwords []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(mapsData), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasPrefix(fields[1], "r") {
			continue
		}
		startStr, endStr, ok := strings.Cut(fields[0], "-")
		if !ok {
			continue
		}
		start, errStart := strconv.ParseInt(startStr, 16, 64)
		end, errEnd := strconv.ParseInt(endStr, 16, 64)
		if errStart != nil || errEnd != nil || end <= start || end-start > maxMapSize {
			continue
		}
		scanRegion(mem, start, end, needle, func(password string) {
			if !seen[password] {
				seen[password] = true
				passwords = append(passwords, password)
			}
		})
	}
	return passwords, nil
}

func scanRegion(mem *os.File, start, end int64, needle []byte, found func(string)) {
	overlap := len(needle) + maxPasswordLen
	buf := make([]byte, scanChunk+overlap)
	pos := start
	for pos < end {
		read := int64(len(buf))
		if end-pos < read {
			read = end - pos
		}
		n, err := mem.ReadAt(buf[:read], pos)
		if n > 0 {
			data := buf[:n]
			for idx := 0; idx < len(data); {
				i := strings.Index(string(data[idx:]), string(needle))
				if i < 0 {
					break
				}
				idx += i + len(needle)
				if password := readPrintable(data[idx:]); password != "" {
					found(password)
				}
			}
		}
		if err != nil && err != io.EOF {
			return
		}
		advance := int64(n) - int64(overlap)
		if advance <= 0 {
			return
		}
		pos += advance
	}
}

func readPrintable(data []byte) string {
	end := 0
	for end < len(data) && end < maxPasswordLen {
		if data[end] < 0x21 || data[end] > 0x7e {
			break
		}
		end++
	}
	return string(data[:end])
}
