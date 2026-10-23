package registry

import (
	"errors"
	"fmt"
	"net/http"
)

const maxRedirects = 10

func CheckRedirect(request *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return errors.New("too many redirects")
	}
	if request.URL.Scheme != "https" && !isLoopback(request.URL.Host) {
		return fmt.Errorf("refusing redirect to plain HTTP host %s", request.URL.Host)
	}
	if request.URL.Host != via[0].URL.Host {
		request.Header.Del("Authorization")
	}
	return nil
}
