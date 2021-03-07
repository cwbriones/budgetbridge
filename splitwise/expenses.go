package splitwise

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type SplitStrategy interface {
	prepareRequest(vw valueWriter)
}

type splitStrategyFunc func(vw valueWriter)

func (f splitStrategyFunc) prepareRequest(vw valueWriter) {
	f(vw)
}

func SplitEqually(groupID int) SplitStrategy {
	return splitStrategyFunc(func(vw valueWriter) {
		vw.Int("group_id", groupID)
	})
}

func SplitManually(users ...UserShare) SplitStrategy {
	return splitStrategyFunc(func(vw valueWriter) {
		arr := vw.Array("users")
		for _, user := range users {
			user.UserOption.prepareRequest(arr)
			arr.Str("owed_share", user.OwedShare)
			arr.Str("paid_share", user.PaidShare)
			arr.Next()
		}
	})
}

type UserShare struct {
	UserOption UserOption
	PaidShare  string
	OwedShare  string
}

type CreateExpenseRequest struct {
	Cost        string `json:"cost"`
	Description string `json:"description"`
	Payment     bool   `json:"payment"`

	SplitStrategy SplitStrategy

	// Optional parameters

	Details        *string         `json:"details"`
	Date           *time.Time      `json:"date"`
	RepeatInterval *RepeatInterval `json:"repeat_interval"`
	CurrencyCode   *string         `json:"currency_code"`
	CategoryID     *int            `json:"category_id"`
}

type ExpenseUser struct {
	NetBalance string `json:"net_balance"`
	OwedShare  string `json:"owed_share"`
	PaidShare  string `json:"paid_share"`
	UserID     int    `json:"user_id"`
	User       User   `json:"user"`
}

