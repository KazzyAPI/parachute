package config

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
)

const maxSecretFileBytes = 1 << 20

type Source int

const (
	SourceDefault Source = iota
	SourceProfile
	SourceEnv
	SourceFlag
)

type Value struct {
	Raw      string
	Source   Source
	Bool     bool
	Int      int
	Decimal  float64
	Duration time.Duration
	List     []string
}

type FileSystem interface {
	ReadRegularFile(path string, limit int64) ([]byte, error)
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
	raw, source := r.pick(setting, args, env, profile)
	if setting.FromFile && raw != "" {
		contents, err := r.fileContents(setting, raw)
		if err != nil {
			return Value{}, err
		}
		raw = contents
	}
	if source == SourceFlag && raw == "" && (setting.Kind == KindEnum || setting.Name == "schedule") {
		return Value{}, fmt.Errorf("invalid value %q for %s: %w", raw, setting.Label(), errSyntax)
	}
	value, err := r.syntax.Parse(setting, raw)
	if err != nil {
		return Value{}, err
	}
	value.Source = source
	return value, nil
}

func (r *Resolver) pick(setting Setting, args Arguments, env map[string]string, profile string) (string, Source) {
	if flagValues, ok := args.Flags[setting.Name]; ok {
		return strings.Join(flagValues, ","), SourceFlag
	}
	if raw, ok := env[setting.Label()]; ok && raw != "" {
		if setting.EnvPresence {
			raw = "true"
		}
		return raw, SourceEnv
	}
	if profileDefault := setting.DefaultFor(profile); profileDefault != setting.Default {
		return profileDefault, SourceProfile
	}
	return setting.Default, SourceDefault
}

func (r *Resolver) fileContents(setting Setting, raw string) (string, error) {
	if !filepath.IsAbs(raw) {
		return raw, nil
	}
	if _, err := r.files.Stat(raw); errors.Is(err, fs.ErrNotExist) {
		return raw, nil
	}
	contents, err := r.files.ReadRegularFile(raw, maxSecretFileBytes)
	if err != nil {
		return "", fmt.Errorf("cannot read %s for %s: %w", raw, setting.Label(), err)
	}
	return strings.TrimSuffix(string(contents), "\n"), nil
}
