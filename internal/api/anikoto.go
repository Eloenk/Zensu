package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	anikotoSearchItemRe  = regexp.MustCompile(`(?s)<a\s+class="item"\s+href="https?://anikototv\.to/watch/([^"]+)">.*?<img\s+src="([^"]+)".*?<div\s+class="name[^"]*"[^>]*>(.*?)</div>`)
	anikotoWatchDataIdRe = regexp.MustCompile(`id="watch-main"[^>]*data-id="(\d+)"`)
	anikotoEpItemRe      = regexp.MustCompile(`data-id="(\d+)"\s+data-num="([^"]+)"(?:\s+data-slug="[^"]*")?(?:\s+data-mal="[^"]*")?(?:\s+data-timestamp="[^"]*")?(?:\s+data-sub="[^"]*")?(?:\s+data-dub="[^"]*")?\s+data-ids="([^"]+)"`)
	anikotoServerItemRe  = regexp.MustCompile(`<li\s+[^>]*data-ep-id="(\d+)"[^>]*data-sv-id="([^"]+)"[^>]*data-link-id="([^"]+)"[^>]*>(.*?)</li>`)
	megaplayStreamIdRe   = regexp.MustCompile(`/stream/[^/]+/(\d+)`)
	megaplayDataIdRe     = regexp.MustCompile(`data-id="(\d+)"`)
)

type megaplaySourcesResponse struct {
	Sources struct {
		File string `json:"file"`
	} `json:"sources"`
}

type anikotoServerResponse struct {
	Status int `json:"status"`
	Result struct {
		URL string `json:"url"`
	} `json:"result"`
}

type anikotoServerListResponse struct {
	Status int    `json:"status"`
	Result string `json:"result"`
}

type anikotoSearchResponse struct {
	Status int `json:"status"`
	Result struct {
		HTML string `json:"html"`
	} `json:"result"`
}

type anikotoEpListResponse struct {
	Status int    `json:"status"`
	Result string `json:"result"`
}

func (c *Client) SearchAnikoto(query string) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("https://anikototv.to/ajax/anime/search?keyword=%s", url.QueryEscape(query))
	headers := map[string]string{
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          "https://anikototv.to/",
	}

	body, err := c.Get(searchURL, headers)
	if err != nil {
		return nil, fmt.Errorf("anikoto search failed: %w", err)
	}

	var resp anikotoSearchResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return nil, fmt.Errorf("failed to decode anikoto search json: %w", err)
	}

	matches := anikotoSearchItemRe.FindAllStringSubmatch(resp.Result.HTML, -1)
	var results []SearchResult
	for _, m := range matches {
		if len(m) < 4 {
			continue
		}
		slug := strings.TrimSpace(m[1])
		poster := strings.TrimSpace(m[2])
		title := strings.TrimSpace(m[3])
		title = regexp.MustCompile(`<[^>]*>`).ReplaceAllString(title, "")
		title = strings.ReplaceAll(title, "&#039;", "'")
		title = strings.ReplaceAll(title, "&amp;", "&")

		results = append(results, SearchResult{
			Session: slug,
			Title:   title,
			Poster:  poster,
		})
	}
	return results, nil
}

func (c *Client) GetAnikotoEpisodes(slug string) ([]Episode, error) {
	watchURL := fmt.Sprintf("https://anikototv.to/watch/%s", slug)
	watchPage, err := c.Get(watchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to load watch page %s: %w", watchURL, err)
	}

	matchId := anikotoWatchDataIdRe.FindStringSubmatch(watchPage)
	if len(matchId) < 2 {
		return nil, fmt.Errorf("failed to find anime data-id on watch page %s", slug)
	}
	animeID := matchId[1]

	epListURL := fmt.Sprintf("https://anikototv.to/ajax/episode/list/%s", animeID)
	headers := map[string]string{
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          watchURL,
	}

	epBody, err := c.Get(epListURL, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch episode list for anime %s: %w", animeID, err)
	}

	var epResp anikotoEpListResponse
	if err := json.Unmarshal([]byte(epBody), &epResp); err != nil {
		return nil, fmt.Errorf("failed to parse episode list JSON: %w", err)
	}

	epMatches := anikotoEpItemRe.FindAllStringSubmatch(epResp.Result, -1)
	var episodes []Episode
	for _, m := range epMatches {
		if len(m) < 4 {
			continue
		}
		epID := m[1]
		epNumStr := m[2]
		serverToken := m[3]

		num, err := strconv.ParseFloat(epNumStr, 64)
		if err != nil {
			continue
		}

		session := fmt.Sprintf("%s|%s", epID, serverToken)
		episodes = append(episodes, Episode{
			Episode: num,
			Session: session,
		})
	}

	return episodes, nil
}

