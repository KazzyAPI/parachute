package config

import (
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var errSyntax = errors.New("invalid syntax")

type Syntax struct {
	decimal *regexp.Regexp
	integer *regexp.Regexp
	listSep *regexp.Regexp
}

func NewSyntax() *Syntax {
	return &Syntax{
		decimal: regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`),
		integer: regexp.MustCompile(`^[0-9]+$`),
		listSep: regexp.MustCompile(`[\s,]+`),
	}
}

func (s *Syntax) Validate(setting Setting, raw string) error {
	var err error
	switch setting.Kind {
	case KindBool:
		_, err = s.Bool(raw)
	case KindPositiveInt:
		var value int
		value, err = s.Int(raw)
		if err == nil && value <= 0 {
			err = errSyntax
		}
	case KindNonNegativeInt:
		_, err = s.Int(raw)
	case KindDecimal:
		_, err = s.Decimal(raw)
	case KindDuration:
		_, err = time.ParseDuration(raw)
	case KindEnum:
		if raw != setting.Default && !slices.Contains(setting.Choices, raw) {
			err = errSyntax
		}
	case KindString, KindList:
	}
	return err
}

func (s *Syntax) Bool(raw string) (bool, error) {
	switch strings.ToLower(raw) {
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	}
	return false, errSyntax
}

func (s *Syntax) Int(raw string) (int, error) {
	if !s.integer.MatchString(raw) {
		return 0, errSyntax
	}
	return strconv.Atoi(raw)
}

func (s *Syntax) Decimal(raw string) (float64, error) {
	if !s.decimal.MatchString(raw) {
		return 0, errSyntax
	}
	return strconv.ParseFloat(raw, 64)
}

func (s *Syntax) Duration(raw string) (time.Duration, error) {
	return time.ParseDuration(raw)
}

func (s *Syntax) List(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) >= 2 && (trimmed[0] == '"' || trimmed[0] == '\'') && trimmed[len(trimmed)-1] == trimmed[0] {
		trimmed = trimmed[1 : len(trimmed)-1]
	}
	var items []string
	for _, item := range s.listSep.Split(trimmed, -1) {
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}
