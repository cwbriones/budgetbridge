package splitwise

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	baseApiUrl = "https://secure.splitwise.com/api/v3.0"
)

// Client to the splitwise API.
type Client struct {
	*http.Client
}

type GetExpensesResponse struct {
	Expenses []Expense `json:"expenses"`
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
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	Category    Category      `json:"category"`
	Cost        string        `json:"cost"`
	Description string        `json:"description"`
	Users       []ExpenseUser `json:"users"`
}

func (c *Client) GetExpenses() (response GetExpensesResponse, err error) {
	err = c.doRequest("get_expenses", &response)
	return
}

func (c *Client) doRequest(endpoint string, apiResponse interface{}) error {
	fullEndpoint := fmt.Sprintf("%s/%s", baseApiUrl, endpoint)
	res, err := c.Client.Get(fullEndpoint)
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
