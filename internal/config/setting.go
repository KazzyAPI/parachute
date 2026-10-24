package config

import "strings"

type Kind int

const (
	KindString Kind = iota
	KindBool
	KindPositiveInt
	KindNonNegativeInt
	KindDecimal
	KindPositiveDecimal
	KindDuration
	KindList
	KindEnum
)

type Setting struct {
	Name              string
	Env               string
	Kind              Kind
	Default           string
	ConvenientDefault *string
	Choices           []string
	FromFile          bool
	EnvPresence       bool
	Repeatable        bool
	Unimplemented     string
}

func (s Setting) Flag() string {
	return "--" + s.Name
}

func (s Setting) EnvName() string {
	if s.Env != "" {
		return s.Env
	}
	return "PARACHUTE_" + strings.ToUpper(strings.ReplaceAll(s.Name, "-", "_"))
}

func (s Setting) Label() string {
	if s.Name != "" {
		return s.Name
	}
	return s.Env
}

func (s Setting) DefaultFor(profile string) string {
	if profile == ProfileConvenient && s.ConvenientDefault != nil {
		return *s.ConvenientDefault
	}
	return s.Default
}
