package api

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	http "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"

	"zensu/internal/logger"
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
		logger.Errorf("API_CLIENT_INIT_ERR", "Failed to create tls client: %v", err)
		return nil, fmt.Errorf("failed to create tls client: %w", err)
	}

	c := &Client{inner: inner, ua: ua, cookies: cookies, domain: domain}
	c.seedCookies(domain)
	c.seedCookies("https://kwik.cx")

	logger.Infof("API_CLIENT_INIT", "Client created successfully for domain %s", domain)
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
	logger.Infof("API_GET", "GET request: %s", rawURL)
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		logger.Errorf("API_GET_ERR", "Failed to create request for %s: %v", rawURL, err)
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
		logger.Errorf("API_GET_ERR", "GET failed for %s: %v", rawURL, err)
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 {
		logger.Errorf("API_CF_BLOCKED", "403 Forbidden on %s — CF blocked, cookies need update", rawURL)
		return "", fmt.Errorf("403 Forbidden — CF blocked, refresh cookies")
	}
	if resp.StatusCode != 200 {
		logger.Errorf("API_GET_BAD_STATUS", "HTTP %d returned for %s", resp.StatusCode, rawURL)
		return "", fmt.Errorf("HTTP %d for %s", resp.StatusCode, rawURL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		logger.Errorf("API_GET_READ_ERR", "Failed to read body from %s: %v", rawURL, err)
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
	logger.Infof("API_GET_BYTES", "GET raw bytes request: %s", rawURL)
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		logger.Errorf("API_BYTES_ERR", "Failed creating raw bytes request for %s: %v", rawURL, err)
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
		logger.Errorf("API_BYTES_ERR", "GET raw bytes failed for %s: %v", rawURL, err)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		logger.Errorf("API_BYTES_BAD_STATUS", "HTTP %d returned for raw bytes from %s", resp.StatusCode, rawURL)
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, rawURL)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		logger.Errorf("API_BYTES_READ_ERR", "Failed to read raw bytes from %s: %v", rawURL, err)
		return nil, err
	}
	return data, nil
}

func (c *Client) TestConnection() error {
	_, err := c.Get(c.domain, nil)
	return err
}

