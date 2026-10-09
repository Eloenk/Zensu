package tracker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"zensu/internal/api"
	"zensu/internal/logger"
)

type TrackedAnime struct {
	Title            string  `json:"title"`
	Slug             string  `json:"slug"`
	Poster           string  `json:"poster"`
	Provider         string  `json:"provider"`
	LastDownloadedEp float64 `json:"lastDownloadedEp"`
	TotalEpisodes    int     `json:"totalEpisodes"`
	AiringStatus     string  `json:"airingStatus"`
	AutoDownload     bool    `json:"autoDownload"`
	NextEpisodeNum   int     `json:"nextEpisodeNum"`
	NextAiringAt     int64   `json:"nextAiringAt"`
	Score            float64 `json:"score"`
	BroadcastDay     string  `json:"broadcastDay"`
	CreatedAt        string  `json:"createdAt"`
	LastCheckedAt    string  `json:"lastCheckedAt"`
}

type Manager struct {
	mu       sync.RWMutex
	items    map[string]*TrackedAnime
	filePath string
}

func NewManager() *Manager {
	dir, err := os.UserConfigDir()
	var path string
	if err == nil {
		path = filepath.Join(dir, "zensu", "tracked.json")
	} else {
		path = "tracked.json"
	}

	m := &Manager{
		items:    make(map[string]*TrackedAnime),
		filePath: path,
	}
	m.load()
	return m
}

func (m *Manager) load() {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return
	}

	var list []*TrackedAnime
	if err := json.Unmarshal(data, &list); err == nil {
		for _, item := range list {
			if item.Provider == "" {
				item.Provider = "animepahe"
			}
			m.items[item.Title] = item
		}
		logger.Infof("TRACKER_LOADED", "Loaded %d tracked anime from disk", len(m.items))
	}
}

func (m *Manager) saveLocked() error {
	dir := filepath.Dir(m.filePath)
	_ = os.MkdirAll(dir, 0700)

	list := make([]*TrackedAnime, 0, len(m.items))
	for _, v := range m.items {
		list = append(list, v)
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.filePath, data, 0600)
}

func (m *Manager) GetList() []TrackedAnime {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make([]TrackedAnime, 0, len(m.items))
	for _, v := range m.items {
		res = append(res, *v)
	}
	return res
}

func (m *Manager) IsTracked(title string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.items[title]
	return ok
}

func (m *Manager) ToggleTrack(title, slug, poster, provider string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.items[title]; exists {
		delete(m.items, title)
		logger.Infof("TRACKER_UNTRACK", "Untracked anime: %q", title)
		if err := m.saveLocked(); err != nil {
			return false, err
		}
		return false, nil
	}

	if provider == "" {
		provider = "animepahe"
	}

	status := string(api.StatusReleasing)
	totalEps := 0
	nextEp := 0
	var nextAiringAt int64
	score := 0.0
	broadcast := ""

	meta, err := api.FetchAnimeMetadata(title)
	if err == nil && meta != nil {
		status = string(meta.AiringStatus)
		totalEps = meta.TotalEpisodes
		nextEp = meta.NextEpisodeNum
		nextAiringAt = meta.NextAiringAt
		score = meta.Score
		broadcast = meta.BroadcastDay
	}

	item := &TrackedAnime{
		Title:            title,
		Slug:             slug,
		Poster:           poster,
		Provider:         provider,
		LastDownloadedEp: 0,
		TotalEpisodes:    totalEps,
		AiringStatus:     status,
		AutoDownload:     true,
		NextEpisodeNum:   nextEp,
		NextAiringAt:     nextAiringAt,
		Score:            score,
		BroadcastDay:     broadcast,
		CreatedAt:        time.Now().Format(time.RFC3339),
		LastCheckedAt:    time.Now().Format(time.RFC3339),
	}
	m.items[title] = item
	logger.Infof("TRACKER_TRACK", "Tracked new anime: %q (provider: %s status: %s score: %.2f)", title, provider, status, score)

	if err := m.saveLocked(); err != nil {
		return false, err
	}
	return true, nil
}

func (m *Manager) BatchTrack(shows []TrackedAnime) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	addedCount := 0
	for _, show := range shows {
		if _, exists := m.items[show.Title]; !exists {
			prov := show.Provider
			if prov == "" {
				prov = "animepahe"
			}
			item := &TrackedAnime{
				Title:            show.Title,
				Slug:             show.Slug,
				Poster:           show.Poster,
				Provider:         prov,
				LastDownloadedEp: 0,
				TotalEpisodes:    show.TotalEpisodes,
				AiringStatus:     show.AiringStatus,
				AutoDownload:     true,
				CreatedAt:        time.Now().Format(time.RFC3339),
				LastCheckedAt:    time.Now().Format(time.RFC3339),
			}

			// Fetch AniList/Jikan metadata if available
			if meta, err := api.FetchAnimeMetadata(show.Title); err == nil && meta != nil {
				item.AiringStatus = string(meta.AiringStatus)
				item.TotalEpisodes = meta.TotalEpisodes
				item.NextEpisodeNum = meta.NextEpisodeNum
				item.NextAiringAt = meta.NextAiringAt
				item.Score = meta.Score
				item.BroadcastDay = meta.BroadcastDay
			}

			m.items[show.Title] = item
			addedCount++
		}
	}

	if addedCount > 0 {
		if err := m.saveLocked(); err != nil {
			return 0, err
		}
	}
	return addedCount, nil
}

func (m *Manager) UpdateLastDownloaded(title string, epNum float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, ok := m.items[title]
	if !ok {
		return
	}

	if epNum > item.LastDownloadedEp {
		item.LastDownloadedEp = epNum
		item.LastCheckedAt = time.Now().Format(time.RFC3339)
		_ = m.saveLocked()
	}
}

func (m *Manager) UpdateFullMetadata(title string, meta *api.MetadataResult) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if item, ok := m.items[title]; ok && meta != nil {
		item.AiringStatus = string(meta.AiringStatus)
		if meta.TotalEpisodes > 0 {
			item.TotalEpisodes = meta.TotalEpisodes
		}
		if meta.NextEpisodeNum > 0 {
			item.NextEpisodeNum = meta.NextEpisodeNum
		}
		if meta.NextAiringAt > 0 {
			item.NextAiringAt = meta.NextAiringAt
		}
		if meta.Score > 0 {
			item.Score = meta.Score
		}
		if meta.BroadcastDay != "" {
			item.BroadcastDay = meta.BroadcastDay
		}
		item.LastCheckedAt = time.Now().Format(time.RFC3339)
		_ = m.saveLocked()
	}
}
