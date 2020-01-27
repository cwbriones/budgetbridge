package main

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

	"budgetbridge/splitwise"

	"golang.org/x/oauth2"
)

// CachingTokenSource implements a TokenSource that writes its token to
// disk on a successful fetch.
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

func NewSplitwiseConfig(clientKey, clientSecret string) oauth2.Config {
	return oauth2.Config{
		ClientID:     clientKey,
		ClientSecret: clientSecret,
		Endpoint:     splitwise.Endpoint,
		RedirectURL:  "http://localhost:4000/auth_redirect",
	}
}

// LocalServerTokenSource implements a TokenSource by starting a local server to
// implement the standard oauth2 flow.
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

	resp, err := waitForCallback(":4000", state)
	if err != nil {
		return nil, fmt.Errorf("Callback failed: %w", err)
	}
	if resp.State != state {
		return nil, fmt.Errorf("Callback state mismatch")
	}
	return p.Config.Exchange(ctx, resp.Code, oauth2.AccessTypeOffline)
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

func waitForCallback(addr, csrfToken string) (resp callbackResponse, err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("Server panicked")
		}
	}()
	c := make(chan callbackResponse)
	var once sync.Once
	server := &http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			code := req.FormValue("code")
			state := req.FormValue("state")
			once.Do(func() {
				c <- callbackResponse{Code: code, State: state}
			})
			res.Write([]byte("✅ Go back to your terminal."))
		}),
	}
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	}()
	// TODO: Add a timeout
	resp = <-c
	err = server.Shutdown(context.TODO())
	return
}
