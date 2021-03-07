package splitwise

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Group struct {
	ID                int           `json:"id"`
	Name              string        `json:"name"`
	UpdatedAt         *time.Time    `json:"updated_at"`
	Members           []GroupMember `json:"members"`
	SimplifyByDefault bool          `json:"simplify_by_default"`
	OriginalDebts     []GroupDebt   `json:"original_debts"`
	GroupType         GroupType     `json:"group_type"`
}

type GroupType int

const (
	GroupTypeOther GroupType = iota
	GroupTypeApartment
	GroupTypeHouse
	GroupTypeTrip
)

func (gt GroupType) String() string {
	return []string{
		"other",
		"apartment",
		"house",
		"trip",
	}[gt]
}

func (gt *GroupType) UnmarshalJSON(bytes []byte) error {
	var name string
	if err := json.Unmarshal(bytes, &name); err != nil {
		return err
	}
	switch name {
	case "other":
		*gt = GroupTypeOther
	case "apartment":
		*gt = GroupTypeApartment
	case "house":
		*gt = GroupTypeHouse
	case "trip":
		*gt = GroupTypeTrip
	default:
		return fmt.Errorf("unknown group: '%s'", name)
	}
	return nil
}

func (gt GroupType) MarshalJSON() ([]byte, error) {
	return json.Marshal(gt.String())
}

type GroupMember struct {
	ID                int           `json:"id"`
	FirstName         string        `json:"first_name"`
	LastName          string        `json:"last_name"`
	Picture           Picture       `json:"picture"`
	Email             string        `json:"email"`
	Registration      Registration  `json:"registration_status"`
	Balance           []Balance     `json:"balance"`
	Members           []GroupMember `json:"members"`
	SimplifyByDefault bool          `json:"simplify_by_default"`
	OriginalDebts     []GroupDebt   `json:"original_debts"`
}

type GroupDebt struct {
	From         int    `json:"from"`
	To           int    `json:"to"`
	Amount       string `json:"amount"`
	CurrencyCode string `json:"currency_code"`
}

type CreateGroupRequest struct {
	Name              string    `json:"name"`
	Whiteboard        string    `json:"whiteboard"`
	GroupType         GroupType `json:"group_type"`
	SimplifyByDefault bool      `json:"simplify_by_default"`
}

type UserOption interface {
	prepareRequest(rw requestWriter)
}

type userOptionFunc func(rw requestWriter)

func (f userOptionFunc) prepareRequest(rw requestWriter) {
	f(rw)
}

// ExistingUser references an already-existing user by their ID.
func ExistingUser(userID int) UserOption {
	return userOptionFunc(func(rw requestWriter) {
		rw.Int("user_id", userID)
	})
}

// NewUser will create a new user within this request
func NewUser(req CreateFriendRequest) UserOption {
	return userOptionFunc(func(rw requestWriter) {
		rw.Str("first_name", req.FirstName)
		rw.Str("last_name", req.LastName)
		rw.Str("email", req.Email)
	})
}

func (c *Client) GetGroups(ctx context.Context) ([]Group, error) {
	var res struct {
		Groups []Group
	}
	err := c.get(ctx, "get_groups", &res)
	return res.Groups, err
}

func (c *Client) GetGroup(ctx context.Context, id int) (*Group, error) {
	var res struct {
		Group Group
	}
	err := c.get(ctx, fmt.Sprintf("get_group/%d", id), &res)
	return &res.Group, err
}

func (c *Client) CreateGroup(ctx context.Context, req CreateGroupRequest, user UserOption, users ...UserOption) (*Group, error) {
	rw := newRequest()

	rw.Str("name", req.Name)
	rw.Str("whiteboard", req.Whiteboard)
	rw.Str("group_type", req.GroupType.String())
	rw.Bool("simplify_by_default", req.SimplifyByDefault)

	arr := rw.Array("users")
	user.prepareRequest(arr)
	arr.Next()
	for _, u := range users {
		u.prepareRequest(arr)
		arr.Next()
	}
	var res struct {
		Group  Group    `json:"group"`
		Errors APIError `json:"errors"`
	}
	err := c.do(
		ctx,
		http.MethodPost,
		&url.URL{Path: "create_group"},
		rw.Values,
		&res,
	)
	if res.Errors.Len() > 0 {
		return nil, &res.Errors
	}
	return &res.Group, err
}

func (c *Client) DeleteGroup(ctx context.Context, id int) error {
	var res struct {
		Success bool     `json:"success"`
		Errors  APIError `json:"errors"`
	}
	err := c.do(
		ctx,
		http.MethodPost,
		&url.URL{Path: fmt.Sprintf("delete_group/%d", id)},
		nil,
		&res,
	)
	if err != nil {
		return err
	}
	if res.Success {
		return nil
	}
	return &res.Errors
}

func (c *Client) UndeleteGroup(ctx context.Context, id int) error {
	var res struct {
		Success bool     `json:"success"`
		Errors  APIError `json:"errors"`
	}
	err := c.do(
		ctx,
		http.MethodPost,
		&url.URL{Path: fmt.Sprintf("undelete_group/%d", id)},
		nil,
		&res,
	)
	if err != nil {
		return err
	}
	if res.Success {
		return nil
	}
	return &res.Errors
}

func (c *Client) AddUserToGroup(ctx context.Context, id int, user UserOption) error {
	rw := newRequest()
	rw.Int("group_id", id)
	user.prepareRequest(rw)
	var res struct {
		Success bool     `json:"success"`
		Errors  APIError `json:"errors"`
	}
	err := c.do(
		ctx,
		http.MethodPost,
		&url.URL{Path: "add_user_to_group"},
		rw.Values,
		&res,
	)
	if err != nil {
		return err
	}
	if res.Success {
		return nil
	}
	return &res.Errors
}

func (c *Client) RemoveUserFromGroup(ctx context.Context, id int, userID int) error {
	req := url.Values{
		"group_id": []string{strconv.Itoa(id)},
		"user_id":  []string{strconv.Itoa(userID)},
	}
	var res struct {
		Success bool     `json:"success"`
		Errors  APIError `json:"errors"`
	}
	err := c.do(
		ctx,
		http.MethodPost,
		&url.URL{Path: "remove_user_from_group"},
		req,
		&res,
	)
	if err != nil {
		return err
	}
	if res.Success {
		return nil
	}
	return &res.Errors
}
