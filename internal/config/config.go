package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Config struct {
	UA          string `json:"ua"`
	CF          string `json:"cf"`
	Cookies     string `json:"cookies"`
	DownloadDir string `json:"downloadDir"`
	MaxParallel int    `json:"maxParallel"`
	Quality     string `json:"quality"`
	Audio       string `json:"audio"`
	Domain      string `json:"domain"`
}

var loaded *Config

func GetConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	zensuDir := filepath.Join(dir, "zensu")
	if err := os.MkdirAll(zensuDir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(zensuDir, "config.json"), nil
}

func Load() (*Config, error) {
	if loaded != nil {
		return loaded, nil
	}

	cfgPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		cfg := Config{
			UA:          "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36",
			Domain:      "https://animepahe.pw",
			MaxParallel: 3,
			Quality:     "1080",
			Audio:       "jpn",
			DownloadDir: defaultDownloadDir(),
		}
		if err := cfg.Save(); err != nil {
			return nil, err
		}
		loaded = &cfg
		return loaded, nil
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if cfg.Cookies == "" && cfg.CF != "" {
		cfg.Cookies = "cf_clearance=" + cfg.CF
	}

	if cfg.DownloadDir == "" {
		cfg.DownloadDir = defaultDownloadDir()
	}

	if cfg.MaxParallel <= 0 {
		cfg.MaxParallel = 3
	}

	cfg.Quality = strings.ToLower(strings.TrimSpace(cfg.Quality))
	if strings.HasSuffix(cfg.Quality, "p") {
		cfg.Quality = strings.TrimSuffix(cfg.Quality, "p")
	}
	if cfg.Quality != "1080" && cfg.Quality != "720" && cfg.Quality != "480" && cfg.Quality != "360" {
		cfg.Quality = "1080"
	}

	cfg.Audio = strings.ToLower(strings.TrimSpace(cfg.Audio))
	if cfg.Audio == "sub" || cfg.Audio == "subbed" || cfg.Audio == "japanese" || cfg.Audio == "jpn" || cfg.Audio == "" {
		cfg.Audio = "jpn"
	} else if cfg.Audio == "dub" || cfg.Audio == "dubbed" || cfg.Audio == "english" || cfg.Audio == "eng" {
		cfg.Audio = "eng"
	} else {
		cfg.Audio = "jpn"
	}

	cfg.Domain = strings.TrimSuffix(strings.TrimSpace(cfg.Domain), "/")
	if cfg.Domain == "" {
		cfg.Domain = "https://animepahe.pw"
	} else if !strings.HasPrefix(cfg.Domain, "http://") && !strings.HasPrefix(cfg.Domain, "https://") {
		cfg.Domain = "https://" + cfg.Domain
	}

	loaded = &cfg
	return loaded, nil
}

func (c *Config) Save() error {
	if c.CF != "" && !strings.Contains(c.Cookies, c.CF) {
		c.Cookies = "cf_clearance=" + c.CF
	}
	cfgPath, err := GetConfigPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, data, 0644)
}

func defaultDownloadDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "downloads"
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(home, "Videos", "Anime")
	}
	return filepath.Join(home, "Videos", "Anime")
}
