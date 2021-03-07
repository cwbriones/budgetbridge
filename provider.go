package main

import (
	"budgetbridge/ynab"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/rs/zerolog/log"
)

// NewProvider is a means to constructing a transaction provider.
type NewProvider interface {
	NewProvider(context.Context) (TransactionProvider, error)
}

type YnabInfo struct {
	LastUpdateHint time.Time
	Categories     []ynab.Category
}

// A TransactionProvider loads the latest transactions from its source given the current Context.
//
// The provider *may* use the LastUpdateHint within the context in order to constrain the time
// range which it searches.
//
// If the current context contains a non-empty list of categories, the provider *must* omit all
// transactions outside of those categories.
type TransactionProvider interface {
	Transactions(context.Context, YnabInfo) ([]ynab.Transaction, error)
}

type NamedProvider struct {
	// The name of this provider.
	Name string
	// The YNAB account ID to associate with this provider.
	//
	// Any new transactions from this provider will be created under this account.
	AccountID string

	// The inner provider.
	TransactionProvider
}

type ProviderConfig struct {
	// The YNAB account ID to associate with this provider.
	//
	// Any new transactions from this provider will be created under this account.
	AccountID string `json:"account_id"`

	// The generic provider options.
	Options NewProvider
}

type Providers struct {
	Map      map[string]ProviderConfig
	registry map[string]reflect.Type
}

func (p Providers) initAll(ctx context.Context) []NamedProvider {
	var providers []NamedProvider
	for providerName, providerConfig := range p.Map {
		log.Debug().Str("provider", providerName).Msg("initialize provider")
		provider, err := providerConfig.Options.NewProvider(ctx)
		if err != nil {
			log.Err(err).Str("provider", providerName).Msg("initialize failed")
			continue
		}
		withAccountID := NamedProvider{
			providerName,
			providerConfig.AccountID,
			provider,
		}
		providers = append(providers, withAccountID)
	}
	return providers
}

func (pm *Providers) SetRegistry(registry map[string]NewProvider) error {
	for k, v := range registry {
		if err := pm.Register(k, v); err != nil {
			return err
		}
	}
	return nil
}

func (pm *Providers) Register(name string, np NewProvider) error {
	if pm.registry == nil {
		pm.registry = make(map[string]reflect.Type)
	}
	_, ok := pm.registry[name]
	if ok {
		return fmt.Errorf("the name '%s' is already registered", name)
	}
	pm.registry[name] = reflect.TypeOf(np)
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
