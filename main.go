package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"golang.org/x/oauth2"

	"budgetbridge/splitwise"
	"budgetbridge/ynab"
)

type Config struct {
	BudgetID     string          `toml:"budget_id"`
	AccountID    string          `toml:"account_id"`
	AccessToken  string          `toml:"access_token"`
	Splitwise    SplitwiseConfig `toml:"splitwise"`
	LookBackDays int             `toml:"lookback_days"`
}

type SplitwiseConfig struct {
	UserID       int    `toml:"user_id"`
	ClientKey    string `toml:"client_key"`
	ClientSecret string `toml:"client_secret"`
	TokenCache   string `toml:"token_cache"`
}

func main() {
	var config Config
	_, err := toml.DecodeFile("config.toml", &config)
	checkErr(err)

	ctx := context.Background()
	authConfig := splitwise.NewConfig(config.Splitwise.ClientKey, config.Splitwise.ClientSecret)
	tokenSource := splitwise.CachingTokenSource{
		TokenSource: &splitwise.LocalServerTokenSource{
			Config: authConfig,
		},
		Path: config.Splitwise.TokenCache,
	}
	splitwiseClient, err := splitwise.NewClientWithToken(ctx, authConfig, &tokenSource)
	checkErr(err)

	ynabClient := ynab.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: config.AccessToken,
	}))

	// Get the most recent YNAB transactions from the last month
	res, err := ynabClient.Transactions(ynab.TransactionsRequest{
		BudgetID:  config.BudgetID,
		AccountID: config.AccountID,
		SinceDate: time.Now().AddDate(0, 0, -config.LookBackDays),
	})
	checkErr(err)

	// Go up to one week before that
	mostRecentDate := time.Time(res.Transactions[len(res.Transactions)-1].Date).AddDate(0, 0, -7)
	fmt.Printf("Fetching up to date %s\n", mostRecentDate.String())

	// Get all splitwise transactions since this date
	expenses, err := splitwiseClient.GetExpenses(splitwise.GetExpensesRequest{
		DatedAfter: &mostRecentDate,
	})
	checkErr(err)

	// Convert to YNAB transactions
	transactions, err := convert(expenses.Expenses, config.Splitwise.UserID, config.AccountID)
	checkErr(err)

	for _, t := range transactions {
		fmt.Printf("%#v\n", t)
	}

	// Create
	if len(transactions) > 0 {
		request := ynab.CreateTransactionsRequest{
			Transactions: transactions,
		}
		res, err = ynabClient.CreateTransactions(config.BudgetID, request)
		checkErr(err)

		fmt.Printf("%#v\n", res)
	}
}

func convert(expenses []splitwise.Expense, userID int, accountId string) ([]ynab.Transaction, error) {
	transactions := make([]ynab.Transaction, 0, len(expenses))

	for _, e := range expenses {
		user, rest := partionUsers(e.Users, userID)
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
			AccountId: accountId,
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

func checkErr(err error) {
	if err != nil {
		log.Fatalf("[error]: %s\n", err)
	}
}

func getenv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("[error]: Missing env var %s", key)
	}
	return value
}
