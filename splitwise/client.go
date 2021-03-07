package splitwise

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"
)

var (
	baseAPIURL, _ = url.Parse("https://secure.splitwise.com/api/v3.0/")
)

var (
	// ErrNotFound is a shorthand for UnexpectedStatus{404}.
	//
	// It can be used to check if a response failed because the requested entity did not exist.
	ErrNotFound = UnexpectedStatus{Status: http.StatusNotFound}

	ErrEndOfQuery = errors.New("end of query")
)

// Client to the splitwise API.
type Client struct {
	HTTPClient
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// UnexpectedStatus indicates the client received an HTTP status code it was not expecting.
//
// Generally this is anything that is not a 2XX code, although this can differ between
// endpoints.
type UnexpectedStatus struct {
	Status int
}

func (he UnexpectedStatus) Error() string {
	return fmt.Sprintf("unexpected status %d", he.Status)
}

func (he UnexpectedStatus) Is(target error) bool {
	other, ok := target.(UnexpectedStatus)
	if !ok {
		return false
	}
	return he.Status == other.Status
}

func NewClient(httpClient HTTPClient) *Client {
	return &Client{httpClient}
}

type Registration int

const (
	RegistrationDummy Registration = iota
	RegistrationConfirmed
	RegistrationInvited
)

func (r Registration) String() string {
	return []string{
		"dummy",
		"confirmed",
		"invited",
	}[r]
}

func (r *Registration) UnmarshalJSON(bytes []byte) error {
	var name string
	if err := json.Unmarshal(bytes, &name); err != nil {
		return err
	}
	switch name {
	case "dummy":
		*r = RegistrationDummy
	case "confirmed":
		*r = RegistrationConfirmed
	case "invited":
		*r = RegistrationInvited
	default:
		return fmt.Errorf("unknown registration: '%s'", name)
	}
	return nil
}

type RepeatInterval int

const (
	RepeatNever RepeatInterval = iota
	RepeatWeekly
	RepeatFortnightly
	RepeatMonthly
	RepeatYearly
)

func (ri RepeatInterval) String() string {
	return []string{
		"never",
		"weekly",
		"fortnightly",
		"monthly",
		"yearly",
	}[ri]
}

func (ri *RepeatInterval) UnmarshalJSON(bytes []byte) error {
	var name string
	if err := json.Unmarshal(bytes, &name); err != nil {
		return err
	}
	switch name {
	case "never":
		*ri = RepeatNever
	case "weekly":
		*ri = RepeatWeekly
	case "fortnightly":
		*ri = RepeatFortnightly
	case "monthly":
		*ri = RepeatYearly
	case "yearly":
		*ri = RepeatYearly
	default:
		return fmt.Errorf("unknown repeat interval: '%s'", name)
	}
	return nil
}

type GetCategoriesResponse struct {
	Categories []Category `json:"categories"`
}

type Category struct {
	ID            int           `json:"id"`
	Name          string        `json:"name"`
	Subcategories []Subcategory `json:"subcategories"`
}

type Subcategory struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type User struct {
	ID           int          `json:"id"`
	FirstName    string       `json:"first_name"`
	LastName     string       `json:"last_name"`
	Picture      Picture      `json:"picture"`
	Email        string       `json:"email"`
	Registration Registration `json:"registration_status"`

	// Only for current user
	DefaultCurrency    string          `json:"default_currency"`
	Locale             string          `json:"locale"`
	NotificationsRead  *time.Time      `json:"notifications_read"` // the last time notifications were marked as read
	NotificationsCount int             `json:"notifications_count"`
	Notifications      NotificationSet `json:"notifications"`
}

type NotificationSet struct {
	AddedAsFriend  bool `json:"added_as_friend"`
	AddedToGroup   bool `json:"added_to_group"`
	ExpenseAdded   bool `json:"expense_added"`
	ExpenseUpdated bool `json:"expense_updated"`
	Bills          bool `json:"bills"`
	Payments       bool `json:"payments"`
	MonthlySummary bool `json:"monthly_summary"`
	Announcements  bool `json:"announcements"`
}

type Picture struct {
	Small  string `json:"small"`
	Medium string `json:"medium"`
	Large  string `json:"large"`
}

type ParseSentenceRequest struct {
	Input    string `json:"input"`
	GroupID  int    `json:"group_id"`
	FriendID int    `json:"friend_id"`
	Autosave bool   `json:"autosave"`
}

type ParseSentenceResponse struct {
	Expense Expense `json:"expense"`
	Valid   bool    `json:"valid"`
	Error   string  `json:"error"`
}

func (c *Client) GetCategories() (*GetCategoriesResponse, error) {
	var res GetCategoriesResponse
	err := c.get("get_categories", &res)
	return &res, err
}

func (c *Client) GetCurrentUser() (*User, error) {
	var res struct {
		User User `json:"user"`
	}
	err := c.get("get_current_user", &res)
	return &res.User, err
}

func (c *Client) GetUser(id int) (*User, error) {
	var res struct {
		User User `json:"user"`
	}
	err := c.get(fmt.Sprintf("get_user/%d", id), &res)
	return &res.User, err
}

func (c *Client) ParseSentence(req ParseSentenceRequest) (*ParseSentenceResponse, error) {
	var res ParseSentenceResponse
	err := c.do(http.MethodPost, &url.URL{Path: "parse_sentence"}, nil, &res)
	return &res, err
}

func (c *Client) get(endpoint string, apiResponse interface{}) error {
	return c.do(http.MethodGet, &url.URL{Path: endpoint}, nil, apiResponse)
}

func (c *Client) do(
	method string,
	u *url.URL,
	apiRequest url.Values,
	apiResponse interface{},
) error {
	u.Scheme = baseAPIURL.Scheme
	u.Host = baseAPIURL.Host
	u.Path = path.Join(baseAPIURL.Path, u.Path)

	var body io.Reader
	var contentLength int
	if apiRequest != nil {
		encoded := []byte(apiRequest.Encode())
		body = bytes.NewReader(encoded)
		contentLength = len(encoded)
	}
	req, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		return fmt.Errorf("could not construct request: %s", err)
	}
	if body != nil {
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Add("Content-Length", strconv.Itoa(contentLength))
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %s", err)
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case 200:
		break
	case 201:
		break
	case 404:
		return ErrNotFound
	default:
		return UnexpectedStatus{res.StatusCode}
	}
	decoder := json.NewDecoder(res.Body)
	if err := decoder.Decode(apiResponse); err != nil {
		return fmt.Errorf("decode: %s", err)
	}
	return nil
}
