// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The oauth package provides support for making
// OAuth2-authenticated HTTP requests.
package oauth

// TODO(adg): A means of automatically saving credentials when updated.

import (
	"http"
	"json"
	"os"
	"time"
)

// Config is the configuration of an OAuth consumer.
type Config struct {
	ClientId     string
	ClientSecret string
	Scope        string
	AuthURL      string
	TokenURL     string
	RedirectURL  string // Defaults to out-of-band mode if empty.
}

func (c *Config) redirectURL() string {
	if c.RedirectURL != "" {
		return c.RedirectURL
	}
	return "oob"
}

// Token contains an end-user's tokens.
// This is the data you must store to persist authentication.
type Token struct {
	AccessToken  string "access_token"
	RefreshToken string "refresh_token"
	TokenExpiry  int64  "expires_in"
}

// Transport implements http.RoundTripper. When configured with a valid
// Config and Token it can be used to make authenticated HTTP requests.
//
//	t := &oauth.Transport{config}
//      t.Exchange(code)
//      // t now contains a valid Token
//	r, _, err := t.Client().Get("http://example.org/url/requiring/auth")
//
// It will automatically refresh the Token if it can,
// updating the supplied Token in place.
type Transport struct {
	*Config
	*Token

	// Transport is the HTTP transport to use when making requests.
	// It will default to http.DefaultTransport if nil.
	// (It should never be an oauth.Transport.)
	Transport http.RoundTripper
}

// Client returns an *http.Client that uses Transport to make requests.
func (t *Transport) Client() *http.Client {
	return &http.Client{t.transport()}
}

func (t *Transport) transport() http.RoundTripper {
	if t.Transport != nil {
		return t.Transport
	}
	return http.DefaultTransport
}

// AuthCodeURL returns a URL that the end-user should be redirected to,
// so that they may obtain an authorization code.
func (c *Config) AuthCodeURL(state string) string {
	url, err := http.ParseURL(c.AuthURL)
	if err != nil {
		panic("AuthURL malformed: " + err.String())
	}
	q := http.EncodeQuery(map[string][]string{
		"response_type": {"code"},
		"client_id":     {c.ClientId},
		"redirect_uri":  {c.redirectURL()},
		"scope":         {c.Scope},
		"state":         {state},
	})
	if url.RawQuery == "" {
		url.RawQuery = q
	} else {
		url.RawQuery += "&" + q
	}
	return url.String()
}

// Exchange takes a code and gets access Token from the remote server.
func (t *Transport) Exchange(code string) (tok *Token, err os.Error) {
	tok = new(Token)
	err = t.updateToken(tok, map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     t.ClientId,
		"client_secret": t.ClientSecret,
		"redirect_uri":  t.redirectURL(),
		"scope":         t.Scope,
		"code":          code,
	})
	if err != nil {
		t.Token = tok
	}
	return
}

// RoundTrip executes a single HTTP transaction using the Transport's
// Token as authorization headers.
func (t *Transport) RoundTrip(req *http.Request) (resp *http.Response, err os.Error) {
	if t.Config == nil {
		return nil, os.NewError("no Config supplied")
	}
	if t.Token == nil {
		return nil, os.NewError("no Token supplied")
	}

	// Make the HTTP request
	req.Header.Set("Authorization", "OAuth "+t.AccessToken)
	if resp, err = t.transport().RoundTrip(req); err != nil {
		return
	}

	// Refresh credentials if they're stale and try again
	if resp.StatusCode == 401 {
		if err = t.refresh(); err != nil {
			return
		}
		resp, err = t.transport().RoundTrip(req)
	}

	return
}

func (t *Transport) refresh() os.Error {
	return t.updateToken(t.Token, map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     t.ClientId,
		"client_secret": t.ClientSecret,
		"refresh_token": t.RefreshToken,
	})
}

func (t *Transport) updateToken(tok *Token, form map[string]string) os.Error {
	r, err := t.Client().PostForm(t.TokenURL, form)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return os.NewError("invalid response: " + r.Status)
	}
	if err = json.NewDecoder(r.Body).Decode(tok); err != nil {
		return err
	}
	if tok.TokenExpiry != 0 {
		tok.TokenExpiry = time.Seconds() + tok.TokenExpiry
	}
	return nil
}
