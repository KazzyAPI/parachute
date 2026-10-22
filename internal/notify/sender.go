package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"

	"github.com/nicholas-fedor/shoutrrr"
	"github.com/nicholas-fedor/shoutrrr/pkg/types"
)

type ShoutrrrSender struct {
	urls   []string
	logger *slog.Logger
}

func NewShoutrrrSender(urls []string, logger *slog.Logger) *ShoutrrrSender {
	return &ShoutrrrSender{urls: urls, logger: logger}
}

func (s *ShoutrrrSender) Send(ctx context.Context, title, body string) {
	params := types.Params{}
	if title != "" {
		params[types.TitleKey] = title
	}
	for _, rawURL := range s.urls {
		if err := shoutrrr.SendContext(ctx, rawURL, body, &params); err != nil {
			s.logFailure(rawURL, err)
		}
	}
}

func (s *ShoutrrrSender) logFailure(rawURL string, err error) {
	attributes := []any{"scheme", "unknown", "host", "unknown"}
	if parsed, parseErr := url.Parse(rawURL); parseErr == nil {
		attributes = []any{"scheme", parsed.Scheme, "host", parsed.Host}
	}
	var networkErr *net.OpError
	if errors.As(err, &networkErr) {
		attributes = append(attributes, "error", networkErr.Error())
	} else {
		attributes = append(attributes, "error_type", fmt.Sprintf("%T", err))
	}
	s.logger.Error("notification delivery failed", attributes...)
}
