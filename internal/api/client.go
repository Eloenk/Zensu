package api

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	http "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

type Client struct {
	inner   tlsclient.HttpClient
	ua      string
	cookies string
	domain  string
}

func NewClient(ua, cookies, domain string) (*Client, error) {
	jar := tlsclient.NewCookieJar()

	options := []tlsclient.HttpClientOption{
		tlsclient.WithTimeoutSeconds(30),
		tlsclient.WithClientProfile(profiles.Chrome_124),
		tlsclient.WithCookieJar(jar),
	}

	inner, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create tls client: %w", err)
	}

	c := &Client{inner: inner, ua: ua, cookies: cookies, domain: domain}
	c.seedCookies(domain)
	c.seedCookies("https://kwik.cx")

	return c, nil
}

func (c *Client) seedCookies(rawURL string) {
	u, _ := url.Parse(rawURL)
	var httpCookies []*http.Cookie
	for _, part := range strings.Split(c.cookies, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		httpCookies = append(httpCookies, &http.Cookie{
			Name:  strings.TrimSpace(kv[0]),
			Value: strings.TrimSpace(kv[1]),
		})
	}
	c.inner.SetCookies(u, httpCookies)
}

func (c *Client) Get(rawURL string, extraHeaders map[string]string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := c.inner.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 {
		return "", fmt.Errorf("403 Forbidden — CF blocked, refresh cookies")
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d for %s", resp.StatusCode, rawURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *Client) Post(rawURL string, referer string, cookies []*http.Cookie, formData url.Values) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, rawURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", referer)
	req.Header.Set("Origin", "https://kwik.cx")

	u, _ := url.Parse(rawURL)
	c.inner.SetCookies(u, cookies)

	resp, err := c.inner.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) GetRawBytes(rawURL string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	req.Header.Set("Sec-Fetch-Dest", "image")
	req.Header.Set("Sec-Fetch-Mode", "no-cors")
	req.Header.Set("Sec-Fetch-Site", "cross-site")

	c.seedCookies(rawURL)

	resp, err := c.inner.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, rawURL)
	}

	return io.ReadAll(resp.Body)
}
