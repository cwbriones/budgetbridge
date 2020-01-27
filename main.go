package main

import (
	"budgetbridge/splitwise"
	"budgetbridge/ynab"
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"time"

	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
)

func main() {
	err := godotenv.Load()
	checkErr(err)
	clientKey := getenv("CLIENT_KEY")
	clientSecret := getenv("CLIENT_SECRET")
	accountId := getenv("ACCOUNT_ID")
	budgetId := getenv("BUDGET_ID")
	userId, _ := strconv.Atoi(getenv("USER_ID"))

	ctx := context.Background()
	authConfig := splitwise.NewConfig(clientKey, clientSecret)
	tokenSource := splitwise.CachingTokenSource{
		TokenSource: &splitwise.LocalServerTokenSource{
			Config: authConfig,
		},
		Path: ".splitwise.token",
	}
	splitwiseClient, err := splitwise.NewClientWithToken(ctx, authConfig, &tokenSource)
	checkErr(err)

	ynabToken := getenv("YNAB_TOKEN")
	ynabClient := ynab.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: ynabToken,
	}))

	// Get the most recent YNAB transaction from the last 30 days
	res, err := ynabClient.Transactions(ynab.TransactionsRequest{
		BudgetID:  budgetId,
		AccountID: accountId,
		SinceDate: time.Now().AddDate(0, 0, -30),
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

	for _, e := range expenses.Expenses {
		fmt.Printf("%#v\n", e)
	}

	// Convert to YNAB transactions
	transactions, err := convert(expenses.Expenses, userId, accountId)
	checkErr(err)

	for _, t := range transactions {
		fmt.Printf("%#v\n", t)
	}

	// Create
	if len(transactions) > 0 {
		request := ynab.CreateTransactionsRequest{
			Transactions: transactions,
		}
		res, err = ynabClient.CreateTransactions(budgetId, request)
		checkErr(err)

		fmt.Printf("%#v\n", res)
	}
}

func convert(expenses []splitwise.Expense, userId int, accountId string) ([]ynab.Transaction, error) {
	transactions := make([]ynab.Transaction, 0, len(expenses))

	for _, e := range expenses {
		user, rest := partionUsers(e.Users, userId)
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

func partionUsers(users []splitwise.ExpenseUser, userId int) (*splitwise.ExpenseUser, []splitwise.ExpenseUser) {
	var user *splitwise.ExpenseUser
	var other []splitwise.ExpenseUser
	for _, u := range users {
		if u.UserId == userId {
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
