package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

type SearchResult struct {
	Session string `json:"session"`
	Title   string `json:"title"`
	Poster  string `json:"poster"`
}

type searchResponse struct {
	Total int            `json:"total"`
	Data  []SearchResult `json:"data"`
}

type Episode struct {
	Episode float64 `json:"episode"`
	Session string  `json:"session"`
}

type episodePageResponse struct {
	LastPage int       `json:"last_page"`
	Data     []Episode `json:"data"`
}

func (c *Client) Search(query string) ([]SearchResult, error) {
	u := fmt.Sprintf("%s/api?m=search&q=%s", c.domain, url.QueryEscape(query))
	body, err := c.Get(u, nil)
	if err != nil {
		return nil, err
	}

	if strings.HasPrefix(strings.TrimSpace(body), "<") {
		return nil, fmt.Errorf("got HTML instead of JSON — cookies expired or CF blocked")
	}

	var resp searchResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return nil, fmt.Errorf("search parse error: %w", err)
	}
	return resp.Data, nil
}

func (c *Client) GetEpisodes(slug string) ([]Episode, error) {
	var all []Episode
	page := 1

	for {
		u := fmt.Sprintf("%s/api?m=release&id=%s&sort=episode_asc&page=%d", c.domain, slug, page)
		body, err := c.Get(u, nil)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(strings.TrimSpace(body), "<") {
			return nil, fmt.Errorf("got HTML instead of JSON — cookies expired")
		}

		var resp episodePageResponse
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			return nil, fmt.Errorf("episode list parse error: %w", err)
		}

		all = append(all, resp.Data...)
		if page >= resp.LastPage {
			break
		}
		page++
	}
	return all, nil
}

var (
	kwikLinkRe = regexp.MustCompile(`data-src="(https://kwik\.cx/e/[^"]+)"`)
	audioRe    = regexp.MustCompile(`data-audio="([^"]+)"`)
	resRe      = regexp.MustCompile(`data-resolution="([^"]+)"`)
	av1Re      = regexp.MustCompile(`data-av1="([^"]+)"`)
)

type KwikCandidate struct {
	URL        string
	Resolution string
	Audio      string
}

func (c *Client) GetKwikLinks(slug, session string) ([]KwikCandidate, error) {
	pageURL := fmt.Sprintf("%s/play/%s/%s", c.domain, slug, session)
	body, err := c.Get(pageURL, map[string]string{"Referer": c.domain + "/"})
	if err != nil {
		return nil, err
	}

	var candidates []KwikCandidate

	body = strings.ReplaceAll(body, "<button", "\n<button")

	for _, line := range strings.Split(body, "\n") {
		if !strings.Contains(line, "<button") || !strings.Contains(line, "data-src") {
			continue
		}

		if m := av1Re.FindStringSubmatch(line); len(m) > 1 && m[1] == "1" {
			continue
		}

		urlMatch := kwikLinkRe.FindStringSubmatch(line)
		if len(urlMatch) < 2 {
			continue
		}

		res := ""
		if m := resRe.FindStringSubmatch(line); len(m) > 1 {
			res = m[1]
		}
		audio := "jpn"
		if m := audioRe.FindStringSubmatch(line); len(m) > 1 {
			audio = m[1]
		}

		candidates = append(candidates, KwikCandidate{
			URL:        urlMatch[1],
			Resolution: res,
			Audio:      audio,
		})
	}
	return candidates, nil
}

func SelectBestKwik(candidates []KwikCandidate, resolution, audio string) string {
	var audioFiltered []KwikCandidate
	for _, c := range candidates {
		if c.Audio == audio {
			audioFiltered = append(audioFiltered, c)
		}
	}
	if len(audioFiltered) == 0 {
		audioFiltered = candidates
	}

	for _, c := range audioFiltered {
		if c.Resolution == resolution {
			return c.URL
		}
	}

	order := []string{"1080", "720", "480", "360"}
	for _, res := range order {
		for _, c := range audioFiltered {
			if c.Resolution == res {
				return c.URL
			}
		}
	}

	if len(audioFiltered) > 0 {
		return audioFiltered[0].URL
	}
	return ""
}
