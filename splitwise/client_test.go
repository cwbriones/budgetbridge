package splitwise

import (
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"
)

type testcase struct {
	status int
	mock   string

	expectedPath string
	expectedBody map[string][]string
	f            func(client *Client) error
}

var (
	testcases = []testcase{
		{
			status: 201,
			mock:   "mocks/create_comment.json",
			f: func(client *Client) error {
				_, err := client.CreateComment(123, "hello, world!")
				return err
			},
		},
		{
			status: 201,
			mock:   "mocks/create_expense.json",
			f: func(client *Client) error {
				repeat := RepeatNever
				req := CreateExpenseRequest{
					Cost:           "20.00",
					Description:    "test",
					Payment:        true,
					Details:        stringPtr("Some more details?"),
					RepeatInterval: &repeat,

					SplitStrategy: SplitManually(
						UserShare{
							UserOption: ExistingUser(270896089),
							PaidShare:  "10.00",
						},
						UserShare{
							UserOption: NewUser(CreateFriendRequest{
								FirstName: "Alan",
								LastName:  "Turing",
								Email:     "hello@example.com",
							}),
							PaidShare: "5.00",
						},
						UserShare{
							UserOption: NewUser(CreateFriendRequest{
								FirstName: "Grace",
								LastName:  "Hopper",
								Email:     "hello@example.com",
							}),
							PaidShare: "5.00",
						},
					),
				}
				_, err := client.CreateExpense(req)
				return err
			},
		},
		{
			status: 403,
			mock:   "mocks/auth_error.json",
			f: func(client *Client) error {
				_, err := client.CreateComment(123, "hello, world!")
				return err
			},
		},
	}
)

func TestClient(t *testing.T) {
	for _, tc := range testcases {
		t.Run(tc.mock, func(t *testing.T) {
			_, err := makeRequest(tc.status, tc.mock, tc.f)
			if err == nil {
				return
			}
			statusErr := new(UnexpectedStatus)
			if !errors.As(err, statusErr) {
				t.Fatalf("unexpected error: %s", err)
			}
			if statusErr.Status != tc.status {
				t.Fatalf("unexpected status: %d", statusErr.Status)
			}
		})
	}
}

func makeRequest(status int, responsePath string, useClient func(client *Client) error) (url.Values, error) {
	var capturedValues url.Values
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		body, _ := ioutil.ReadAll(r.Body)
		r.Body.Close()
		requestValues, err := url.ParseQuery(string(body))
		capturedValues = requestValues
		if err != nil {
			panic(err)
		}

		f, err := os.Open(responsePath)
		if err != nil {
			panic(err)
		}
		stat, err := f.Stat()
		if err != nil {
			panic(err)
		}
		rw.Header().Add("Content-Type", "application/json")
		rw.Header().Add("Content-Length", strconv.Itoa(int(stat.Size())))
		rw.WriteHeader(status)
		io.Copy(rw, f)
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	if err != nil {
		return url.Values{}, err
	}
	client := &Client{
		&testHTTPClient{u: u},
	}
	err = useClient(client)
	return capturedValues, err
}

type testHTTPClient struct {
	u      *url.URL
	client http.Client
}

func (tc *testHTTPClient) Do(req *http.Request) (*http.Response, error) {
	req.URL.Host = tc.u.Host
	req.URL.Scheme = tc.u.Scheme
	return tc.client.Do(req)
}

func stringPtr(val string) *string { return &val }
