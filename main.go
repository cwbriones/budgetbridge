package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"reflect"
	"time"

	"encoding/json"

	"budgetbridge/ynab"

	"golang.org/x/oauth2"
)

type Config struct {
	BudgetID     string `json:"budget_id"`
	AccessToken  string `json:"access_token"`
	LookBackDays int64  `json:"lookback_days"`
	Providers    Providers
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
		return fmt.Errorf("The name '%s' is already registered", name)
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
			return fmt.Errorf("Unknown provider '%s'", k)
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

type NewProvider interface {
	NewProvider(context.Context) (TransactionProvider, error)
}

type YnabInfo struct {
	LastUpdateHint time.Time
	Categories     []ynab.Category
}

type TransactionProvider interface {
	Transactions(context.Context, YnabInfo) ([]ynab.Transaction, error)
}

func main() {
	f, err := os.Open("config.json")
	checkErr(err)
	defer f.Close()

	var config Config
	err = config.Providers.SetRegistry(map[string]NewProvider{
		"splitwise": &SplitwiseConfig{},
	})
	checkErr(err)

	err = json.NewDecoder(bufio.NewReader(f)).Decode(&config)
	checkErr(err)

	ctx := context.Background()

	ynabClient := ynab.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: config.AccessToken,
	}))

	categoriesResponse, err := ynabClient.Categories(ynab.CategoriesRequest{
		BudgetID: config.BudgetID,
	})
	checkErr(err)

	var categories []ynab.Category
	for _, group := range categoriesResponse.CategoryGroups {
		categories = append(categories, group.Categories...)
	}

	var transactions []ynab.Transaction
	for _, providerConfig := range config.Providers.Map {
		provider, err := providerConfig.Options.NewProvider(ctx)
		checkErr(err)

		// Get the most recent YNAB transactions from this account
		res, err := ynabClient.Transactions(ynab.TransactionsRequest{
			BudgetID:  config.BudgetID,
			AccountID: providerConfig.AccountID,
			SinceDate: time.Now().AddDate(0, 0, -int(config.LookBackDays)),
		})
		checkErr(err)

		mostRecentDate := time.Time(res.Transactions[len(res.Transactions)-1].Date)
		fmt.Printf("Fetching up to date %s\n", mostRecentDate.String())

		fetched, err := provider.Transactions(ctx, YnabInfo{
			LastUpdateHint: mostRecentDate,
			Categories:     categories,
		})
		checkErr(err)
		for i := 0; i < len(fetched); i++ {
			fetched[i].AccountId = providerConfig.AccountID
		}
		transactions = append(transactions, fetched...)
	}

	for _, t := range transactions {
		fmt.Printf(
			"Transaction{Date=%s,\tMemo=%s,\tAmount=%d,\tPayeeName=%s,\tImportId=%s}\n",
			t.Date.String(),
			t.Memo,
			t.Amount,
			t.PayeeName,
			*t.ImportId)
	}

	if len(transactions) > 0 {
		request := ynab.CreateTransactionsRequest{
			Transactions: transactions,
		}
		_, err = ynabClient.CreateTransactions(config.BudgetID, request)
		checkErr(err)
	}
}

func checkErr(err error) {
	if err != nil {
		log.Fatalf("[error]: %s\n", err)
	}
}
