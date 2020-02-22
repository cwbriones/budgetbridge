package main

import (
	"budgetbridge/splitwise"
	swEndpoint "budgetbridge/splitwise/endpoint"
	"budgetbridge/ynab"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
)

type SplitwiseTransactionProvider struct {
	userID     int
	client     *splitwise.Client
	categories map[string]CategorySpec
}

type SplitwiseOptions struct {
	UserID       *int                    `json:"user_id"`
	ClientKey    string                  `json:"client_key"`
	ClientSecret string                  `json:"client_secret"`
	TokenCache   string                  `json:"token_cache"`
	Categories   map[string]CategorySpec `json:"categories"`
}

type CategorySpec struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

func (options *SplitwiseOptions) newSplitwiseClient(ctx context.Context) *splitwise.Client {
	httpClient := oauth2.NewClient(ctx, &CachingTokenSource{
		TokenSource: &LocalServerTokenSource{
			Config: oauth2.Config{
				ClientID:     options.ClientKey,
				ClientSecret: options.ClientSecret,
				Endpoint:     swEndpoint.Endpoint,
				RedirectURL:  "http://localhost:4000/auth_redirect",
			},
		},
		Path: options.TokenCache,
	})
	return splitwise.NewClient(httpClient)
}

func (options *SplitwiseOptions) NewProvider(ctx context.Context) (TransactionProvider, error) {
	client := options.newSplitwiseClient(ctx)

	var userID int
	if options.UserID == nil {
		if res, err := client.GetCurrentUser(); err != nil {
			return nil, err
		} else {
			userID = res.User.Id
		}
	} else {
		userID = *options.UserID
	}

	return &SplitwiseTransactionProvider{
		userID:     userID,
		categories: options.Categories,
		client:     client,
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
		categoryId, ok := sts.mapCategory(ynabInfo.Categories, e.Category)
		if ok {
			transaction.CategoryId = &categoryId
		}

		transactions = append(transactions, transaction)
	}
	return transactions, nil
}

func (sts *SplitwiseTransactionProvider) mapCategory(
	ynabCategories []ynab.Category,
	category splitwise.Category,
) (string, bool) {
	ynabCategoriesByName := make(map[string]ynab.Category)
	ynabCategoriesByID := make(map[string]ynab.Category)
	for _, c := range ynabCategories {
		ynabCategoriesByName[c.Name] = c
		ynabCategoriesByID[c.Id] = c
	}

	m, ok := sts.categories[category.Name]
	if !ok {
		return "", false
	}
	if m.ID != "" {
		ynabCategory, ok := ynabCategoriesByID[m.ID]
		if !ok {
			log.Warn().
				Str("categoryID", m.ID).
				Msg("unknown YNAB category ID in splitwise mapping")
			return "", false
		}
		return ynabCategory.Id, true
	}
	if m.Name != "" {
		ynabCategory, ok := ynabCategoriesByName[m.Name]
		if !ok {
			log.Warn().
				Str("name", m.Name).
				Msg("unknown YNAB category name in splitwise mapping")
			return "", false
		}
		return ynabCategory.Id, true
	}
	return "", false
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
