package config

import (
	"errors"
	"fmt"
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

func (s *Syntax) Parse(setting Setting, raw string) (Value, error) {
	value := Value{Raw: raw}
	var err error
	switch setting.Kind {
	case KindBool:
		value.Bool, err = s.parseBool(raw)
	case KindPositiveInt, KindNonNegativeInt:
		value.Int, err = s.parseInt(raw)
		if err == nil && (setting.Kind == KindPositiveInt && value.Int <= 0 || setting.Max > 0 && value.Int > setting.Max) {
			err = errSyntax
		}
	case KindDecimal, KindPositiveDecimal:
		value.Decimal, err = s.parseDecimal(raw)
		if err == nil && (setting.Kind == KindPositiveDecimal && value.Decimal <= 0 || setting.Max > 0 && value.Decimal > float64(setting.Max)) {
			err = errSyntax
		}
	case KindDuration, KindPositiveDuration:
		value.Duration, err = time.ParseDuration(raw)
		if err == nil && setting.Kind == KindPositiveDuration && value.Duration <= 0 {
			err = errSyntax
		}
	case KindEnum:
		if raw != setting.Default && !slices.Contains(setting.Choices, raw) {
			err = errSyntax
		}
	case KindList:
		value.List = s.List(raw)
	case KindString:
	}
	if err != nil {
		return Value{}, fmt.Errorf("invalid value %q for %s: %w", raw, setting.Label(), err)
	}
	return value, nil
}

func (s *Syntax) parseBool(raw string) (bool, error) {
	switch strings.ToLower(raw) {
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	}
	return false, errSyntax
}

func (s *Syntax) parseInt(raw string) (int, error) {
	if !s.integer.MatchString(raw) {
		return 0, errSyntax
	}
	return strconv.Atoi(raw)
}

func (s *Syntax) parseDecimal(raw string) (float64, error) {
	if !s.decimal.MatchString(raw) {
		return 0, errSyntax
	}
	return strconv.ParseFloat(raw, 64)
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
