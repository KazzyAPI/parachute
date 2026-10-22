package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

type Challenge struct {
	Scheme string
	Params map[string]string
}

type Authenticator struct {
	http      *http.Client
	userAgent string
	param     *regexp.Regexp
}

func NewAuthenticator(httpClient *http.Client, userAgent string) *Authenticator {
	return &Authenticator{http: httpClient, userAgent: userAgent, param: regexp.MustCompile(`(\w+)="([^"]*)"`)}
}

func (a *Authenticator) Parse(header string) Challenge {
	scheme, rest, _ := strings.Cut(header, " ")
	challenge := Challenge{Scheme: strings.ToLower(scheme), Params: map[string]string{}}
	for _, match := range a.param.FindAllStringSubmatch(rest, -1) {
		challenge.Params[strings.ToLower(match[1])] = match[2]
	}
	return challenge
}

func (a *Authenticator) Authorization(ctx context.Context, challenge Challenge, credential *Credential) (string, error) {
	switch challenge.Scheme {
	case "basic":
		if credential == nil {
			return "", nil
		}
		return a.basic(*credential), nil
	case "bearer":
		token, err := a.token(ctx, challenge, credential)
		if err != nil {
			return "", err
		}
		return "Bearer " + token, nil
	}
	return "", nil
}

func (a *Authenticator) basic(credential Credential) string {
	request := http.Request{Header: http.Header{}}
	request.SetBasicAuth(credential.Username, credential.Password)
	return request.Header.Get("Authorization")
}

func (a *Authenticator) token(ctx context.Context, challenge Challenge, credential *Credential) (string, error) {
	realm, err := url.Parse(challenge.Params["realm"])
	if err != nil || realm.Host == "" {
		return "", &Error{Reason: ReasonUnreachable, Err: fmt.Errorf("invalid token realm %q", challenge.Params["realm"])}
	}
	query := realm.Query()
	for _, key := range []string{"service", "scope"} {
		if value := challenge.Params[key]; value != "" {
			query.Set(key, value)
		}
	}
	realm.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, realm.String(), nil)
	if err != nil {
		return "", &Error{Reason: ReasonUnreachable, Err: err}
	}
	request.Header.Set("User-Agent", a.userAgent)
	if credential != nil {
		request.SetBasicAuth(credential.Username, credential.Password)
	}
	response, err := a.http.Do(request)
	if err != nil {
		return "", &Error{Reason: ReasonUnreachable, Err: fmt.Errorf("token request to %s: %w", realm.Host, err)}
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return "", &Error{Reason: ReasonUnauthorized, Err: fmt.Errorf("token request to %s: %s", realm.Host, response.Status)}
	}
	if response.StatusCode != http.StatusOK {
		return "", &Error{Reason: ReasonUnreachable, Err: fmt.Errorf("token request to %s: %s", realm.Host, response.Status)}
	}
	var body struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return "", &Error{Reason: ReasonUnreachable, Err: fmt.Errorf("token response from %s: %w", realm.Host, err)}
	}
	if body.Token != "" {
		return body.Token, nil
	}
	return body.AccessToken, nil
}
