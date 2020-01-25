package ynab

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	baseApiUrl = "https://api.youneedabudget.com/v1"
)

// Client to the YNAB API.
type Client struct {
	HttpClient  http.Client
	AccessToken string
}

type ApiResponse struct {
	Data  interface{} `json:"data,omitempty"`
	Error *ApiError   `json:"error,omitempty"`
}

type ApiError struct {
	Id     string `json:"id"`
	Name   string `json:"name"`
	Detail string `json:"detail"`
}

func (err *ApiError) Error() string {
	return err.Detail
}

type BudgetsResponse struct {
	Budgets []BudgetSummary `json:"budgets"`
}

type BudgetSummary struct {
	Id             string         `json:"id"`
	Name           string         `json:"name"`
	LastModifiedOn time.Time      `json:"last_modified_on"`
	DateFormat     DateFormat     `json:"date_format"`
	CurrencyFormat CurrencyFormat `json:"currency_format"`
	FirstMonth     string         `json:"first_month"`
	LastMonth      string         `json:"last_month"`
}

type DateFormat struct {
	Format string `json:"format"`
}

type CurrencyFormat struct {
	IsoCode          string `json:"iso_code"`
	ExampleFormat    string `json:"example_format"`
	DecimalDigits    int    `json:"decimal_digits"`
	DecimalSeparator string `json:"decimal_separator"`
	SymbolFirst      bool   `json:"symbol_first"`
	GroupSeparator   string `json:"group_separator"`
	CurrencySymbol   string `json:"currency_symbol"`
	DisplaySymbol    bool   `json:"display_symbol"`
}

type CreateTransactionsRequest struct {
	Transactions []Transaction `json:"transactions"`
}

type Transaction struct {
	AccountId  string    `json:"account_id"`
	Date       time.Time `json:"date"`
	Amount     int       `json:"amount"`
	PayeeId    string    `json:"payee_id"`
	PayeeName  string    `json:"payee_name"`
	CategoryId string    `json:"category_id"`
	Memo       string    `json:"memo"`
	ImportId   string    `json:"import_id"`
	Approved   bool      `json:"approved"`
	// TODO
	// Cleared
	// FlagColor
}

type AccountsResponse struct {
	Accounts []Account `json:"accounts"`
}

type AccountResponse struct {
	Account Account `json:"account"`
}

type Account struct {
	Id               string `json:"id"`
	Name             string `json:"name"`
	OnBudget         bool   `json:"on_budget"`
	Closed           bool   `json:"closed"`
	Note             string `json:"note"`
	Balance          int    `json:"balance"`
	ClearedBalance   int    `json:"cleared_balance"`
	UnclearedBalance int    `json:"uncleared_balance"`
	TransferPayeeId  string `json:"transfer_payee_id"`
	Deleted          bool   `json:"deleted"`

	// FIXME
	Type string `json:"type"`
}

func (c *Client) Budgets() (response BudgetsResponse, err error) {
	err = c.doRequest(&response, "budgets")
	return
}

func (c *Client) Accounts(budgetID string) (response AccountsResponse, err error) {
	err = c.doRequest(&response, "budgets", budgetID, "accounts")
	return
}

func (c *Client) Account(budgetID, accountID string) (response AccountResponse, err error) {
	err = c.doRequest(&response, "budgets", budgetID, "accounts", accountID)
	return
}

func (c *Client) doRequest(response interface{}, path ...string) error {
	endpoint := strings.Join(path, "/")
	fullEndpoint := fmt.Sprintf("%s/%s", baseApiUrl, endpoint)

	req, err := http.NewRequest(http.MethodGet, fullEndpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.AccessToken))
	res, err := c.HttpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	decoder := json.NewDecoder(res.Body)

	apiResponse := ApiResponse{
		Data: response,
	}
	if err := decoder.Decode(&apiResponse); err != nil {
		return err
	}
	// You can't return apiResponse.Error directly because it is a typed nil
	if apiResponse.Error != nil {
		return apiResponse.Error
	}
	return nil
}