type Expense struct {
	ID          int           `json:"id"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	DeletedAt   *time.Time    `json:"deleted_at"`
	Category    Category      `json:"category"`
	Cost        string        `json:"cost"`
	Description string        `json:"description"`
	Users       []ExpenseUser `json:"users"`
}

type GetExpensesRequest struct {
	DatedAfter    *time.Time `json:"dated_after"`
	DatedBefore   *time.Time `json:"dated_before"`
	UpdatedAfter  *time.Time `json:"updated_after"`
	UpdatedBefore *time.Time `json:"updated_before"`
	Limit         int        `json:"limit"`
	Offset        int        `json:"offset"`
}

type Comment struct {
	ID           int        `json:"id"`
	Content      string     `json:"content"`
	CommentType  string     `json:"comment_type"`
	RelationType string     `json:"relation_type"`
	RelationID   int        `json:"relation_id"`
	CreatedAt    *time.Time `json:"created_at"`
	DeletedAt    *time.Time `json:"deleted_at"`
	User         User       `json:"user"`
}

func (c *Client) CreateExpense(ctx context.Context, req CreateExpenseRequest) (*Expense, error) {
	var res struct {
		Expense Expense  `json:"expense"`
		Errors  APIError `json:"errors"`
	}
	rw := newRequest()
	rw.Str("cost", req.Cost)
	rw.Str("description", req.Description)
	rw.Bool("payment", req.Payment)
	if req.Details != nil {
		rw.Str("details", *req.Details)
	}
	if req.RepeatInterval != nil {
		rw.Set("repeat_interval", req.RepeatInterval.String())
	}
	if req.CurrencyCode != nil {
		rw.Str("currency_code", *req.CurrencyCode)
	}
	if req.CategoryID != nil {
		rw.Int("category_id", *req.CategoryID)
	}
	req.SplitStrategy.prepareRequest(rw)
	err := c.do(
		ctx,
		http.MethodPost,
		&url.URL{Path: "create_expense"},
		rw.Values,
		&res,
	)
	if err != nil {
		return nil, err
	}
	if res.Errors.Len() > 0 {
		return nil, &res.Errors
	}
	return &res.Expense, nil
}

func (c *Client) GetExpense(ctx context.Context, id int) (*Expense, error) {
	var res struct {
		Expense Expense `json:"expense"`
	}
	err := c.get(ctx, fmt.Sprintf("get_expense/%d", id), &res)
	return &res.Expense, err
}

func (c *Client) GetExpenses(ctx context.Context, req *GetExpensesRequest) ([]Expense, error) {
	values := make(url.Values)
	if req.DatedAfter != nil {
		values.Add("dated_after", req.DatedAfter.Format("2006-01-02"))
	}
	if req.DatedBefore != nil {
		values.Add("dated_before", req.DatedBefore.Format("2006-01-02"))
	}
	if req.UpdatedBefore != nil {
		values.Add("updated_before", req.UpdatedBefore.Format("2006-01-02"))
	}
	if req.UpdatedAfter != nil {
		values.Add("updated_after", req.UpdatedAfter.Format("2006-01-02"))
	}
	if req.Offset > 0 {
		values.Add("offset", strconv.Itoa(req.Offset))
	}
	if req.Limit > 0 {
		values.Add("limit", strconv.Itoa(req.Limit))
	}
	var res struct {
		Expenses []Expense `json:"expenses"`
	}
	u := &url.URL{
		Path:     "get_expenses",
		RawQuery: values.Encode(),
	}
	if err := c.do(ctx, http.MethodGet, u, nil, &res); err != nil {
		return nil, err
	}
	req.Offset = len(res.Expenses)
	return res.Expenses, nil
}

func (c *Client) DeleteExpense(ctx context.Context, id int) error {
	var res struct {
		Success *bool `json:"success"`
	}
	if err := c.do(ctx, http.MethodPost, &url.URL{Path: fmt.Sprintf("delete_expense/%d", id)}, nil, &res); err != nil {
		return err
	}
	if res.Success != nil && *res.Success {
		return nil
	}
	return errors.New("unsuccessful")
}

func (c *Client) UndeleteExpense(ctx context.Context, id int) error {
	var res struct {
		Success *bool `json:"success"`
	}
	if err := c.do(ctx, http.MethodPost, &url.URL{Path: fmt.Sprintf("undelete_expense/%d", id)}, nil, &res); err != nil {
		return err
	}
	if res.Success != nil && *res.Success {
		return nil
	}
	return errors.New("unsuccessful")
}

func (c *Client) GetComments(ctx context.Context, expenseID int) ([]Comment, error) {
	var res struct {
		Comments []Comment `json:"comments"`
	}
	err := c.get(ctx, "get_comments", &res)
	return res.Comments, err
}

func (c *Client) CreateComment(ctx context.Context, expenseID int, content string) (*Comment, error) {
	values := url.Values{
		"expense_id": []string{strconv.Itoa(expenseID)},
		"content":    []string{content},
	}
	var res struct {
		Comment Comment  `json:"comment"`
		Errors  APIError `json:"errors"`
	}
	err := c.do(ctx, http.MethodPost, &url.URL{Path: "create_comment"}, values, &res)
	if err != nil {
		return nil, err
	}
	if res.Errors.Len() > 0 {
		return nil, &res.Errors
	}
	return &res.Comment, nil
}

func (c *Client) GetComment(ctx context.Context, id int) (*Comment, error) {
	var res struct {
		Comment Comment  `json:"comment"`
		Errors  APIError `json:"errors"`
	}
	err := c.get(ctx, fmt.Sprintf("get_comment/%d", id), &res)
	if err != nil {
		return nil, err
	}
	if res.Errors.Len() > 0 {
		return nil, &res.Errors
	}
	return &res.Comment, nil
}

func (c *Client) DeleteComment(ctx context.Context, id int) (*Comment, error) {
	var res struct {
		Comment Comment  `json:"comment"`
		Errors  APIError `json:"errors"`
	}
	err := c.get(ctx, fmt.Sprintf("delete_comment/%d", id), &res)
	if err != nil {
		return nil, err
	}
	if res.Errors.Len() > 0 {
		return nil, &res.Errors
	}
	return &res.Comment, nil
}
