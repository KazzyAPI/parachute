package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

const (
	ReasonUnreachable  = "registry_unreachable"
	ReasonUnauthorized = "registry_unauthorized"
	ReasonNotFound     = "not_found"
	maxManifestBytes   = 4 << 20
	maxBodyBytes       = 1 << 20
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
	digest    *regexp.Regexp
}

func NewClient(httpClient *http.Client, auth *Authenticator, userAgent string, head HeadFailureLogger) *Client {
	return &Client{
		http:      httpClient,
		auth:      auth,
		userAgent: userAgent,
		head:      head,
		digest:    regexp.MustCompile(`^sha256:[0-9a-f]{64}$`),
		accept: strings.Join([]string{
			"application/vnd.oci.image.index.v1+json",
			"application/vnd.oci.image.manifest.v1+json",
			"application/vnd.docker.distribution.manifest.list.v2+json",
			"application/vnd.docker.distribution.manifest.v2+json",
		}, ", "),
	}
}

func (c *Client) Digest(ctx context.Context, ref Reference, credential *Credential) (string, error) {
	head, err := c.request(ctx, http.MethodHead, ref, credential)
	if err == nil && c.digest.MatchString(head.digest) {
		return head.digest, nil
	}
	if err == nil {
		err = &Error{Reason: ReasonUnreachable, Err: fmt.Errorf("HEAD %s: missing or invalid Docker-Content-Digest header", ref.APIHost())}
		c.head.HeadFailed(ctx, ref, err)
		return "", err
	}
	c.head.HeadFailed(ctx, ref, err)
	get, err := c.request(ctx, http.MethodGet, ref, credential)
	if err != nil {
		return "", err
	}
	if get.digest != "" && get.digest != get.body {
		return "", &Error{Reason: ReasonUnreachable, Err: fmt.Errorf("GET %s: Docker-Content-Digest header does not match the manifest", ref.APIHost())}
	}
	return get.body, nil
}

type reply struct {
	status    int
	text      string
	challenge string
	digest    string
	body      string
}

func (c *Client) request(ctx context.Context, method string, ref Reference, credential *Credential) (reply, error) {
	answer, err := c.exchange(ctx, method, ref, "")
	if err != nil {
		return reply{}, err
	}
	if answer.status == http.StatusUnauthorized {
		authorization, err := c.auth.Authorization(ctx, c.auth.Parse(answer.challenge), credential)
		if err != nil {
			return reply{}, err
		}
		if authorization == "" {
			return reply{}, &Error{Reason: ReasonUnauthorized, Err: fmt.Errorf("%s %s: authentication required", method, ref.APIHost())}
		}
		if answer, err = c.exchange(ctx, method, ref, authorization); err != nil {
			return reply{}, err
		}
	}
	return answer, c.interpret(method, ref, answer)
}

func (c *Client) exchange(ctx context.Context, method string, ref Reference, authorization string) (answer reply, err error) {
	request, err := http.NewRequestWithContext(ctx, method, ref.ManifestURL(), nil)
	if err != nil {
		return reply{}, &Error{Reason: ReasonUnreachable, Err: err}
	}
	request.Header.Set("Accept", c.accept)
	request.Header.Set("User-Agent", c.userAgent)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return reply{}, &Error{Reason: ReasonUnreachable, Err: fmt.Errorf("%s %s: %w", method, ref.APIHost(), err)}
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil && err == nil {
			err = &Error{Reason: ReasonUnreachable, Err: fmt.Errorf("%s %s: %w", method, ref.APIHost(), closeErr)}
		}
	}()
	answer = reply{status: response.StatusCode, text: response.Status, challenge: response.Header.Get("WWW-Authenticate"), digest: response.Header.Get("Docker-Content-Digest")}
	if method != http.MethodGet || answer.status != http.StatusOK {
		return answer, nil
	}
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(response.Body, maxManifestBytes+1))
	if err != nil {
		return reply{}, &Error{Reason: ReasonUnreachable, Err: fmt.Errorf("%s %s: %w", method, ref.APIHost(), err)}
	}
	if size > maxManifestBytes {
		return reply{}, &Error{Reason: ReasonUnreachable, Err: fmt.Errorf("%s %s: manifest larger than %d bytes", method, ref.APIHost(), maxManifestBytes)}
	}
	answer.body = "sha256:" + hex.EncodeToString(hash.Sum(nil))
	return answer, nil
}

func (c *Client) interpret(method string, ref Reference, answer reply) error {
	status := fmt.Errorf("%s %s: %s", method, ref.APIHost(), answer.text)
	switch {
	case answer.status == http.StatusUnauthorized || answer.status == http.StatusForbidden:
		return &Error{Reason: ReasonUnauthorized, Err: status}
	case answer.status == http.StatusNotFound:
		return &Error{Reason: ReasonNotFound, Err: status}
	case answer.status != http.StatusOK:
		return &Error{Reason: ReasonUnreachable, Err: status}
	}
	return nil
}
