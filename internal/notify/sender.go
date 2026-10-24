package notify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"strings"

	"github.com/nicholas-fedor/shoutrrr"
	"github.com/nicholas-fedor/shoutrrr/pkg/types"
)

const loggerScheme = "logger://"

type Redactor interface {
	Redact(text string) string
}

type ShoutrrrSender struct {
	urls     []string
	logger   *slog.Logger
	redactor Redactor
	stdout   io.Writer
}

func NewShoutrrrSender(urls []string, logger *slog.Logger, redactor Redactor, stdout io.Writer) *ShoutrrrSender {
	return &ShoutrrrSender{urls: urls, logger: logger, redactor: redactor, stdout: stdout}
}

func ValidateURL(rawURL string) error {
	if strings.HasPrefix(rawURL, loggerScheme) {
		return nil
	}
	if _, err := shoutrrr.CreateSenderWithOptions(types.SenderOptions{}, rawURL); err != nil {
		return errors.New("invalid --notification-url: the value is not a valid service URL")
	}
	return nil
}

func (s *ShoutrrrSender) Send(ctx context.Context, title, body string) {
	title, body = s.redactor.Redact(title), s.redactor.Redact(body)
	params := types.Params{}
	if title != "" {
		params[types.TitleKey] = title
	}
	for _, rawURL := range s.urls {
		if strings.HasPrefix(rawURL, loggerScheme) {
			s.log(title, body)
			continue
		}
		if err := shoutrrr.SendContext(ctx, rawURL, body, &params); err != nil {
			s.logFailure(rawURL, err)
		}
	}
}

func (s *ShoutrrrSender) log(title, body string) {
	if s.stdout == nil {
		s.logger.Info("notification", "title", title, "body", body)
		return
	}
	if _, err := io.WriteString(s.stdout, strings.TrimSuffix(body, "\n")+"\n"); err != nil {
		s.logger.Error("cannot write notification to stdout", "error", err.Error())
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
