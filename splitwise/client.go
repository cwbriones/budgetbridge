package splitwise

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

var (
	baseApiUrl, _ = url.Parse("https://secure.splitwise.com/api/v3.0/")
)

// Client to the splitwise API.
type Client struct {
	*http.Client
}

func NewClient(httpClient *http.Client) *Client {
	return &Client{httpClient}
}

type GetExpensesRequest struct {
	DatedAfter    *time.Time
	DatedBefore   *time.Time
	UpdatedAfter  *time.Time
	UpdatedBefore *time.Time
	// Limit
	// Offset
}

type GetExpensesResponse struct {
	Expenses []Expense `json:"expenses"`
}

type GetCurrentUserResponse struct {
	User User `json:"user"`
}

type Category struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type User struct {
	FirstName string `json:"first_name"`
	Id        int    `json:"id"`
	LastName  string `json:"last_name"`
}

type ExpenseUser struct {
	NetBalance string `json:"net_balance"`
	OwedShare  string `json:"owed_share"`
	PaidShare  string `json:"paid_share"`
	UserId     int    `json:"user_id"`
	User       User   `json:"user"`
}

type Expense struct {
	Id          int           `json:"id"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	DeletedAt   *time.Time    `json:"deleted_at"`
	Category    Category      `json:"category"`
	Cost        string        `json:"cost"`
	Description string        `json:"description"`
	Users       []ExpenseUser `json:"users"`
}

func (c *Client) GetExpenses(req GetExpensesRequest) (res GetExpensesResponse, err error) {
	values := make(url.Values)
	if req.DatedAfter != nil {
		values.Add("dated_after", req.DatedAfter.Format("2006-01-02"))
	}
	err = c.doRequest("get_expenses?"+values.Encode(), &res)
	return
}

func (c *Client) GetCurrentUser() (res GetCurrentUserResponse, err error) {
	err = c.doRequest("get_current_user", &res)
	return
}

func (c *Client) doRequest(endpoint string, apiResponse interface{}) error {
	u, err := baseApiUrl.Parse(endpoint)
	if err != nil {
		return err
	}
	res, err := c.Client.Get(u.String())
	if err != nil {
		return err
	}
	defer res.Body.Close()
	decoder := json.NewDecoder(res.Body)
	if err := decoder.Decode(apiResponse); err != nil {
		return err
	}
	return nil
}
