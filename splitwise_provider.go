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

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
)

type SplitwiseTransactionProvider struct {
	userID     int
	client     *splitwise.Client
	categories map[string]CategorySpec
}

type SplitwiseOptions struct {
	UserID       *int           `json:"user_id"`
	ClientKey    string         `json:"client_key"`
	ClientSecret string         `json:"client_secret"`
	TokenCache   string         `json:"token_cache"`
	Categories   []CategorySpec `json:"category_mapping"`
}

type CategorySpec struct {
	Name string `json:"name"`

	YnabID   string `json:"ynab_id"`
	YnabName string `json:"ynab_name"`
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
		if res, err := client.GetCurrentUser(ctx); err != nil {
			return nil, err
		} else {
			userID = res.ID
		}
	} else {
		userID = *options.UserID
	}

	if len(options.Categories) == 0 {
		log.Debug().Msg("no category mapping specified")
	}
	categories := make(map[string]CategorySpec)
	for _, spec := range options.Categories {
		if _, ok := categories[spec.Name]; ok {
			return nil, fmt.Errorf("duplicate splitwise name in category mapping: %s", spec.Name)
		}
		categories[spec.Name] = spec
	}

	mapping := zerolog.Dict()
	for key, val := range categories {
		mapping = mapping.Dict(key, zerolog.Dict().
			Str("id", val.YnabID).
			Str("name", val.YnabName),
		)
	}
	log.Debug().
		Dict("options", zerolog.Dict().
			Int("user.id", userID).
			Dict("categoryMapping", mapping),
		).
		Msg("Creating splitwise provider")

	return &SplitwiseTransactionProvider{
		userID:     userID,
		categories: categories,
		client:     client,
	}, nil
}

func (sts *SplitwiseTransactionProvider) Transactions(ctx context.Context, ynabInfo YnabInfo) ([]ynab.Transaction, error) {
	log.Info().
		Int("user", sts.userID).
		Msg("Splitwise Transactions")
	// Get all splitwise transactions since this date
	// Go up to one week before hint
	datedAfter := ynabInfo.LastUpdateHint.AddDate(0, 0, -7)
	req := splitwise.GetExpensesRequest{
		DatedAfter: &datedAfter,
	}
	var transactions []ynab.Transaction
	for {
		expenses, err := sts.client.GetExpenses(ctx, &req)
		if err != nil {
			return nil, fmt.Errorf("get expenses: %s", err)
		}
		for _, e := range expenses {
			if e.DeletedAt != nil {
				continue
			}
			user, rest := partionUsers(e.Users, sts.userID)
			if len(rest) > 1 {
				return nil, fmt.Errorf("not implemented: multi-user transactions")
			}
			log.Debug().
				Str("expense", fmt.Sprintf("%+v", e)).
				Dict("user", zerolog.Dict().
					Str("NetBalance", user.NetBalance).
					Str("OwedShare", user.OwedShare).
					Str("PaidShare", user.PaidShare).
					Int("UserId", user.UserID).
					Str("FirstName", user.User.FirstName).
					Str("LastName", user.User.LastName),
				).
				Msg("expense")

			net, err := netBalanceToMilliUnits(user.NetBalance)
			if err != nil {
				return nil, err
			}

			importId := strconv.Itoa(e.ID)
			transaction := ynab.Transaction{
				Amount:    net,
				PayeeName: rest[0].User.FirstName,
				Memo:      e.Description,
				Approved:  false,
				Date:      ynab.Date(e.CreatedAt.In(time.Local)),
				ImportId:  &importId,
			}
			categoryId, ok := sts.mapCategory(ynabInfo.Categories, e.Category)
			if ok {
				log.Debug().
					Int("splitwise.category.id", e.Category.ID).
					Str("splitwise.category.Name", e.Category.Name).
					Str("ynab.category.id", categoryId).
					Msg("mapping found")
				transaction.CategoryId = &categoryId
			} else {
				log.Debug().
					Int("splitwise.category.id", e.Category.ID).
					Str("splitwise.category.Name", e.Category.Name).
					Msg("no mapping found for splitwise category")
			}

			transactions = append(transactions, transaction)
		}
		if len(expenses) == 0 {
			break
		}
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
	if m.YnabID != "" {
		log.Debug().Str("id", m.YnabID).Msg("mapping to ynab ID")
		ynabCategory, ok := ynabCategoriesByID[m.YnabID]
		if !ok {
			log.Warn().
				Str("categoryID", m.YnabID).
				Msg("unknown YNAB category ID in splitwise mapping")
			return "", false
		}
		return ynabCategory.Id, true
	}
	if m.Name != "" {
		log.Debug().Str("name", m.Name).Msg("mapping to ynab Name")
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
	if len(split[1]) == 1 {
		cents *= 10
	}
	return (dollars*100 + cents) * 10, nil
}

func partionUsers(users []splitwise.ExpenseUser, userID int) (splitwise.ExpenseUser, []splitwise.ExpenseUser) {
	var user splitwise.ExpenseUser
	var other []splitwise.ExpenseUser
	for _, u := range users {
		if u.UserID == userID {
			user = u
		} else {
			other = append(other, u)
		}
	}
	return user, other
}
