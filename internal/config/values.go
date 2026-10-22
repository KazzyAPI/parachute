package config

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

type Source int

const (
	SourceDefault Source = iota
	SourceProfile
	SourceEnv
	SourceFlag
)

type Value struct {
	Raw    string
	Source Source
}

type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	Stat(path string) (fs.FileInfo, error)
}

type Resolver struct {
	registry *Registry
	syntax   *Syntax
	files    FileSystem
}

func NewResolver(registry *Registry, syntax *Syntax, files FileSystem) *Resolver {
	return &Resolver{registry: registry, syntax: syntax, files: files}
}

func (r *Resolver) Resolve(args Arguments, env map[string]string) (map[string]Value, error) {
	profileSetting, _ := r.registry.ByFlag("profile")
	profile, err := r.resolveOne(profileSetting, args, env, ProfileSafe)
	if err != nil {
		return nil, err
	}
	values := map[string]Value{}
	for _, setting := range r.registry.Settings() {
		value, err := r.resolveOne(setting, args, env, profile.Raw)
		if err != nil {
			return nil, err
		}
		values[setting.Label()] = value
	}
	return values, nil
}

func (r *Resolver) resolveOne(setting Setting, args Arguments, env map[string]string, profile string) (Value, error) {
	value := r.pick(setting, args, env, profile)
	if setting.FromFile && value.Raw != "" {
		contents, err := r.fileContents(setting, value.Raw)
		if err != nil {
			return Value{}, err
		}
		value.Raw = contents
	}
	if err := r.syntax.Validate(setting, value.Raw); err != nil {
		return Value{}, fmt.Errorf("invalid value %q for %s", value.Raw, setting.Label())
	}
	return value, nil
}

func (r *Resolver) pick(setting Setting, args Arguments, env map[string]string, profile string) Value {
	if flagValues, ok := args.Flags[setting.Name]; ok && setting.Name != "" {
		return Value{Raw: strings.Join(flagValues, ","), Source: SourceFlag}
	}
	if raw, ok := env[setting.Label()]; ok && raw != "" {
		if setting.EnvPresence {
			raw = "true"
		}
		return Value{Raw: raw, Source: SourceEnv}
	}
	if profileDefault := setting.DefaultFor(profile); profileDefault != setting.Default {
		return Value{Raw: profileDefault, Source: SourceProfile}
	}
	return Value{Raw: setting.Default, Source: SourceDefault}
}

func (r *Resolver) fileContents(setting Setting, raw string) (string, error) {
	if !filepath.IsAbs(raw) {
		return raw, nil
	}
	if _, err := r.files.Stat(raw); errors.Is(err, fs.ErrNotExist) {
		return raw, nil
	}
	contents, err := r.files.ReadFile(raw)
	if err != nil {
		return "", fmt.Errorf("cannot read %s for %s: %w", raw, setting.Label(), err)
	}
	return strings.TrimSuffix(string(contents), "\n"), nil
}
