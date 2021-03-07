package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"

	"budgetbridge/ynab"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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

func getBudgetID(ctx context.Context, ynabClient *ynab.Client, config Config) (string, error) {
	if config.BudgetID != nil {
		return *config.BudgetID, nil
	}
	res, err := ynabClient.Budgets(ctx)
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

func initLogging() func() error {
	w := bufio.NewWriter(os.Stderr)

	level := zerolog.InfoLevel
	if debug := os.Getenv("DEBUG"); debug != "" {
		level = zerolog.DebugLevel
	}
	zerolog.SetGlobalLevel(level)
	log.Logger = zerolog.
		New(w).
		With().
		Timestamp().
		Logger()

	return w.Flush
}

type dateFlag struct {
	time   time.Time
	layout string
}

func (f *dateFlag) Set(value string) error {
	var t time.Time
	var err error
	if t, err = time.Parse(f.layout, value); err != nil {
		return err
	}
	f.time = t
	return nil
}

func (f *dateFlag) String() string {
	return f.time.Format(f.layout)
}

func main() {
	configPath := flag.String("config", "config.json", "the path of your config.json file")
	dryRun := flag.Bool("dry", false, "emit the transactions but do not create them.")

	lastUpdateHint := dateFlag{
		layout: "2006-01-02",
	}
	flag.Var(&lastUpdateHint, "since", "how far to look back for transactions.")

	flag.Parse()
	flush := initLogging()
	defer flush()

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
	budgetID, err := getBudgetID(ctx, ynabClient, config)
	checkErr(err)
	categoriesCache := CategoriesCache{
		client:   ynabClient,
		budgetID: budgetID,
		enabled:  true,
		path:     filepath.Join(config.Cache.Dir, "categories"),
	}

	categories, err := categoriesCache.Categories(ctx)
	checkErr(err)

	categoriesById := make(map[string]ynab.Category)
	for _, c := range categories {
		categoriesById[c.Id] = c
	}

	var transactions []ynab.Transaction
	for _, providerConfig := range config.Providers.Map {
		provider, err := providerConfig.Options.NewProvider(ctx)
		checkErr(err)

		// Get the most recent YNAB transactions from this account
		res, err := ynabClient.Transactions(ctx, ynab.TransactionsRequest{
			BudgetID:  budgetID,
			AccountID: providerConfig.AccountID,
			SinceDate: time.Now().AddDate(0, 0, -int(config.LookBackDays)),
		})
		checkErr(err)

		if lastUpdateHint.time.IsZero() {
			lastUpdateHint.time = time.Time(res.Transactions[len(res.Transactions)-1].Date)
		}
		log.Info().Time("since", lastUpdateHint.time).Msg("fetching transactions")

		fetched, err := provider.Transactions(ctx, YnabInfo{
			LastUpdateHint: lastUpdateHint.time,
			Categories:     categories,
		})
		checkErr(err)
		for i := 0; i < len(fetched); i++ {
			fetched[i].AccountId = providerConfig.AccountID
		}
		transactions = append(transactions, fetched...)
	}

	if *dryRun {
		log.Info().Msg("DRY RUN: No transactions will be created.")
		for _, t := range transactions {
			var importID string
			var categoryID string
			var categoryName string
			if t.ImportId != nil {
				importID = *t.ImportId
			}
			if t.CategoryId != nil {
				categoryID = *t.CategoryId
				if c, ok := categoriesById[categoryID]; ok {
					categoryName = c.Name
				}
			}
			log.Info().
				Dict("transaction", zerolog.Dict().
					Time("date", t.Date.Time()).
					Str("memo", t.Memo).
					Int("amount", t.Amount).
					Str("payeeName", t.PayeeName).
					Str("importID", importID).
					Str("category.id", categoryID).
					Str("category.name", categoryName),
				).
				Msg("DRY RUN: would create")
		}
		return
	}

	if len(transactions) > 0 {
		request := ynab.CreateTransactionsRequest{
			Transactions: transactions,
		}
		res, err := ynabClient.CreateTransactions(ctx, budgetID, request)
		checkErr(err)
		if len(res.Transactions) > 0 {
			for _, t := range res.Transactions {
				importID := "<unset>"
				if t.ImportId != nil {
					importID = *t.ImportId
				}
				log.Info().
					Dict("transaction", zerolog.Dict().
						Time("date", t.Date.Time()).
						Str("memo", t.Memo).
						Int("amount", t.Amount).
						Str("payeeName", t.PayeeName).
						Str("importID", importID),
					).
					Msg("created transaction")
			}
			log.Info().Int("count", len(res.Transactions)).Msg("transactions successfully created")
		} else {
			log.Info().Msg("no new transactions were created")
		}
		if len(res.DuplicateImportIDs) > 0 {
			log.Info().Int("count", len(res.DuplicateImportIDs)).Msg("duplicate transaction IDs were ignored.")
		}
	}
}

func checkErr(err error) {
	if err != nil {
		log.Fatal().Err(err)
	}
}
