package kwik

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	http "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

const (
	kwikHost   = "https://kwik.cx"
	refererURL = "https://animepahe.pw/"
)

var (
	packerArgsRe = regexp.MustCompile(`'((?:[^'\\]|\\.)*)'\s*,\s*(\d+|\[\])\s*,\s*(\d+)\s*,\s*'((?:[^'\\]|\\.)*)'\s*\.\s*split\s*\(\s*['"]\|['"]\s*\)`)

	sourceRe = regexp.MustCompile(`source\s*=\s*['"]([^'"]+\.m3u8)['"]`)

	formActionRe = regexp.MustCompile(`action="(https://kwik\.cx/d/[^"]+)"`)
	tokenRe      = regexp.MustCompile(`name="_token"\s+value="([^"]+)"`)

	cookieRe = regexp.MustCompile(`document\.cookie\s*=\s*"([^"]+)"`)
)

type Extractor struct {
	ua      string
	cookies string
}

func NewExtractor(ua, cookies string) *Extractor {
	return &Extractor{ua: ua, cookies: cookies}
}

func (e *Extractor) GetDownloadURL(kwikEmbedURL string) (string, bool, error) {

	body, respCookies, err := e.fetchPage(kwikEmbedURL, refererURL)
	if err != nil {
		return "", false, fmt.Errorf("failed to fetch kwik page: %w", err)
	}

	dlURL, err := e.tryPostForm(body, kwikEmbedURL, respCookies)
	if err == nil && dlURL != "" {
		return dlURL, false, nil
	}

	m3u8, err := e.tryEvalM3u8(body)
	if err == nil && m3u8 != "" {
		return m3u8, true, nil
	}

	return "", false, fmt.Errorf("could not extract download URL from kwik page")
}

func (e *Extractor) tryPostForm(body, pageURL string, pageCookies []*http.Cookie) (string, error) {

	actionMatch := formActionRe.FindStringSubmatch(body)
	if len(actionMatch) < 2 {
		return "", fmt.Errorf("no form action found")
	}
	action := actionMatch[1]

	tokenMatch := tokenRe.FindStringSubmatch(body)
	if len(tokenMatch) < 2 {
		return "", fmt.Errorf("no _token found")
	}
	token := tokenMatch[1]

	var extraCookies []*http.Cookie
	for _, m := range cookieRe.FindAllStringSubmatch(body, -1) {
		if len(m) > 1 {
			parts := strings.SplitN(m[1], "=", 2)
			if len(parts) == 2 {
				name := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(strings.Split(parts[1], ";")[0])
				extraCookies = append(extraCookies, &http.Cookie{Name: name, Value: val})
			}
		}
	}

	allCookies := append(pageCookies, extraCookies...)

	formData := url.Values{}
	formData.Set("_token", token)

	client, err := e.newClient()
	if err != nil {
		return "", err
	}

	kwikURL, _ := url.Parse(kwikHost)
	client.SetCookies(kwikURL, allCookies)

	req, err := http.NewRequest(http.MethodPost, action, strings.NewReader(formData.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", e.ua)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", pageURL)
	req.Header.Set("Origin", kwikHost)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("POST failed: %w", err)
	}
	defer resp.Body.Close()

	loc := resp.Header.Get("Location")
	if loc == "" {
		loc = resp.Header.Get("location")
	}
	if loc != "" {
		return loc, nil
	}

	return "", fmt.Errorf("no redirect location in POST response (status %d)", resp.StatusCode)
}

func (e *Extractor) tryEvalM3u8(body string) (string, error) {

	matchesAll := packerArgsRe.FindAllStringSubmatch(body, -1)
	if len(matchesAll) == 0 {
		return "", fmt.Errorf("no packed blocks found in kwik page")
	}

	for _, m := range matchesAll {
		payload := m[1]
		radixStr := m[2]
		countStr := m[3]
		wordsStr := m[4]

		unpacked, err := unpackParams(payload, radixStr, countStr, wordsStr)
		if err != nil {
			continue
		}

		sourceMatch := sourceRe.FindStringSubmatch(unpacked)
		if len(sourceMatch) > 1 {
			return sourceMatch[1], nil
		}
	}

	return "", fmt.Errorf("could not find m3u8 in unpacked javascript")
}

func unpackParams(payload, radixStr, countStr, wordsStr string) (string, error) {
	radix := 62
	if radixStr != "[]" {
		var err error
		radix, err = strconv.Atoi(radixStr)
		if err != nil {
			return "", err
		}
	}

	_, err := strconv.Atoi(countStr)
	if err != nil {
		return "", err
	}

	symtab := strings.Split(wordsStr, "|")

	payload = strings.ReplaceAll(payload, "\\\\", "\\")
	payload = strings.ReplaceAll(payload, "\\'", "'")

	alphabet := "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	if radix > 62 {
		alphabet = " !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~"
	}
	if radix > len(alphabet) {
		return "", fmt.Errorf("radix too large: %d", radix)
	}

	dict := make(map[rune]int)
	for i, r := range alphabet[:radix] {
		dict[r] = i
	}

	unbase := func(s string) int {
		val := 0
		for _, r := range s {
			d, ok := dict[r]
			if !ok {
				return -1
			}
			val = val*radix + d
		}
		return val
	}

	wordRe := regexp.MustCompile(`[a-zA-Z0-9_]+`)
	unpacked := wordRe.ReplaceAllStringFunc(payload, func(word string) string {
		idx := unbase(word)
		if idx >= 0 && idx < len(symtab) {
			if symtab[idx] != "" {
				return symtab[idx]
			}
		}
		return word
	})

	return unpacked, nil
}

func (e *Extractor) fetchPage(pageURL, referer string) (string, []*http.Cookie, error) {
	client, err := e.newClient()
	if err != nil {
		return "", nil, err
	}

	kwikURL, _ := url.Parse(kwikHost)
	var existingCookies []*http.Cookie
	for _, part := range strings.Split(e.cookies, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			existingCookies = append(existingCookies, &http.Cookie{
				Name:  strings.TrimSpace(kv[0]),
				Value: strings.TrimSpace(kv[1]),
			})
		}
	}
	client.SetCookies(kwikURL, existingCookies)

	req, err := http.NewRequest(http.MethodGet, pageURL, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", e.ua)
	req.Header.Set("Referer", referer)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 {
		return "", nil, fmt.Errorf("403 from kwik — CF blocked")
	}

	var buf strings.Builder
	buf.Grow(1 << 16)
	tmp := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if err != nil {
			break
		}
	}

	parsedURL, _ := url.Parse(pageURL)
	respCookies := client.GetCookies(parsedURL)

	return buf.String(), respCookies, nil
}

func (e *Extractor) newClient() (tlsclient.HttpClient, error) {
	jar := tlsclient.NewCookieJar()
	options := []tlsclient.HttpClientOption{
		tlsclient.WithTimeoutSeconds(30),
		tlsclient.WithClientProfile(profiles.Chrome_124),
		tlsclient.WithCookieJar(jar),
		tlsclient.WithNotFollowRedirects(),
	}
	return tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), options...)
}
