package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/adwise/developer-skills-manager/internal/manager"
	"github.com/adwise/developer-skills-manager/internal/skill"
	"github.com/adwise/developer-skills-manager/internal/source"
	"github.com/adwise/developer-skills-manager/internal/tui"
)

func Run(arguments []string, stdout, stderr io.Writer) int {
	return RunWithIO(arguments, os.Stdin, stdout, stderr)
}

func RunWithIO(arguments []string, stdin io.Reader, stdout, stderr io.Writer) int {
	options, args, err := parseArguments(arguments, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		usage(stdout)
		return 0
	}

	workingDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "Unable to determine the current directory: %v\n", err)
		return 1
	}
	mgr := manager.Manager{
		Repository: source.Repository{Source: options.source, CacheDir: options.cache},
		GlobalDir:  filepath.Join(options.home, ".agents", "skills", options.namespace),
		LocalDir:   filepath.Join(workingDir, ".agents", "skills"),
		Out:        stdout,
	}

	command := args[0]
	commandArgs := args[1:]
	var commandErr error
	switch command {
	case "init":
		commandErr = noExtraArgs(commandArgs, func() error { return initialize(options, stdout) })
	case "available", "search":
		commandErr = mgr.Available(strings.Join(commandArgs, " "))
	case "list":
		commandErr = noExtraArgs(commandArgs, mgr.List)
	case "sets":
		commandErr = noExtraArgs(commandArgs, mgr.Sets)
	case "install":
		if len(commandArgs) == 0 {
			commandErr = runInstaller(stdin, stdout, mgr)
		} else {
			commandErr = mgr.InstallMany(commandArgs)
		}
	case "tui":
		commandErr = noExtraArgs(commandArgs, func() error { return runInstaller(stdin, stdout, mgr) })
	case "uninstall", "remove":
		commandErr = oneOptional(commandArgs, true, mgr.Uninstall)
	case "update":
		commandErr = oneOptional(commandArgs, false, mgr.Update)
	case "diff":
		commandErr = oneOptional(commandArgs, false, mgr.Diff)
	case "doctor":
		commandErr = noExtraArgs(commandArgs, mgr.Doctor)
	case "validate":
		commandErr = oneOptional(commandArgs, false, mgr.Validate)
	case "version":
		fmt.Fprintln(stdout, "skills-manager dev")
		return 0
	default:
		fmt.Fprintf(stderr, "Unknown command %q.\n\n", command)
		usage(stderr)
		return 2
	}
	if commandErr != nil {
		fmt.Fprintf(stderr, "Error: %v\n", commandErr)
		return 1
	}
	return 0
}

type options struct {
	source    string
	home      string
	cache     string
	namespace string
}

func parseArguments(arguments []string, stderr io.Writer) (options, []string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return options{}, nil, fmt.Errorf("determine user home: %w", err)
	}
	cacheBase, err := os.UserCacheDir()
	if err != nil {
		cacheBase = filepath.Join(home, ".cache")
	}
	defaults := options{
		source:    envOr("SKILLS_MANAGER_SOURCE", ""),
		home:      envOr("SKILLS_MANAGER_HOME", home),
		cache:     envOr("SKILLS_MANAGER_CACHE", filepath.Join(cacheBase, "skills-manager", "repository")),
		namespace: envOr("SKILLS_MANAGER_NAMESPACE", ""),
	}

	// Global options are accepted before or after the command for a friendly CLI.
	var flags []string
	var positional []string
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--source" || argument == "--home" || argument == "--cache" || argument == "--namespace" {
			if index+1 >= len(arguments) {
				return options{}, nil, fmt.Errorf("%s requires a value", argument)
			}
			flags = append(flags, argument, arguments[index+1])
			index++
			continue
		}
		if strings.HasPrefix(argument, "--source=") || strings.HasPrefix(argument, "--home=") || strings.HasPrefix(argument, "--cache=") || strings.HasPrefix(argument, "--namespace=") {
			flags = append(flags, argument)
			continue
		}
		positional = append(positional, argument)
	}

	set := flag.NewFlagSet("skills-manager", flag.ContinueOnError)
	set.SetOutput(stderr)
	set.StringVar(&defaults.source, "source", defaults.source, "Git URL or local central repository")
	set.StringVar(&defaults.home, "home", defaults.home, "user home containing .agents/skills and configuration")
	set.StringVar(&defaults.cache, "cache", defaults.cache, "remote repository cache")
	set.StringVar(&defaults.namespace, "namespace", defaults.namespace, "installation namespace (default: skills-manager)")
	if err := set.Parse(flags); err != nil {
		return options{}, nil, err
	}
	saved, err := readConfiguration(defaults.home)
	if err != nil {
		return options{}, nil, err
	}
	if defaults.source == "" {
		defaults.source = saved.Source
	}
	if defaults.namespace == "" {
		defaults.namespace = saved.Namespace
	}
	if defaults.namespace == "" {
		defaults.namespace = "skills-manager"
	}
	if !skill.IsValidName(defaults.namespace) {
		return options{}, nil, fmt.Errorf("invalid namespace %q; use lowercase letters, digits, and hyphens", defaults.namespace)
	}
	return defaults, positional, nil
}

