package ynab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

var (
	baseApiUrl, _ = url.Parse("https://api.youneedabudget.com/v1/")
)

// Client to the YNAB API.
type Client struct {
	HttpClient *http.Client
}

func NewClient(client *http.Client) *Client {
	return &Client{client}
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
	Budgets       []BudgetSummary `json:"budgets"`
	DefaultBudget *BudgetSummary  `json:"default_budget"`
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

type TransactionsResponse struct {
	Transactions       []Transaction `json:"transactions"`
	DuplicateImportIDs []string      `json:"duplicate_import_ids"`
}

type CategoriesRequest struct {
	BudgetID string
}

type CategoriesResponse struct {
	CategoryGroups []CategoryGroup `json:"category_groups"`
}

type CategoryGroup struct {
	Id         string     `json:"id"`
	Name       string     `json:"name"`
	Hidden     bool       `json:"hidden"`
	Deleted    bool       `json:"deleted"`
	Categories []Category `json:"categories"`
}

type Category struct {
	Id                      string `json:"id"`
	CategoryGroupId         string `json:"category_group_id"`
	Name                    string `json:"name"`
	Hidden                  bool   `json:"hidden"`
	OriginalCategoryGroupId string `json:"original_category_group_id"`
	Note                    string `json:"note"`
	Budgeted                int    `json:"budgeted"`
	Activity                int    `json:"activity"`
	Balance                 int    `json:"balance"`
	GoalType                string `json:"goal_type"` // TODO: Make this its own type
	GoalCreationMonth       string `json:"goal_creation_month"`
	GoalTarget              int    `json:"goal_target"`
	GoalTargetMonth         string `json:"goal_target_month"`
	GoalPercentagComplete   int    `json:"goal_percentag_complete"`
	Deleted                 bool   `json:"deleted"`
}

type Date time.Time

func (d *Date) Time() time.Time {
	return time.Time(*d)
}

func (d *Date) String() string {
	return d.Time().Format("2006-01-02")
}

func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	t, err := time.Parse("2006-01-02", s)
	*d = Date(t)
	return err
}

func (d *Date) MarshalJSON() ([]byte, error) {
	s := time.Time(*d).Format("2006-01-02")
	return json.Marshal(s)
}

type Transaction struct {
	AccountId  string  `json:"account_id"`
	Date       Date    `json:"date"`
	Amount     int     `json:"amount"`
	PayeeId    *string `json:"payee_id,omitempty"`
	PayeeName  string  `json:"payee_name"`
	CategoryId *string `json:"category_id,omitempty"`
	Memo       string  `json:"memo"`
	ImportId   *string `json:"import_id,omitempty"`
	Approved   bool    `json:"approved"`
	// TODO
	// Cleared
	FlagColor *string `json:"flag_color,omitempty"`
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
	req, err := c.newRequest("GET", "budgets", nil)
	if err != nil {
		return
	}
	err = c.do(req, &response)
	return
}

func (c *Client) Accounts(budgetID string) (response AccountsResponse, err error) {
	u := fmt.Sprintf("budgets/%s/accounts", budgetID)
	req, err := c.newRequest("GET", u, nil)
	if err != nil {
		return
	}
	err = c.do(req, &response)
	return
}

func (c *Client) Account(budgetID, accountID string) (response AccountResponse, err error) {
	u := fmt.Sprintf("budgets/%s/accounts/%s", budgetID, accountID)
	req, err := c.newRequest("GET", u, nil)
	if err != nil {
		return
	}
	err = c.do(req, &response)
	return
}

type TransactionsRequest struct {
	BudgetID  string
	AccountID string
	SinceDate time.Time
}

func (c *Client) Transactions(request TransactionsRequest) (response TransactionsResponse, err error) {
	var u string
	if request.BudgetID == "" {
		return TransactionsResponse{}, fmt.Errorf("Missing BudgetID")
	} else if request.AccountID != "" {
		u = fmt.Sprintf("budgets/%s/accounts/%s/transactions", request.BudgetID, request.AccountID)
	} else {
		u = fmt.Sprintf("budgets/%s/transactions", request.BudgetID)
	}
	req, err := c.newRequest("GET", u, &request)
	if request.SinceDate.Unix() > 0 {
		qs := make(url.Values)
		qs.Add("since_date", request.SinceDate.Format("2006-01-02"))
		req.URL.RawQuery = qs.Encode()
	}
	if err != nil {
		return
	}
	err = c.do(req, &response)
	return
}

func (c *Client) CreateTransactions(budgetID string, request CreateTransactionsRequest) (response TransactionsResponse, err error) {
	u := fmt.Sprintf("budgets/%s/transactions", budgetID)
	req, err := c.newRequest("POST", u, &request)
	if err != nil {
		return
	}
	err = c.do(req, &response)
	return
}

func (c *Client) Categories(request CategoriesRequest) (response CategoriesResponse, err error) {
	u := fmt.Sprintf("budgets/%s/categories", request.BudgetID)
	req, err := c.newRequest("GET", u, &request)
	if err != nil {
		return
	}
	err = c.do(req, &response)
	return
}

func (c *Client) newRequest(method, path string, body interface{}) (*http.Request, error) {
	var buf io.ReadWriter
	if body != nil {
		buf = &bytes.Buffer{}
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return nil, err
		}
	}

	u, err := baseApiUrl.Parse(path)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, u.String(), buf)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Add("Content-Type", "application/json")
	}
	return req, nil
}

func (c *Client) do(req *http.Request, response interface{}) error {
	res, err := c.HttpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	decoder := json.NewDecoder(res.Body)

	apiResponse := struct {
		Data  interface{} `json:"data,omitempty"`
		Error *ApiError   `json:"error,omitempty"`
	}{
		Data: response,
	}
	if err := decoder.Decode(&apiResponse); err != nil {
		return err
	}
	if apiResponse.Error != nil {
		// This if-statement is necessary to avoid returning a typed nil
		return apiResponse.Error
	}
	return nil
}
