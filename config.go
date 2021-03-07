package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
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

type Providers struct {
	Map      map[string]ProviderConfig
	registry map[string]reflect.Type
}

func (pm *Providers) SetRegistry(registry map[string]NewProvider) error {
	for k, v := range registry {
		if err := pm.Register(k, v); err != nil {
			return err
		}
	}
	return nil
}

func (pm *Providers) Register(name string, v NewProvider) error {
	if pm.registry == nil {
		pm.registry = make(map[string]reflect.Type)
	}
	_, ok := pm.registry[name]
	if ok {
		return fmt.Errorf("the name '%s' is already registered", name)
	}
	pm.registry[name] = reflect.TypeOf(v)
	return nil
}

func (pm *Providers) UnmarshalJSON(bytes []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(bytes, &raw); err != nil {
		return err
	}
	pm.Map = make(map[string]ProviderConfig, len(raw))
	for k, v := range raw {
		var providerConfig ProviderConfig

		rt, ok := pm.registry[k]
		if !ok {
			return fmt.Errorf("unknown provider '%s'", k)
		}

		p := reflect.New(rt).Elem()
		if p.Kind() == reflect.Ptr {
			// initialize the pointer to a valid struct
			rs := reflect.New(p.Type().Elem())
			p.Set(rs)
		}
		// safety: This type assertion should always succeed since the type is provided
		// via the `Register` or `SetRegistry` methods.
		providerConfig.Options = p.Interface().(NewProvider)

		if err := json.Unmarshal(v, &providerConfig); err != nil {
			return err
		}
		pm.Map[k] = providerConfig
	}
	return nil
}

type ProviderConfig struct {
	AccountID      string `json:"account_id"`
	LastUpdateHint bool   `json:"last_update_hint"`
	Options        NewProvider
}
