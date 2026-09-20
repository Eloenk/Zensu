package api

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"zensu/internal/logger"
)

var (
	ahSearchCardRe  = regexp.MustCompile(`<a class='ac' href='/anime\.php\?([^']+)'>[\s\S]*?<img class='coverimg' src='([^']+)' alt='([^']*)'`)
	ahEpGateKeyRe   = regexp.MustCompile(`<a class='c' onmouseover='gateh\( "([a-f0-9]+)"\)' onclick='gatea\( "([a-f0-9]+)"\)'[^>]*id ="([a-f0-9]+)"[^>]*href= 'gate.php'[\s\S]*?<div\s+class= '\s*watch2\s+bc\s*'\s*>(\d+(\.\d+)?)</div>`)
	ahVideoSourceRe = regexp.MustCompile(`<source src='(https://[^']+(?:/video\.mp4|\.m3u8|\.mp4)\?[^']+)'`)
	ahDirectDownRe  = regexp.MustCompile(`<a href='(https://[^']+(?:/video\.mp4|\.m3u8|\.mp4)\?[^']+&d)'>`)
)

func (c *Client) SearchAnimeHeaven(query string) ([]SearchResult, error) {
	u := fmt.Sprintf("https://animeheaven.me/fastsearch.php?xhr=1&s=%s", url.QueryEscape(query))
	body, err := c.Get(u, nil)
	if err != nil {
		logger.Errorf("AH_SEARCH_ERR", "AnimeHeaven fastsearch failed for query %q: %v", query, err)
		return nil, err
	}

	matches := ahSearchCardRe.FindAllStringSubmatch(body, -1)
	var results []SearchResult
	for _, m := range matches {
		if len(m) < 4 {
			continue
		}
		slug := strings.TrimSpace(m[1])
		posterRel := strings.TrimSpace(m[2])
		title := strings.TrimSpace(m[3])

		poster := posterRel
		if !strings.HasPrefix(poster, "http") {
			poster = "https://animeheaven.me/" + strings.TrimPrefix(poster, "/")
		}

		results = append(results, SearchResult{
			Session: slug,
			Title:   title,
			Poster:  poster,
		})
	}
	return results, nil
}

func (c *Client) GetAnimeHeavenEpisodes(slug string) ([]Episode, error) {
	u := fmt.Sprintf("https://animeheaven.me/anime.php?%s", url.QueryEscape(slug))
	body, err := c.Get(u, nil)
	if err != nil {
		logger.Errorf("AH_EPISODES_ERR", "Failed fetching AnimeHeaven anime page %s: %v", slug, err)
		return nil, err
	}

	matches := ahEpGateKeyRe.FindAllStringSubmatch(body, -1)
	var episodes []Episode
	seen := make(map[float64]bool)

	for _, m := range matches {
		if len(m) < 5 {
			continue
		}
		keyHash := strings.TrimSpace(m[1])
		epVal, err := strconv.ParseFloat(strings.TrimSpace(m[4]), 64)
		if err != nil {
			continue
		}
		if seen[epVal] {
			continue
		}
		seen[epVal] = true

		episodes = append(episodes, Episode{
			Episode: epVal,
			Session: keyHash,
		})
	}

	for i := 0; i < len(episodes)/2; i++ {
		j := len(episodes) - 1 - i
		episodes[i], episodes[j] = episodes[j], episodes[i]
	}

	return episodes, nil
}

func (c *Client) GetAnimeHeavenStream(keyHash, slug string) ([]string, string, error) {
	gateURL := "https://animeheaven.me/gate.php"
	extraHeaders := map[string]string{
		"Cookie":  fmt.Sprintf("key=%s; path=/", keyHash),
		"Referer": fmt.Sprintf("https://animeheaven.me/anime.php?%s", slug),
	}

	body, err := c.Get(gateURL, extraHeaders)
	if err != nil {
		logger.Errorf("AH_GATE_ERR", "Failed fetching gate.php for key %s: %v", keyHash, err)
		return nil, "", err
	}

	var directDown string
	if m := ahDirectDownRe.FindStringSubmatch(body); len(m) > 1 {
		directDown = m[1]
	}

	sourceMatches := ahVideoSourceRe.FindAllStringSubmatch(body, -1)
	var mirrors []string
	seen := make(map[string]bool)

	if directDown != "" {
		mirrors = append(mirrors, directDown)
		seen[directDown] = true
	}

	for _, m := range sourceMatches {
		if len(m) > 1 {
			src := m[1]
			if !seen[src] {
				seen[src] = true
				mirrors = append(mirrors, src)
			}
		}
	}

	if len(mirrors) == 0 {
		return nil, "", fmt.Errorf("no video sources found in gate.php for key %s", keyHash)
	}

	return mirrors, directDown, nil
}
