package config

import (
	"fmt"
	"slices"
	"strings"
)

type Arguments struct {
	Command     Command
	Positionals []string
	Flags       map[string][]string
}

type ArgumentParser struct {
	registry *Registry
	removed  *RemovedSettings
	suggest  *Suggester
	commands []Command
}

func NewArgumentParser(registry *Registry, removed *RemovedSettings, suggest *Suggester) *ArgumentParser {
	return &ArgumentParser{registry: registry, removed: removed, suggest: suggest, commands: []Command{CommandHealthcheck, CommandStatus, CommandRestoreVerify}}
}

func (p *ArgumentParser) Parse(args []string) (Arguments, error) {
	parsed := Arguments{Flags: map[string][]string{}}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			parsed.Positionals = append(parsed.Positionals, args[index+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "--") {
			if err := p.addPositional(&parsed, arg); err != nil {
				return Arguments{}, err
			}
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		if parsed.Command == CommandRestoreVerify && name == "host" {
			name = "managed-host"
		}
		setting, err := p.lookup(name)
		if err != nil {
			return Arguments{}, err
		}
		if !hasValue && setting.Kind == KindBool {
			value, hasValue = "true", true
		}
		if !hasValue {
			if index+1 >= len(args) {
				return Arguments{}, fmt.Errorf("%s needs a value", setting.Flag())
			}
			index++
			value = args[index]
		}
		if len(parsed.Flags[name]) > 0 && !setting.Repeatable && setting.Kind != KindList {
			return Arguments{}, fmt.Errorf("%s is given more than once", setting.Flag())
		}
		parsed.Flags[name] = append(parsed.Flags[name], value)
	}
	return parsed, nil
}

func (p *ArgumentParser) addPositional(parsed *Arguments, arg string) error {
	if parsed.Command == CommandDaemon && len(parsed.Positionals) == 0 {
		if replacement, removed := p.removed.Command(arg); removed {
			return fmt.Errorf("the %s command was removed: %s", arg, replacement)
		}
		if slices.Contains(p.commands, Command(arg)) {
			parsed.Command = Command(arg)
			return nil
		}
	}
	parsed.Positionals = append(parsed.Positionals, arg)
	return nil
}

func (p *ArgumentParser) lookup(name string) (Setting, error) {
	if replacement, removed := p.removed.Flag(name); removed {
		return Setting{}, fmt.Errorf("--%s was removed: %s", name, replacement)
	}
	setting, ok := p.registry.ByFlag(name)
	if !ok {
		return Setting{}, p.suggest.Unknown("--"+name, p.registry.FlagNames())
	}
	return setting, nil
}
