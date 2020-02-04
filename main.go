package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"encoding/json"

	"budgetbridge/ynab"

	"golang.org/x/oauth2"
)

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

type CategoriesCache struct {
	client   *ynab.Client
	budgetID string
	path     string
	enabled  bool
}

func (c *CategoriesCache) Categories() ([]ynab.Category, error) {
	var categories []ynab.Category
	if err := c.get(&categories); err == nil {
		return categories, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	categories, err := c.fetch()
	if err != nil {
		return nil, err
	}
	if err := c.put(categories); err != nil {
		return nil, err
	}
	return categories, nil
}

func (c *CategoriesCache) put(v interface{}) error {
	f, err := os.Create(c.path)
	if err != nil {
		return err
	}
	defer f.Close()
	encoder := json.NewEncoder(bufio.NewWriter(f))
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

func (c *CategoriesCache) get(v interface{}) error {
	f, err := os.Open(c.path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(bufio.NewReader(f)).Decode(v)
}

func (c *CategoriesCache) fetch() ([]ynab.Category, error) {
	categoriesResponse, err := c.client.Categories(ynab.CategoriesRequest{
		BudgetID: c.budgetID,
	})
	if err != nil {
		return nil, err
	}
	var categories []ynab.Category
	for _, group := range categoriesResponse.CategoryGroups {
		categories = append(categories, group.Categories...)
	}
	return categories, nil
}

func main() {
	var config Config
	err := config.Providers.SetRegistry(map[string]NewProvider{
		"splitwise": &SplitwiseOptions{},
	})
	checkErr(err)

	err = config.load("config.json")
	checkErr(err)

	ctx := context.Background()

	ynabClient := ynab.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: config.AccessToken,
	}))

	if config.Cache.CreateMissingDir {
		err = os.MkdirAll(config.Cache.Dir, os.ModePerm)
		checkErr(err)
	}
	categoriesCache := CategoriesCache{
		client:   ynabClient,
		budgetID: config.BudgetID,
		enabled:  true,
		path:     filepath.Join(config.Cache.Dir, "categories"),
	}

	categories, err := categoriesCache.Categories()
	checkErr(err)

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
