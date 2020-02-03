package main

import (
	"budgetbridge/splitwise"
	"budgetbridge/ynab"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type SplitwiseTransactionProvider struct {
	userID int
	client *splitwise.Client
}

type SplitwiseConfig struct {
	UserID       int    `json:"user_id"`
	ClientKey    string `json:"client_key"`
	ClientSecret string `json:"client_secret"`
	TokenCache   string `json:"token_cache"`
}

func (config *SplitwiseConfig) NewProvider(ctx context.Context) (TransactionProvider, error) {
	authConfig := NewSplitwiseConfig(
		config.ClientKey,
		config.ClientSecret,
	)
	client := splitwise.NewClient(ctx, &CachingTokenSource{
		TokenSource: &LocalServerTokenSource{
			Config: authConfig,
		},
		Path: config.TokenCache,
	})
	return &SplitwiseTransactionProvider{
		userID: config.UserID,
		client: client,
	}, nil
}

func (sts *SplitwiseTransactionProvider) Transactions(ctx context.Context, ynabInfo YnabInfo) ([]ynab.Transaction, error) {
	// Get all splitwise transactions since this date

	// Go up to one week before hint
	datedAfter := ynabInfo.LastUpdateHint.AddDate(0, 0, -7)
	res, err := sts.client.GetExpenses(splitwise.GetExpensesRequest{
		DatedAfter: &datedAfter,
	})
	if err != nil {
		return nil, err
	}
	var transactions []ynab.Transaction
	for _, e := range res.Expenses {
		if e.DeletedAt != nil {
			continue
		}
		user, rest := partionUsers(e.Users, sts.userID)
		if len(rest) > 1 {
			return nil, fmt.Errorf("not implemented: multi-user transactions")
		}

		net, err := netBalanceToMilliUnits(user.NetBalance)
		if err != nil {
			return nil, err
		}

		importId := strconv.Itoa(e.Id)
		transaction := ynab.Transaction{
			Amount:    -1 * net,
			PayeeName: rest[0].User.FirstName,
			Memo:      e.Description,
			Approved:  false,
			Date:      ynab.Date(e.CreatedAt.In(time.Local)),
			ImportId:  &importId,
		}
		transactions = append(transactions, transaction)
	}
	return transactions, nil
}

func netBalanceToMilliUnits(owed string) (int, error) {
	split := strings.Split(owed, ".")
	if len(split) != 2 {
		return 0, fmt.Errorf("invalid value")
	}
	dollars, err := strconv.Atoi(split[0])
	if err != nil {
		return 0, err
	}
	cents, err := strconv.Atoi(split[1])
	if err != nil {
		return 0, err
	}
	if dollars < 0 {
		cents *= -1
	}
	return (dollars*100 + cents) * 10, nil
}

func partionUsers(users []splitwise.ExpenseUser, userID int) (*splitwise.ExpenseUser, []splitwise.ExpenseUser) {
	var user *splitwise.ExpenseUser
	var other []splitwise.ExpenseUser
	for _, u := range users {
		if u.UserId == userID {
			user = &u
		} else {
			other = append(other, u)
		}
	}
	return user, other
}