func (c *Client) GetAnikotoStreamURL(slug, epSession, audioPref string) (string, bool, error) {
	parts := strings.SplitN(epSession, "|", 2)
	if len(parts) < 2 {
		return "", false, fmt.Errorf("invalid anikoto episode session: %s", epSession)
	}
	serverToken := parts[1]

	serverListURL := fmt.Sprintf("https://anikototv.to/ajax/server/list?servers=%s", url.QueryEscape(serverToken))
	watchURL := fmt.Sprintf("https://anikototv.to/watch/%s", slug)
	headers := map[string]string{
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          watchURL,
	}

	body, err := c.Get(serverListURL, headers)
	if err != nil {
		return "", false, fmt.Errorf("failed to fetch server list: %w", err)
	}

	var listResp anikotoServerListResponse
	if err := json.Unmarshal([]byte(body), &listResp); err != nil {
		return "", false, fmt.Errorf("failed to parse server list JSON: %w", err)
	}

	wantDub := (audioPref == "eng" || audioPref == "dub")
	var candidateLinkIds []string
	seen := make(map[string]bool)

	sections := strings.Split(listResp.Result, "data-type=\"")
	for _, section := range sections {
		if (wantDub && strings.HasPrefix(section, "dub\"")) || (!wantDub && strings.HasPrefix(section, "sub\"")) {
			matches := anikotoServerItemRe.FindAllStringSubmatch(section, -1)
			for _, m := range matches {
				if len(m) > 3 && !seen[m[3]] {
					candidateLinkIds = append(candidateLinkIds, m[3])
					seen[m[3]] = true
				}
			}
		}
	}

	// Fallback to any remaining servers if preferred audio servers failed/empty
	allMatches := anikotoServerItemRe.FindAllStringSubmatch(listResp.Result, -1)
	for _, m := range allMatches {
		if len(m) > 3 && !seen[m[3]] {
			candidateLinkIds = append(candidateLinkIds, m[3])
			seen[m[3]] = true
		}
	}

	if len(candidateLinkIds) == 0 {
		return "", false, fmt.Errorf("no server links found in response")
	}

	var lastErr error
	for _, linkId := range candidateLinkIds {
		serverGetURL := fmt.Sprintf("https://anikototv.to/ajax/server?get=%s", url.QueryEscape(linkId))
		bodyGet, err := c.Get(serverGetURL, headers)
		if err != nil {
			lastErr = fmt.Errorf("failed to resolve embed link: %w", err)
			continue
		}

		var getResp anikotoServerResponse
		if err := json.Unmarshal([]byte(bodyGet), &getResp); err != nil {
			lastErr = fmt.Errorf("failed to parse embed response JSON: %w", err)
			continue
		}

		embedURL := getResp.Result.URL
		if embedURL == "" {
			lastErr = fmt.Errorf("empty embed url returned from anikoto")
			continue
		}

		embedBody, err := c.Get(embedURL, map[string]string{
			"Referer": watchURL,
		})
		if err != nil {
			lastErr = fmt.Errorf("failed to fetch megaplay embed page: %w", err)
			continue
		}

		targetID := ""
		dataIdMatch := megaplayDataIdRe.FindStringSubmatch(embedBody)
		if len(dataIdMatch) >= 2 {
			targetID = dataIdMatch[1]
		} else {
			mID := megaplayStreamIdRe.FindStringSubmatch(embedURL)
			if len(mID) >= 2 {
				targetID = mID[1]
			}
		}

		if targetID == "" {
			lastErr = fmt.Errorf("could not extract megaplay ID from embed page or URL")
			continue
		}

		mpHeaders := map[string]string{
			"X-Requested-With": "XMLHttpRequest",
			"Referer":          embedURL,
		}

		sourcesEndpoints := []string{
			fmt.Sprintf("https://megaplay.buzz/stream/getSourcesNew?id=%s&id=%s", targetID, targetID),
			fmt.Sprintf("https://megaplay.buzz/stream/getSources?id=%s", targetID),
		}

		var m3u8URL string
		for _, endpoint := range sourcesEndpoints {
			sourcesBody, err := c.Get(endpoint, mpHeaders)
			if err != nil {
				continue
			}

			var mpResp megaplaySourcesResponse
			if err := json.Unmarshal([]byte(sourcesBody), &mpResp); err == nil && mpResp.Sources.File != "" {
				m3u8URL = mpResp.Sources.File
				break
			}
		}

		if m3u8URL == "" {
			lastErr = fmt.Errorf("empty m3u8 stream URL from megaplay for ID %s", targetID)
			continue
		}

		return m3u8URL, true, nil
	}

	return "", false, fmt.Errorf("all server links failed: %v", lastErr)
}
