package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	ReasonUnreachable  = "registry_unreachable"
	ReasonUnauthorized = "registry_unauthorized"
	ReasonNotFound     = "not_found"
)

type Error struct {
	Reason string
	Err    error
}

func (e *Error) Error() string { return e.Reason + ": " + e.Err.Error() }

func (e *Error) Unwrap() error { return e.Err }

type HeadFailureLogger interface {
	HeadFailed(ctx context.Context, ref Reference, err error)
}

type Client struct {
	http      *http.Client
	auth      *Authenticator
	userAgent string
	accept    string
	head      HeadFailureLogger
}

func NewClient(httpClient *http.Client, auth *Authenticator, userAgent string, head HeadFailureLogger) *Client {
	return &Client{
		http:      httpClient,
		auth:      auth,
		userAgent: userAgent,
		head:      head,
		accept: strings.Join([]string{
			"application/vnd.oci.image.index.v1+json",
			"application/vnd.oci.image.manifest.v1+json",
			"application/vnd.docker.distribution.manifest.list.v2+json",
			"application/vnd.docker.distribution.manifest.v2+json",
		}, ", "),
	}
}

func (c *Client) Digest(ctx context.Context, ref Reference, credential *Credential) (string, error) {
	digest, err := c.request(ctx, http.MethodHead, ref, credential)
	if err == nil {
		return digest, nil
	}
	c.head.HeadFailed(ctx, ref, err)
	return c.request(ctx, http.MethodGet, ref, credential)
}

func (c *Client) request(ctx context.Context, method string, ref Reference, credential *Credential) (string, error) {
	response, err := c.send(ctx, method, ref, "")
	if err != nil {
		return "", err
	}
	if response.StatusCode == http.StatusUnauthorized {
		response, err = c.authenticated(ctx, method, ref, credential, response)
		if err != nil {
			return "", err
		}
	}
	digest, digestErr := c.digest(method, ref, response)
	if err := c.drain(response); err != nil && digestErr == nil {
		return "", &Error{Reason: ReasonUnreachable, Err: err}
	}
	return digest, digestErr
}

func (c *Client) authenticated(ctx context.Context, method string, ref Reference, credential *Credential, challenged *http.Response) (*http.Response, error) {
	challenge := c.auth.Parse(challenged.Header.Get("WWW-Authenticate"))
	if err := c.drain(challenged); err != nil {
		return nil, &Error{Reason: ReasonUnreachable, Err: err}
	}
	authorization, err := c.auth.Authorization(ctx, challenge, credential)
	if err != nil {
		return nil, err
	}
	if authorization == "" {
		return nil, &Error{Reason: ReasonUnauthorized, Err: fmt.Errorf("%s %s: authentication required", method, ref.APIHost())}
	}
	return c.send(ctx, method, ref, authorization)
}

func (c *Client) send(ctx context.Context, method string, ref Reference, authorization string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, ref.ManifestURL(), nil)
	if err != nil {
		return nil, &Error{Reason: ReasonUnreachable, Err: err}
	}
	request.Header.Set("Accept", c.accept)
	request.Header.Set("User-Agent", c.userAgent)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, &Error{Reason: ReasonUnreachable, Err: fmt.Errorf("%s %s: %w", method, ref.APIHost(), err)}
	}
	return response, nil
}

func (c *Client) digest(method string, ref Reference, response *http.Response) (string, error) {
	status := fmt.Errorf("%s %s: %s", method, ref.APIHost(), response.Status)
	switch {
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
		return "", &Error{Reason: ReasonUnauthorized, Err: status}
	case response.StatusCode == http.StatusNotFound:
		return "", &Error{Reason: ReasonNotFound, Err: status}
	case response.StatusCode != http.StatusOK:
		return "", &Error{Reason: ReasonUnreachable, Err: status}
	}
	if digest := response.Header.Get("Docker-Content-Digest"); digest != "" {
		return digest, nil
	}
	if method == http.MethodHead {
		return "", &Error{Reason: ReasonUnreachable, Err: fmt.Errorf("%s %s: no digest header", method, ref.APIHost())}
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, response.Body); err != nil {
		return "", &Error{Reason: ReasonUnreachable, Err: fmt.Errorf("%s %s: %w", method, ref.APIHost(), err)}
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func (c *Client) drain(response *http.Response) error {
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		return err
	}
	return response.Body.Close()
}
