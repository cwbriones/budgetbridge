package splitwise

import "golang.org/x/oauth2"

var Endpoint = oauth2.Endpoint{
	AuthURL:  "https://secure.splitwise.com/oauth/authorize",
	TokenURL: "https://secure.splitwise.com/oauth/token",
}
