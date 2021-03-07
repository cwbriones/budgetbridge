package main

import (
	"context"
	"fmt"
	"time"

	"budgetbridge/ynab"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type ynabClient interface {
	Budgets(context.Context) (ynab.BudgetsResponse, error)
	Transactions(context.Context, ynab.TransactionsRequest) (ynab.TransactionsResponse, error)
	CreateTransactions(context.Context, string, ynab.CreateTransactionsRequest) (ynab.TransactionsResponse, error)
}

type BudgetBridge struct {
	BudgetID     string
	LookBackDays int64
	ynabClient   ynabClient
	providers    []NamedProvider
	categories   []ynab.Category
	dryRun       bool
}

func (bb BudgetBridge) ImportAll(ctx context.Context, config Config) error {
	var transactions []ynab.Transaction
	for _, provider := range bb.providers {
		log.Debug().Str("provider", provider.Name).Msg("load transactions")

		// Get the most recent YNAB transactions from this account
		res, err := bb.ynabClient.Transactions(ctx, ynab.TransactionsRequest{
			BudgetID:  bb.BudgetID,
			AccountID: provider.AccountID,
			SinceDate: time.Now().AddDate(0, 0, -int(config.LookBackDays)),
		})
		if err != nil {
			log.Err(err).Str("provider", provider.Name).Msg("fetch most recent txs failed")
			continue
		}

		mostRecentDate := time.Time(res.Transactions[len(res.Transactions)-1].Date)
		log.Info().Str("provider", provider.Name).Time("since", mostRecentDate).Msg("fetching transactions")

		fetched, err := provider.Transactions(ctx, YnabInfo{
			LastUpdateHint: mostRecentDate,
			Categories:     bb.categories,
		})
		if err != nil {
			log.Err(err).Str("provider", provider.Name).Msg("transactions failed")
			continue
		}
		for i := 0; i < len(fetched); i++ {
			fetched[i].AccountId = provider.AccountID
		}
		transactions = append(transactions, fetched...)
	}

	// FIXME: ideally this map could be precomputed.
	categoriesByID := make(map[string]ynab.Category)
	for _, c := range bb.categories {
		categoriesByID[c.Id] = c
	}

	if bb.dryRun {
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
				if c, ok := categoriesByID[categoryID]; ok {
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
		return nil
	}

	if len(transactions) > 0 {
		request := ynab.CreateTransactionsRequest{
			Transactions: transactions,
		}
		res, err := bb.ynabClient.CreateTransactions(ctx, bb.BudgetID, request)
		if err != nil {
			return fmt.Errorf("could not create transactions: %s", err)
		}
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
	return nil
}