func noExtraArgs(args []string, fn func() error) error {
	if len(args) > 0 {
		return fmt.Errorf("this command does not accept arguments")
	}
	return fn()
}

func oneOptional(args []string, required bool, fn func(string) error) error {
	if len(args) > 1 {
		return fmt.Errorf("this command accepts at most one argument")
	}
	if required && len(args) == 0 {
		return fmt.Errorf("a skill name is required")
	}
	value := ""
	if len(args) == 1 {
		value = args[0]
	}
	return fn(value)
}

func runInstaller(stdin io.Reader, stdout io.Writer, mgr manager.Manager) error {
	catalog, installed, updates, err := mgr.InstallOptionsWithUpdates()
	if err != nil {
		return err
	}
	selected, err := tui.RunInstallerWithUpdates(stdin, stdout, catalog.Skills, catalog.Sets, installed, updates)
	if err != nil {
		return fmt.Errorf("run installer: %w", err)
	}
	if len(selected.Removals) > 0 {
		if err := mgr.UninstallMany(selected.Removals); err != nil {
			return err
		}
	}
	if len(selected.Installs) > 0 {
		if err := mgr.InstallMany(selected.Installs); err != nil {
			return err
		}
	}
	if len(selected.Updates) > 0 {
		return mgr.UpdateMany(selected.Updates)
	}
	return nil
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func usage(out io.Writer) {
	fmt.Fprint(out, `Skills Manager installs agent skills from your Git repository as plain files.

Usage:
  skills-manager init --source <url-or-path>  Save your source repository
  skills-manager available [query]          List or search available skills
  skills-manager sets                       List skill sets
  skills-manager list                       List global and repository skills
  skills-manager install                    Open the interactive installer
  skills-manager install <name|@set>...      Install skills or skill sets
  skills-manager tui                        Open the interactive installer
  skills-manager uninstall <name>           Remove a global skill
  skills-manager update [name]              Update global skills
  skills-manager diff [name]                Review changes before updating
  skills-manager doctor                     Check sources and installations
  skills-manager validate [path]            Validate skills or a repository
  skills-manager version                    Show the CLI version

Options (before or after the command):
  --source <url-or-path>  Source Git repository or local checkout
  --namespace <name>     Directory under ~/.agents/skills (default: skills-manager)
  --home <path>          Override the user home and configuration location
  --cache <path>         Override the Git cache directory

Configuration: ~/.config/skills-manager/config.json
Precedence: command-line options > environment > saved configuration > defaults.
Environment:
  SKILLS_MANAGER_SOURCE, SKILLS_MANAGER_NAMESPACE, SKILLS_MANAGER_HOME, SKILLS_MANAGER_CACHE
`)
}
