package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
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

func getBudgetID(ynabClient *ynab.Client, config Config) (string, error) {
	if config.BudgetID != nil {
		return *config.BudgetID, nil
	}
	res, err := ynabClient.Budgets()
	if err != nil {
		return "", err
	}
	if len(res.Budgets) == 1 {
		return res.Budgets[0].Id, nil
	}
	if res.DefaultBudget != nil {
		return res.DefaultBudget.Id, nil
	}
	return "", fmt.Errorf("no default budget available")
}

func newYNABClient(ctx context.Context, accessToken string) *ynab.Client {
	httpClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: accessToken,
	}))
	return ynab.NewClient(httpClient)
}

func main() {
	configPath := flag.String("config", "config.json", "the path of your config.json file")

	var config Config
	err := config.Providers.SetRegistry(map[string]NewProvider{
		"splitwise": &SplitwiseOptions{},
	})
	checkErr(err)

	err = config.load(*configPath)
	checkErr(err)

	ctx := context.Background()

	ynabClient := newYNABClient(ctx, config.AccessToken)

	if config.Cache.CreateMissingDir {
		err = os.MkdirAll(config.Cache.Dir, os.ModePerm)
		checkErr(err)
	}
	budgetID, err := getBudgetID(ynabClient, config)
	checkErr(err)
	categoriesCache := CategoriesCache{
		client:   ynabClient,
		budgetID: budgetID,
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
			BudgetID:  budgetID,
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

	if len(transactions) > 0 {
		request := ynab.CreateTransactionsRequest{
			Transactions: transactions,
		}
		res, err := ynabClient.CreateTransactions(budgetID, request)
		checkErr(err)
		if len(res.Transactions) > 0 {
			for _, t := range res.Transactions {
				importID := "<unset>"
				if t.ImportId != nil {
					importID = *t.ImportId
				}
				fmt.Printf(
					"Transaction{Date=%s,Memo=%s,Amount=%d,PayeeName=%s,ImportId=%s}\n",
					t.Date.String(),
					t.Memo,
					t.Amount,
					t.PayeeName,
					importID)
			}
			fmt.Printf("Created %d transactions.\n", len(res.Transactions))
		} else {
			fmt.Printf("No new transactions were created.\n")
		}
		if len(res.DuplicateImportIDs) > 0 {
			fmt.Printf("%d duplicate transaction IDs were ignored.\n", len(res.DuplicateImportIDs))
		}
	}
}

func checkErr(err error) {
	if err != nil {
		log.Fatalf("[error]: %s\n", err)
	}
}
