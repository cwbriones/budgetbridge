package main

import (
	"budgetbridge/splitwise"
	"budgetbridge/ynab"
	"fmt"
	"log"
	"os"

	"context"
	"encoding/json"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	checkErr(err)
	clientKey := getenv("CLIENT_KEY")
	clientSecret := getenv("CLIENT_SECRET")
	ynabToken := getenv("YNAB_TOKEN")
	budgetId := getenv("ACCOUNT_ID")
	accountId := getenv("BUDGET_ID")

	ctx := context.Background()
	authConfig := splitwise.NewConfig(clientKey, clientSecret)
	tokenSource := splitwise.CachingTokenSource{
		TokenSource: &splitwise.LocalServerTokenSource{
			Config: authConfig,
		},
		Path: ".splitwise.token",
	}
	client, err := splitwise.NewClientWithToken(ctx, authConfig, &tokenSource)
	checkErr(err)

	if _, err := client.GetExpenses(); err != nil {
		checkErr(err)
	}

	ynabClient := ynab.Client{
		AccessToken: ynabToken,
	}
	if budgets, err := ynabClient.Account(accountId, budgetId); err != nil {
		checkErr(err)
	} else {
		marshalled, _ := json.MarshalIndent(budgets, "", "  ")
		fmt.Printf("%s\n", marshalled)
	}
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
