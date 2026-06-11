package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Config struct {
	ServerURL                  string             `json:"server_url"`
	AgentToken                 string             `json:"agent_token"`
	SyncIntervalMinutes        int                `json:"sync_interval_minutes"`
	PollIntervalSeconds        int                `json:"poll_interval_seconds"`
	DeviceUUID                 string             `json:"device_uuid"`
	PrinterPageCounts          map[string]int     `json:"printer_page_counts"`
	LastProcessedEventRecordID int64              `json:"last_processed_event_record_id"`
}

var (
	mu         sync.Mutex
	configPath string
)

func init() {
	exe, err := os.Executable()
	if err != nil {
		configPath = "agent_config.json"
		return
	}
	configPath = filepath.Join(filepath.Dir(exe), "agent_config.json")
}

func Load() *Config {
	mu.Lock()
	defer mu.Unlock()

	cfg := defaultConfig()

	data, err := os.ReadFile(configPath)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, cfg)

	// Ensure defaults if fields are zero
	if cfg.SyncIntervalMinutes == 0 {
		cfg.SyncIntervalMinutes = 30
	}
	if cfg.PollIntervalSeconds == 0 {
		cfg.PollIntervalSeconds = 30
	}
	if cfg.PrinterPageCounts == nil {
		cfg.PrinterPageCounts = map[string]int{}
	}

	return cfg
}

func Save(cfg *Config) error {
	mu.Lock()
	defer mu.Unlock()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

func defaultConfig() *Config {
	return &Config{
		ServerURL:           "http://localhost:8080/api/v1",
		AgentToken:          "yuna_secret_token_2024",
		SyncIntervalMinutes: 30,
		PollIntervalSeconds: 30,
		PrinterPageCounts:   map[string]int{},
	}
}
