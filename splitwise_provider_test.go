package main

import (
	"budgetbridge/splitwise"
	"budgetbridge/ynab"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExpensesToTransactions(t *testing.T) {
	userID := 456
	client := mockClient{
		expensesResponse: "fixtures/mock_expenses.json",
	}
	provider := SplitwiseTransactionProvider{
		userID,
		&client,
		make(map[string]CategoryMappingEntry),
	}

	r := require.New(t)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	txs, err := provider.Transactions(ctx, YnabInfo{})
	r.NoError(err)
	expected := []ynab.Transaction{
		{
			Date:      ynab.Date(time.Date(2020, 8, 9, 1, 00, 31, 0, time.UTC)),
			Amount:    -75000,
			PayeeName: "Annie",
			Memo:      "Groceries",
			ImportId:  stringPtr("1"),
		},
		{
			Date:      ynab.Date(time.Date(2020, 8, 3, 7, 42, 21, 0, time.UTC)),
			Amount:    -16500,
			PayeeName: "Annie",
			Memo:      "Dinner",
			ImportId:  stringPtr("2"),
		},
		{
			Date:      ynab.Date(time.Date(2020, 7, 5, 3, 1, 34, 0, time.UTC)),
			Amount:    67020,
			PayeeName: "Annie",
			Memo:      "Electric Bill",
			ImportId:  stringPtr("3"),
		},
	}
	r.Equal(expected, txs)
}

func TestNetBalanceToMilliunits(t *testing.T) {
	r := require.New(t)

	testcases := []struct {
		balance string
		units   int
		err     bool
	}{
		{
			balance: "81.1",
			units:   81100,
		},
		{
			balance: "1.23",
			units:   1230,
		},
		{
			balance: "-81.1",
			units:   -81100,
		},
		{
			balance: "-1.23",
			units:   -1230,
		},
	}
	for _, tc := range testcases {
		units, err := netBalanceToMilliUnits(tc.balance)
		r.NoError(err)
		r.Equal(tc.units, units)
	}
}

func TestCategoryMapping(t *testing.T) {
	mapping := make(CategoryMapping)
	// Both name and ID
	mapping.Add(CategoryMappingEntry{
		Name:     "Groceries",
		YnabId:   "1234",
		YnabName: "YnabGroceries",
	})
	// Only name
	mapping.Add(CategoryMappingEntry{
		Name:     "Internet",
		YnabName: "YnabInternet",
	})

	ynabCategories := []ynab.Category{
		{
			Id:   "1234",
			Name: "YnabGroceries",
		},
		{
			Id:   "4567",
			Name: "YnabInternet",
		},
	}

	r := require.New(t)
	id, _ := mapping.Categorize(ynabCategories, splitwise.Expense{
		Category: splitwise.Category{
			Name: "Groceries",
		},
	})
	r.Equal(id, "1234")

	id, _ = mapping.Categorize(ynabCategories, splitwise.Expense{
		Category: splitwise.Category{
			Name: "Internet",
		},
	})
	r.Equal(id, "4567")

	id, ok := mapping.Categorize(ynabCategories, splitwise.Expense{
		Category: splitwise.Category{
			Name: "Dining Out",
		},
	})
	r.Equal(id, "")
	r.False(ok)
}

type mockClient struct {
	expensesResponse string
}

func (c *mockClient) GetCurrentUser() (*splitwise.User, error) {
	var res splitwise.User
	return &res, nil
}

func (c *mockClient) GetExpenses(ctx context.Context, req *splitwise.GetExpensesRequest) ([]splitwise.Expense, error) {
	if req.Offset > 0 {
		return nil, nil
	}
	f, err := os.Open(c.expensesResponse)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	var res struct {
		Expenses []splitwise.Expense `json:"expenses"`
	}
	if err := decoder.Decode(&res); err != nil {
		return nil, err
	}
	req.Offset = len(res.Expenses)
	return res.Expenses, nil
}

func stringPtr(value string) *string {
	return &value
}
