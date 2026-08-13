package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type AiringStatus string

const (
	StatusReleasing AiringStatus = "RELEASING"
	StatusFinished  AiringStatus = "FINISHED"
	StatusUnknown   AiringStatus = "UNKNOWN"
)

type MetadataResult struct {
	Title          string       `json:"title"`
	AiringStatus   AiringStatus `json:"airingStatus"`
	TotalEpisodes  int          `json:"totalEpisodes"`
	Source         string       `json:"source"`
	NextEpisodeNum int          `json:"nextEpisodeNum"`
	NextAiringAt   int64        `json:"nextAiringAt"` // Unix timestamp in seconds
	Score          float64      `json:"score"`
	BroadcastDay   string       `json:"broadcastDay"`
}

type AniListResponse struct {
	Data struct {
		Media struct {
			Status            string `json:"status"`
			Episodes          int    `json:"episodes"`
			MeanScore         int    `json:"meanScore"`
			NextAiringEpisode *struct {
				Episode  int   `json:"episode"`
				AiringAt int64 `json:"airingAt"`
			} `json:"nextAiringEpisode"`
		} `json:"Media"`
	} `json:"data"`
}

type JikanResponse struct {
	Data []struct {
		Status    string  `json:"status"`
		Airing    bool    `json:"airing"`
		Episodes  int     `json:"episodes"`
		Score     float64 `json:"score"`
		Broadcast struct {
			Day string `json:"day"`
		} `json:"broadcast"`
	} `json:"data"`
}

func FetchAnimeMetadata(title string) (*MetadataResult, error) {
	result, err := FetchAniListMetadata(title)
	if err == nil && result != nil && result.AiringStatus != StatusUnknown {
		// Enrich with Jikan score/broadcast if AniList score is zero
		if jResult, jErr := FetchJikanMetadata(title); jErr == nil && jResult != nil {
			if result.Score == 0 && jResult.Score > 0 {
				result.Score = jResult.Score
			}
			if result.BroadcastDay == "" && jResult.BroadcastDay != "" {
				result.BroadcastDay = jResult.BroadcastDay
			}
		}
		return result, nil
	}

	jikanResult, jikanErr := FetchJikanMetadata(title)
	if jikanErr == nil && jikanResult != nil && jikanResult.AiringStatus != StatusUnknown {
		return jikanResult, nil
	}

	if result != nil {
		return result, nil
	}
	if jikanResult != nil {
		return jikanResult, nil
	}
	return &MetadataResult{Title: title, AiringStatus: StatusUnknown, Source: "none"}, nil
}

func FetchAniListMetadata(title string) (*MetadataResult, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	query := `
	query ($search: String) {
		Media (search: $search, type: ANIME) {
			status
			episodes
			meanScore
			nextAiringEpisode {
				episode
				airingAt
			}
		}
	}`
	reqBody, err := json.Marshal(map[string]interface{}{
		"query": query,
		"variables": map[string]string{
			"search": title,
		},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://graphql.anilist.co", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var aniResp AniListResponse
	if err := json.NewDecoder(resp.Body).Decode(&aniResp); err != nil {
		return nil, err
	}

	status := StatusUnknown
	s := strings.ToUpper(aniResp.Data.Media.Status)
	if s == "RELEASING" {
		status = StatusReleasing
	} else if s == "FINISHED" || s == "CANCELLED" {
		status = StatusFinished
	}

	res := &MetadataResult{
		Title:         title,
		AiringStatus:  status,
		TotalEpisodes: aniResp.Data.Media.Episodes,
		Score:         float64(aniResp.Data.Media.MeanScore) / 10.0,
		Source:        "AniList",
	}

	if aniResp.Data.Media.NextAiringEpisode != nil {
		res.NextEpisodeNum = aniResp.Data.Media.NextAiringEpisode.Episode
		res.NextAiringAt = aniResp.Data.Media.NextAiringEpisode.AiringAt
	}

	return res, nil
}

func FetchJikanMetadata(title string) (*MetadataResult, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	u := fmt.Sprintf("https://api.jikan.moe/v4/anime?q=%s&limit=1", url.QueryEscape(title))
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ZensuApp/1.3.2")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var jikanResp JikanResponse
	if err := json.NewDecoder(resp.Body).Decode(&jikanResp); err != nil {
		return nil, err
	}

	if len(jikanResp.Data) == 0 {
		return nil, fmt.Errorf("no results found on Jikan")
	}

	item := jikanResp.Data[0]
	status := StatusUnknown
	if item.Airing || strings.EqualFold(item.Status, "Currently Airing") {
		status = StatusReleasing
	} else if strings.EqualFold(item.Status, "Finished Airing") {
		status = StatusFinished
	}

	return &MetadataResult{
		Title:         title,
		AiringStatus:  status,
		TotalEpisodes: item.Episodes,
		Score:         item.Score,
		BroadcastDay:  item.Broadcast.Day,
		Source:        "Jikan",
	}, nil
}
