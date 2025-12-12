package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
)

type FnosProxy struct {
	*httputil.ReverseProxy
	debug            bool
	expectedPassword string
	sid              string
	port             int
	watcher          *QbMonitor
}

func NewFnosProxy(debug bool, expectedPassword string, port int) *FnosProxy {
	watcher := NewQbMonitor()
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if uds, err := watcher.GetUds(); err != nil {
				return nil, fmt.Errorf("get qbittorrent-nox uds: %w", err)
			} else {
				return net.Dial("unix", uds)
			}
		},
	}
	p := &FnosProxy{
		debug:            debug,
		expectedPassword: expectedPassword,
		port:             port,
		watcher:          watcher,
	}

	p.ReverseProxy = &httputil.ReverseProxy{
		Transport:      transport,
		Rewrite:        p.Rewrite,
		ModifyResponse: p.ModifyResponse,
		ErrorHandler:   p.ErrorHandler,
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

func (p *FnosProxy) autoAuth() bool {
	return p.expectedPassword == ""
}

func (p *FnosProxy) updateSid(resp *http.Response) error {
	if !p.autoAuth() {
		return nil
	}
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
	if !p.autoAuth() {
		return nil
	}

	data := url.Values{}

	data.Set("username", "admin")
	if password, err := p.watcher.GetPassword(); err != nil {
		return fmt.Errorf("fetch qbittorrent-nox password: %w", err)
	} else {
		data.Set("password", password)
	}

	resp, err := p.fetch("POST", "/api/v2/auth/login", strings.NewReader(data.Encode()), func(req *http.Request) {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	})
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()

	if err := p.updateSid(resp); err != nil {
		return fmt.Errorf("update SID from cookies: %w", err)
	}
	return nil
}

func (p *FnosProxy) handlAuth(r *httputil.ProxyRequest, body []byte) error {
	if strings.Contains(r.In.URL.Path, "/api/v2/auth/login") {
		outPassword, err := p.watcher.GetPassword()
		if err != nil {
			return fmt.Errorf("fetch qbittorrent-nox password: %w", err)
		}
		if !p.autoAuth() {
			parts := strings.Split(string(body), "&")
			p.debugf("parts: %v\n", parts)
			for _, part := range parts {
				if after, ok := strings.CutPrefix(part, "password="); ok {
					inputPassword := after
					if inputPassword != p.expectedPassword {
						outPassword = ""
						break
					}
				}
			}
		}

		body = fmt.Appendf(nil, "username=admin&password=%s", outPassword)
		r.Out.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
		r.Out.ContentLength = int64(len(body))
	}
	return nil
}

func (p *FnosProxy) ErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	fmt.Printf("http: proxy error: %v\n", err)
	w.WriteHeader(http.StatusBadGateway)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<h1>Proxy Error</h1><p>%v</p>", err)
}

func (p *FnosProxy) Rewrite(r *httputil.ProxyRequest) {
	p.debugf("request: %v\n", r.In.URL.Path)
	r.Out.URL.Scheme = "http"
	r.Out.URL.Host = "unix"
	r.Out.Host = "unix"
	if p.sid != "" {
		r.Out.AddCookie(&http.Cookie{
			Name:  "SID",
			Value: p.sid,
		})
	}

	body, _ := io.ReadAll(r.In.Body)
	r.In.ParseForm()
	err := p.handlAuth(r, body)
	if err != nil {
		fmt.Printf("handle auth login: %v\n", err)
	}
	r.Out.Header.Del("Referer")
	r.Out.Header.Del("Origin")
	r.Out.Body = io.NopCloser(bytes.NewBuffer(body))
}

func (p *FnosProxy) ModifyResponse(resp *http.Response) error {
	if !p.autoAuth() {
		return nil
	}
	// update SID from response cookies
	p.updateSid(resp)

	contentType := resp.Header.Get("Content-Type")
	isHtml := strings.Contains(contentType, "text/html")

	// for any non-HTML response with 403 status, reload SID
	if resp.StatusCode == http.StatusForbidden && !isHtml {
		return p.reloadSid()
	}

	// Check if response is HTML with login form
	if resp.Request.URL.Path == "/" && isHtml {
		return p.handleHtmlIndex(resp)
	}
	return nil
}

func (p *FnosProxy) handleHtmlIndex(resp *http.Response) error {
	body, err := readBody(resp)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}
	if !hasLoginForm(body) {
		restoreBody(resp, body)
		return nil
	}

	p.debugf("login page detected, refetching with new SID...\n")
	// update the sid
	if err := p.reloadSid(); err != nil {
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
	return nil
}

func readBody(resp *http.Response) ([]byte, error) {
	var reader io.ReadCloser
	var err error

	// Ensure original body is closed eventually
	defer resp.Body.Close()

	if resp.Header.Get("Content-Encoding") == "gzip" {
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
	} else {
		reader = resp.Body
	}

	return io.ReadAll(reader)
}

func restoreBody(resp *http.Response, body []byte) {
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.Header.Del("Content-Encoding")
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
}

func hasLoginForm(body []byte) bool {
	match, _ := regexp.MatchString(`<form[^>]+id=["']loginform["']`, string(body))
	return match
}
