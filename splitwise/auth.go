package splitwise

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"

	"golang.org/x/oauth2"
)

type CachingTokenSource struct {
	oauth2.TokenSource
	Path string
}

func (cts *CachingTokenSource) Token() (*oauth2.Token, error) {
	if token, err := cts.get(); errors.Is(err, os.ErrNotExist) {
		token, err = cts.TokenSource.Token()
		if err != nil {
			return nil, err
		}
		err = cts.put(token)
		return token, err
	} else if err != nil {
		return nil, err
	} else {
		return token, nil
	}
}

func (cts *CachingTokenSource) get() (*oauth2.Token, error) {
	f, err := os.Open(cts.Path)
	if err != nil {
		return nil, err
	}
	r := bufio.NewReader(f)

	var token oauth2.Token
	err = json.NewDecoder(r).Decode(&token)
	return &token, err
}

func (cts *CachingTokenSource) put(token *oauth2.Token) error {
	f, err := os.Create(cts.Path)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	defer w.Flush()

	encoder := json.NewEncoder(w)
	return encoder.Encode(token)
}

type LocalServerTokenSource struct {
	Config oauth2.Config
}

func (p *LocalServerTokenSource) Token() (*oauth2.Token, error) {
	ctx := context.Background()
	state, err := newState()
	if err != nil {
		return nil, fmt.Errorf("Could not generate CSRF token: %w", err)
	}
	url := p.Config.AuthCodeURL(state, oauth2.AccessTypeOffline)
	fmt.Printf("Open this URL in the browser to authenticate.\n\n%s\n", url)

	resp, err := waitForCallback(state)
	if err != nil {
		return nil, fmt.Errorf("Callback failed: %w", err)
	}
	if resp.State != state {
		return nil, fmt.Errorf("Callback state mismatch")
	}
	return p.Config.Exchange(ctx, resp.Code, oauth2.AccessTypeOffline)
}

func NewConfig(clientKey, clientSecret string) oauth2.Config {
	return oauth2.Config{
		ClientID:     clientKey,
		ClientSecret: clientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://secure.splitwise.com/oauth/authorize",
			TokenURL: "https://secure.splitwise.com/oauth/token",
		},
		RedirectURL: "http://localhost:4000/auth_redirect",
	}
}

func NewClient(ctx context.Context, clientKey, clientSecret string) (*Client, error) {
	config := NewConfig(clientKey, clientSecret)
	tokenSource := &LocalServerTokenSource{
		Config: config,
	}
	return NewClientWithToken(ctx, config, tokenSource)
}

func NewClientWithToken(ctx context.Context, config oauth2.Config, tokenSource oauth2.TokenSource) (*Client, error) {
	token, err := tokenSource.Token()
	if err != nil {
		return nil, err
	}
	httpClient := config.Client(ctx, token)
	return &Client{httpClient}, nil
}

func newState() (string, error) {
	buf := make([]byte, 24)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}
	s := base64.URLEncoding.EncodeToString(buf)
	return s, nil
}

type callbackResponse struct {
	Code  string
	State string
}

func waitForCallback(csrfToken string) (resp callbackResponse, err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("Server panicked")
		}
	}()
	c := make(chan callbackResponse)
	var once sync.Once
	server := &http.Server{
		Addr: ":4000",
		Handler: http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			code := req.FormValue("code")
			state := req.FormValue("state")
			once.Do(func() {
				c <- callbackResponse{Code: code, State: state}
			})
			res.Write([]byte("Go back to your terminal. :)"))
		}),
	}
	go func() {
		err := server.ListenAndServe()
		if err != nil {
			panic(err)
		}
	}()
	// TODO: Add a timeout
	resp = <-c
	return
}
