package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
)

func fetchQbPassword() (string, error) {
	// exec command "ps aux | grep [q]bittorrent-nox"
	cmd := exec.Command("bash", "-c", "ps aux | grep [q]bittorrent-nox")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("exec command %s: %w", cmd.String(), err)
	}

	// parse output(likes --webui-password=xxx) to get password
	re := regexp.MustCompile(`--webui-password=(\S+)`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) > 1 {
		return matches[1], nil
	}

	return "", fmt.Errorf("no qbittorrent-nox process found")
}

func fetchQbSid(uds string, password string) (string, error) {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", uds)
			},
		},
	}

	data := url.Values{}
	data.Set("username", "admin")
	data.Set("password", password)

	req, err := http.NewRequest("POST", "http://unix/api/v2/auth/login", strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	for _, cookie := range resp.Cookies() {
		if cookie.Name == "SID" {
			return cookie.Value, nil
		}
	}

	return "", fmt.Errorf("no SID cookie found")
}

func watchQbPassword(ch chan string) {
	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		password, err := fetchQbPassword()
		if err != nil {
			fmt.Printf("fetch qbittorrent-nox password: %v\n", err)
			continue
		}

		ch <- password
	}
}

func proxyCmd(ctx *cli.Context) error {
	uds := ctx.String("uds")
	debug := ctx.Bool("debug")
	port := ctx.Int("port")
	expectedPassword := ctx.String("password")
	password, err := fetchQbPassword()
	if err != nil {
		return fmt.Errorf("fetch qbittorrent-nox password: %w", err)
	}
	updateSid := func() string {
		if expectedPassword != "" {
			return ""
		}
		sid, err := fetchQbSid(uds, password)
		if err != nil {
			fmt.Printf("fetch qbittorrent-nox sid: %v\n", err)
		}
		return sid
	}

	sid := updateSid()

	debugf := func(format string, args ...any) {
		if debug {
			fmt.Printf(format, args...)
		}
	}

	fmt.Printf("proxy running on port %d\n", port)

	ch := make(chan string)
	go watchQbPassword(ch)
	go func() {
		for newPassword := range ch {
			if newPassword == password {
				continue
			}
			password = newPassword
			sid = updateSid()
			fmt.Printf("new password: %s\n", password)
			fmt.Printf("new sid: %s\n", sid)
		}
	}()

	proxy := httputil.ReverseProxy{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", uds)
			},
		},
		Rewrite: func(r *httputil.ProxyRequest) {
			debugf("request: %v\n", r.In.URL.Path)
			r.Out.URL.Scheme = "http"
			r.Out.URL.Host = fmt.Sprintf("file://%s", uds)
			r.Out.Host = fmt.Sprintf("file://%s", uds)
			if sid != "" {
				// Always set the correct SID for authentication
				r.Out.AddCookie(&http.Cookie{
					Name:  "SID",
					Value: sid,
				})
			}

			body, _ := io.ReadAll(r.In.Body)
			r.In.ParseForm()
			if strings.Contains(r.In.URL.Path, "/api/v2/auth/login") {
				outPassword := password
				if expectedPassword != "" {
					parts := strings.Split(string(body), "&")
					debugf("parts: %v\n", parts)
					for _, part := range parts {
						if strings.HasPrefix(part, "password=") {
							inputPassword := strings.TrimPrefix(part, "password=")
							if inputPassword != expectedPassword {
								outPassword = ""
								break
							}
						}
					}
				}

				body = []byte(fmt.Sprintf("username=admin&password=%s", outPassword))
				r.Out.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
				r.Out.ContentLength = int64(len(body))
			}
			r.Out.Header.Del("Referer")
			r.Out.Header.Del("Origin")
			r.Out.Body = io.NopCloser(bytes.NewBuffer(body))
		},
	}
	err = http.ListenAndServe(fmt.Sprintf(":%d", port), &proxy)
	if err != nil {
		return fmt.Errorf("listen and serve: %w", err)
	}
	return nil
}

func main() {
	app := &cli.App{
		Name:   "fnos-qb-proxy",
		Usage:  "fnos-qb-proxy is a proxy for qBittorrent in fnOS",
		Action: proxyCmd,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "password",
				Aliases: []string{"p"},
				Usage:   "if not set, any password will be accepted",
				Value:   "",
			},
			&cli.StringFlag{
				Name:  "uds",
				Usage: "qBittorrent unix domain socket(uds) path",
				Value: "/home/admin/qbt.sock",
			},
			&cli.BoolFlag{
				Name:    "debug",
				Aliases: []string{"d"},
				Value:   false,
			},
			&cli.IntFlag{
				Name:  "port",
				Usage: "proxy running port",
				Value: 8080,
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
