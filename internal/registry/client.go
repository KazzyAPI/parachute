package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const (
	ReasonUnreachable  = "registry_unreachable"
	ReasonUnauthorized = "registry_unauthorized"
	ReasonNotFound     = "not_found"
	maxManifestBytes   = 4 << 20
	maxBodyBytes       = 1 << 20
	maxTagListPages    = 64
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
	head, err := c.request(ctx, http.MethodHead, ref, ref.ManifestURL(), credential)
	if err == nil && c.digest.MatchString(head.digest) {
		return head.digest, nil
	}
	if err == nil {
		err = &Error{Reason: ReasonUnreachable, Err: fmt.Errorf("HEAD %s: missing or invalid Docker-Content-Digest header", ref.APIHost())}
		c.head.HeadFailed(ctx, ref, err)
		return "", err
	}
	c.head.HeadFailed(ctx, ref, err)
	get, err := c.request(ctx, http.MethodGet, ref, ref.ManifestURL(), credential)
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
	content   []byte
	link      string
}

func (c *Client) request(ctx context.Context, method string, ref Reference, target string, credential *Credential) (reply, error) {
	answer, err := c.exchange(ctx, method, ref, target, "", maxManifestBytes)
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
		if answer, err = c.exchange(ctx, method, ref, target, authorization, maxManifestBytes); err != nil {
			return reply{}, err
		}
	}
	return answer, c.interpret(method, ref, answer)
}

func (c *Client) exchange(ctx context.Context, method string, ref Reference, target, authorization string, readLimit int) (answer reply, err error) {
	request, err := http.NewRequestWithContext(ctx, method, target, nil)
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
	answer = reply{
		status:    response.StatusCode,
		text:      response.Status,
		challenge: response.Header.Get("WWW-Authenticate"),
		digest:    response.Header.Get("Docker-Content-Digest"),
		link:      response.Header.Get("Link"),
	}
	if method != http.MethodGet || answer.status != http.StatusOK || readLimit <= 0 {
		return answer, nil
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, int64(readLimit)+1))
	if err != nil {
		return reply{}, &Error{Reason: ReasonUnreachable, Err: fmt.Errorf("%s %s: %w", method, ref.APIHost(), err)}
	}
	if len(content) > readLimit {
		return reply{}, &Error{Reason: ReasonUnreachable, Err: fmt.Errorf("%s %s: response larger than %d bytes", method, ref.APIHost(), readLimit)}
	}
	answer.content = content
	if readLimit == maxManifestBytes {
		sum := sha256.Sum256(content)
		answer.body = "sha256:" + hex.EncodeToString(sum[:])
	}
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

func (c *Client) Tags(ctx context.Context, ref Reference, credential *Credential) ([]string, error) {
	target := ref.TagsURL()
	seen := map[string]struct{}{}
	var merged []string
	for page := 0; page < maxTagListPages; page++ {
		if _, again := seen[target]; again {
			return merged, nil
		}
		seen[target] = struct{}{}
		answer, err := c.tagListPage(ctx, ref, target, credential)
		if err != nil {
			return nil, err
		}
		var list struct {
			Tags []string `json:"tags"`
		}
		if err := json.Unmarshal(answer.content, &list); err != nil {
			return nil, &Error{Reason: ReasonUnreachable, Err: fmt.Errorf("GET %s: invalid tag list: %w", ref.APIHost(), err)}
		}
		merged = append(merged, list.Tags...)
		next, err := tagListNextURL(answer.link, target)
		if err != nil {
			return nil, &Error{Reason: ReasonUnreachable, Err: fmt.Errorf("GET %s: %w", ref.APIHost(), err)}
		}
		if next == "" {
			return merged, nil
		}
		target = next
	}
	return nil, &Error{Reason: ReasonUnreachable, Err: fmt.Errorf("GET %s: tag list exceeded %d pages", ref.APIHost(), maxTagListPages)}
}

func (c *Client) tagListPage(ctx context.Context, ref Reference, target string, credential *Credential) (reply, error) {
	answer, err := c.exchange(ctx, http.MethodGet, ref, target, "", maxBodyBytes)
	if err != nil {
		return reply{}, err
	}
	if answer.status == http.StatusUnauthorized {
		authorization, err := c.auth.Authorization(ctx, c.auth.Parse(answer.challenge), credential)
		if err != nil {
			return reply{}, err
		}
		if authorization == "" {
			return reply{}, &Error{Reason: ReasonUnauthorized, Err: fmt.Errorf("GET %s: authentication required", ref.APIHost())}
		}
		if answer, err = c.exchange(ctx, http.MethodGet, ref, target, authorization, maxBodyBytes); err != nil {
			return reply{}, err
		}
	}
	return answer, c.interpret(http.MethodGet, ref, answer)
}

func tagListNextURL(linkHeader, base string) (string, error) {
	if linkHeader == "" {
		return "", nil
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	for _, part := range strings.Split(linkHeader, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) && !strings.Contains(part, `rel=next`) {
			continue
		}
		start, end := strings.Index(part, "<"), strings.Index(part, ">")
		if start < 0 || end <= start {
			continue
		}
		next, err := url.Parse(strings.TrimSpace(part[start+1 : end]))
		if err != nil {
			return "", err
		}
		return baseURL.ResolveReference(next).String(), nil
	}
	return "", nil
}
