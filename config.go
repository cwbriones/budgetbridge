package main

import (
	"bufio"
	"encoding/json"
	"os"
)

type Config struct {
	BudgetID     *string     `json:"budget_id"`
	AccessToken  string      `json:"access_token"`
	LookBackDays int64       `json:"lookback_days"`
	Cache        CacheConfig `json:"cache"`
	Providers    Providers   `json:"providers"`
}

func (config *Config) load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(bufio.NewReader(f)).Decode(config)
}

type CacheConfig struct {
	Dir              string `json:"dir"`
	CreateMissingDir bool   `json:"create_missing_dir"`
	Categories       bool   `json:"categories"`
}
