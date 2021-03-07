package splitwise

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Friend struct {
	ID        int              `json:"id"`
	FirstName string           `json:"first_name"`
	LastName  string           `json:"last_name"`
	Picture   Picture          `json:"picture"`
	Balance   []Balance        `json:"balance"`
	Groups    []BalanceByGroup `json:"groups"`
	UpdatedAt *time.Time       `json:"updated_at"`
}

type BalanceByGroup struct {
	GroupID int       `json:"group_id"`
	Balance []Balance `json:"balance"`
}

type Balance struct {
	CurrencyCode string `json:"currency_code"`
	Amount       string `json:"amount"`
}

type CreateFriendRequest struct {
	FirstName string
	LastName  string
	Email     string
}

func (c *Client) GetFriends(ctx context.Context) ([]Friend, error) {
	var res struct {
		Friends []Friend `json:"friends"`
	}
	err := c.get(ctx, "get_friends", &res)
	return res.Friends, err
}

func (c *Client) GetFriend(ctx context.Context, id int) (*Friend, error) {
	var res struct {
		Friend Friend `json:"friend"`
	}
	err := c.get(ctx, fmt.Sprintf("get_friend/%d", id), &res)
	return &res.Friend, err
}

func (c *Client) DeleteFriend(ctx context.Context, id int) error {
	var res struct {
		Success bool     `json:"success"`
		Errors  APIError `json:"errors"`
	}
	if err := c.do(
		ctx,
		http.MethodPost,
		&url.URL{
			Path: fmt.Sprintf("delete_friend/%d", id),
		},
		nil,
		&res,
	); err != nil {
		return err
	}
	if res.Success {
		return nil
	}
	return &res.Errors
}

func (c *Client) CreateFriend(ctx context.Context, req *CreateFriendRequest) (*Friend, error) {
	values := map[string][]string{
		"user_email":      {req.Email},
		"user_first_name": {req.FirstName},
		"user_last_name":  {req.LastName},
	}
	var res struct {
		Friend Friend `json:"friend"`
	}
	err := c.do(
		ctx,
		http.MethodPost,
		&url.URL{Path: "create_friend"},
		values,
		&res,
	)
	return &res.Friend, err
}

func (c *Client) CreateFriends(ctx context.Context, req ...*CreateFriendRequest) ([]Friend, error) {
	rw := newRequest()
	arr := rw.Array("friends")
	for _, user := range req {
		arr.Str("user_first_name", user.FirstName)
		arr.Str("user_last_name", user.LastName)
		arr.Str("user_email", user.Email)
		arr.Next()
	}
	var res struct {
		Friends []Friend `json:"friends"`
	}
	err := c.do(
		ctx,
		http.MethodPost,
		&url.URL{Path: "create_friend"},
		rw.Values,
		&res,
	)
	return res.Friends, err
}
