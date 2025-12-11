package main

import (
	"bytes"
	"compress/gzip"
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

type FnosProxy struct {
	*httputil.ReverseProxy
	uds              string
	debug            bool
	expectedPassword string
	password         string
	sid              string
	port             int
}

func NewFnosProxy(uds string, debug bool, expectedPassword string, password string, port int) *FnosProxy {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial("unix", uds)
		},
	}
	p := &FnosProxy{
		uds:              uds,
		debug:            debug,
		expectedPassword: expectedPassword,
		password:         password,
		port:             port,
	}
	p.ReverseProxy = &httputil.ReverseProxy{
		Transport:      transport,
		Rewrite:        p.Rewrite,
		ModifyResponse: p.ModifyResponse,
	}
	// initial SID
	if err := p.reloadSid(); err != nil {
		fmt.Printf("initial load SID: %v\n", err)
	}
	return p
}

// fetch makes an HTTP request to qBittorrent via unix socket
func (p *FnosProxy) fetch(method string, path string, body io.Reader, configure func(*http.Request)) (*http.Response, error) {
	client := &http.Client{Transport: p.Transport}
	req, err := http.NewRequest(method, "http://unix"+path, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Allow caller to configure the request
	if configure != nil {
		configure(req)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}

	return resp, nil
}

func (p *FnosProxy) debugf(format string, args ...any) {
	if p.debug {
		fmt.Printf(format, args...)
	}
}

func (p *FnosProxy) updateSidFromResp(resp *http.Response) error {
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "SID" {
			p.sid = cookie.Value
			p.debugf("updated sid: %s\n", p.sid)
			return nil
		}
	}
	return fmt.Errorf("no SID cookie found")
}

func (p *FnosProxy) reloadSid() error {
	if p.expectedPassword != "" {
		return nil
	}
	data := url.Values{}
	data.Set("username", "admin")
	data.Set("password", p.password)

	resp, err := p.fetch("POST", "/api/v2/auth/login", strings.NewReader(data.Encode()), func(req *http.Request) {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	})
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()

	if err := p.updateSidFromResp(resp); err != nil {
		return fmt.Errorf("update SID from cookies: %w", err)
	}
	return nil
}

func (p *FnosProxy) Rewrite(r *httputil.ProxyRequest) {
	p.debugf("request: %v\n", r.In.URL.Path)
	r.Out.URL.Scheme = "http"
	r.Out.URL.Host = fmt.Sprintf("file://%s", p.uds)
	r.Out.Host = fmt.Sprintf("file://%s", p.uds)
	if p.sid != "" {
		r.Out.AddCookie(&http.Cookie{
			Name:  "SID",
			Value: p.sid,
		})
	}

	body, _ := io.ReadAll(r.In.Body)
	r.In.ParseForm()
	if strings.Contains(r.In.URL.Path, "/api/v2/auth/login") {
		outPassword := p.password
		if p.expectedPassword != "" {
			parts := strings.Split(string(body), "&")
			p.debugf("parts: %v\n", parts)
			for _, part := range parts {
				if strings.HasPrefix(part, "password=") {
					inputPassword := strings.TrimPrefix(part, "password=")
					if inputPassword != p.expectedPassword {
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
}

func (p *FnosProxy) ModifyResponse(resp *http.Response) error {
	if p.expectedPassword != "" {
		return nil
	}
	// update SID from response cookies
	p.updateSidFromResp(resp)

	contentType := resp.Header.Get("Content-Type")
	// for any non-HTML response with 403 status, refresh SID
	if !strings.Contains(contentType, "text/html") && resp.StatusCode == http.StatusForbidden {
		return p.reloadSid()
	}

	// Check if response is HTML with login form
	if resp.Request.URL.Path == "/" && strings.Contains(contentType, "text/html") {
		var reader io.ReadCloser
		var err error

		// Check if response is gzip-compressed
		if resp.Header.Get("Content-Encoding") == "gzip" {
			reader, err = gzip.NewReader(resp.Body)
			if err != nil {
				resp.Body.Close()
				return err
			}
			defer reader.Close()
		} else {
			reader = resp.Body
		}

		body, err := io.ReadAll(reader)
		if err != nil {
			return err
		}
		resp.Body.Close()

		// Check if body contains loginform
		if strings.Contains(string(body), `id="loginform"`) || strings.Contains(string(body), `id='loginform'`) {
			fmt.Printf("login page detected, refetching with new SID...\n")
			// update the sid
			err := p.reloadSid()
			if err != nil {
				return fmt.Errorf("reload SID: %w", err)
			}
			// refetch the request with new sid
			newResp, err := p.fetch(resp.Request.Method, resp.Request.URL.Path, nil, func(req *http.Request) {
				req.AddCookie(&http.Cookie{
					Name:  "SID",
					Value: p.sid,
				})
			})
			if err != nil {
				return fmt.Errorf("refetch request: %w", err)
			}
			*resp = *newResp
		} else {
			// Restore the body (uncompressed)
			resp.Body = io.NopCloser(bytes.NewReader(body))
			// Update headers for uncompressed body
			resp.Header.Del("Content-Encoding")
			resp.ContentLength = int64(len(body))
			resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
		}
	}
	return nil
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
	proxy := NewFnosProxy(uds, debug, expectedPassword, password, port)
	fmt.Printf("proxy running on port %d\n", port)

	ch := make(chan string)
	go watchQbPassword(ch)
	go func() {
		for newPassword := range ch {
			if newPassword == password {
				continue
			}
			password = newPassword
			fmt.Printf("new password: %s\n", password)
			if err := proxy.reloadSid(); err != nil {
				fmt.Printf("reload SID: %v\n", err)
			}
		}
	}()

	err = http.ListenAndServe(fmt.Sprintf(":%d", port), proxy)
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
				Usage:   "if not set, qBittorrent will login automatically",
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
