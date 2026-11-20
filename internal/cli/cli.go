package cli

import (
	"bufio"
	"errors"
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

// Version is set by release builds using -ldflags.
var Version = "dev"

func reportCommandError(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "Error: %v\n", err)
	if errors.Is(err, manager.ErrContributionPartial) {
		return 3
	}
	return 1
}

func Run(arguments []string, stdout, stderr io.Writer) int {
	return RunWithIO(arguments, os.Stdin, stdout, stderr)
}

func RunWithIO(arguments []string, stdin io.Reader, stdout, stderr io.Writer) int {
	options, args, err := parseArguments(arguments, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if len(args) == 0 {
		args = []string{"tui"}
	}
	for _, arg := range args[1:] {
		if arg == "--all" && args[0] != "update" && args[0] != "contribute" {
			fmt.Fprintln(stderr, "Error: --all is only valid for update or contribute")
			return 1
		}
		if arg == "--discard-local" && args[0] != "update" {
			fmt.Fprintln(stderr, "Error: --discard-local is only valid for update")
			return 1
		}
	}
	if args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
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
		StateDir:   filepath.Join(options.home, ".agents", ".skills-manager", options.namespace),
		Out:        stdout,
	}

	command := args[0]
	commandArgs := args[1:]
	for _, arg := range commandArgs {
		if arg == "--discard-local" {
			var discardErr error
			scopes := []sourceManager{{name: options.namespace, Manager: mgr}}
			if len(options.sources) > 0 {
				scopes, discardErr = configuredManagers(options, workingDir, stdout)
			}
			if discardErr == nil {
				discardErr = runDiscard(scopes, options.from != "", commandArgs, stdin, stdout)
			}
			if discardErr != nil {
				fmt.Fprintf(stderr, "Error: %v\n", discardErr)
				return 1
			}
			return 0
		}
	}
	if command == "update" {
		for _, arg := range commandArgs {
			if arg == "--all" {
				if len(commandArgs) != 1 {
					fmt.Fprintln(stderr, "Error: --all cannot be combined with a skill name or other update options")
					return 1
				}
				commandArgs = nil
				break
			}
		}
	}
	if len(options.sources) > 0 && command != "init" && command != "config" && command != "source" && command != "validate" && command != "version" {
		if err := runSources(options, command, commandArgs, workingDir, stdin, stdout); err != nil {
			return reportCommandError(stderr, err)
		}
		return 0
	}
	var commandErr error
	switch command {
	case "source":
		commandErr = runSource(options, commandArgs, stdout)
	case "init":
		commandErr = noExtraArgs(commandArgs, func() error { return initialize(options, stdout) })
	case "config":
		if len(commandArgs) != 3 || commandArgs[0] != "set" || commandArgs[1] != "repo" {
			commandErr = fmt.Errorf("usage: skills-manager config set repo PATH")
		} else {
			commandErr = setSourceRepo(options.home, commandArgs[2], options.from, stdout)
		}
	case "status":
		commandErr = oneOptional(commandArgs, false, mgr.Status)
	case "contribute":
		commandErr = runContribute(commandArgs, options.repo, stdin, stdout, mgr)
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
		local := false
		var names []string
		for _, arg := range commandArgs {
			if arg == "--local" {
				local = true
			} else {
				names = append(names, arg)
			}
		}
		if local {
			commandErr = oneOptional(names, false, mgr.DiffLocal)
		} else {
			commandErr = oneOptional(names, false, mgr.Diff)
		}
	case "doctor":
		commandErr = noExtraArgs(commandArgs, mgr.Doctor)
	case "validate":
		commandErr = oneOptional(commandArgs, false, mgr.Validate)
	case "version":
		fmt.Fprintln(stdout, "skills-manager", Version)
		return 0
	default:
		fmt.Fprintf(stderr, "Unknown command %q.\n\n", command)
		usage(stderr)
		return 2
	}
	if commandErr != nil {
		return reportCommandError(stderr, commandErr)
	}
	return 0
}

type options struct {
	source            string
	home              string
	cache             string
	namespace         string
	repo              string
	from              string
	repoOverride      string
	sourceOverride    bool
	namespaceOverride bool
	sources           map[string]sourceConfiguration
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
		if argument == "--source" || argument == "--home" || argument == "--cache" || argument == "--namespace" || argument == "--from" || argument == "--repo" {
			if index+1 >= len(arguments) {
				return options{}, nil, fmt.Errorf("%s requires a value", argument)
			}
			flags = append(flags, argument, arguments[index+1])
			index++
			continue
		}
		if strings.HasPrefix(argument, "--source=") || strings.HasPrefix(argument, "--home=") || strings.HasPrefix(argument, "--cache=") || strings.HasPrefix(argument, "--namespace=") || strings.HasPrefix(argument, "--from=") || strings.HasPrefix(argument, "--repo=") {
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
	set.StringVar(&defaults.from, "from", "", "configured source name")
	set.StringVar(&defaults.repoOverride, "repo", "", "contribution checkout override")
	if err := set.Parse(flags); err != nil {
		return options{}, nil, err
	}
	defaults.sourceOverride = defaults.source != ""
	defaults.namespaceOverride = defaults.namespace != ""
	saved, err := readConfiguration(defaults.home)
	if err != nil {
		return options{}, nil, err
	}
	defaults.sources = saved.Sources
	defaults.repo = saved.Repo
	if defaults.repoOverride != "" {
		defaults.repo = defaults.repoOverride
	}
	if defaults.from != "" {
		entry, exists := saved.Sources[defaults.from]
		if !exists {
			return options{}, nil, fmt.Errorf("unknown source %q", defaults.from)
		}
		saved.Source, saved.Namespace, saved.Repo = entry.Source, entry.Namespace, entry.Repo
		defaults.repo = entry.Repo
		if defaults.repoOverride != "" {
			defaults.repo = defaults.repoOverride
		}
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
	catalog, installed, updates, states, err := mgr.InstallOptionsWithStatus()
	if err != nil {
		return err
	}
	selected, err := tui.RunInstallerWithStatus(stdin, stdout, catalog.Skills, catalog.Sets, installed, updates, states)
	if err != nil {
		return fmt.Errorf("run installer: %w", err)
	}
	if len(selected.Discards) > 0 {
		return applyDiscardSelection([]sourceManager{{Manager: mgr}}, []tui.InstallerSelection{selected}, stdin, stdout)
	}
	if err := mgr.Preflight(selected.Installs, selected.Updates, selected.Removals); err != nil {
		return err
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

func runContribute(args []string, repo string, stdin io.Reader, stdout io.Writer, mgr manager.Manager) error {
	var names []string
	yes := false
	all := false
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--all":
			all = true
		case args[i] == "--yes":
			yes = true
		case args[i] == "--repo":
			if i+1 >= len(args) {
				return fmt.Errorf("--repo requires a path")
			}
			i++
			repo = args[i]
		case strings.HasPrefix(args[i], "--repo="):
			repo = strings.TrimPrefix(args[i], "--repo=")
		case strings.HasPrefix(args[i], "-"):
			return fmt.Errorf("unknown contribute option %s", args[i])
		default:
			names = append(names, args[i])
		}
	}
	confirm := func() (bool, error) {
		if yes {
			return true, nil
		}
		fmt.Fprint(stdout, "Copy these changes to the checkout? [y/N] ")
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && err != io.EOF {
			return false, err
		}
		line = strings.ToLower(strings.TrimSpace(line))
		return line == "y" || line == "yes", nil
	}
	if all {
		if len(names) != 0 {
			return fmt.Errorf("--all cannot be combined with a skill name")
		}
		return mgr.ContributeAll(repo, confirm)
	}
	return oneOptional(names, true, func(name string) error {
		return mgr.Contribute(name, repo, confirm)
	})
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func usage(out io.Writer) {
	fmt.Fprint(out, `Usage: skills-manager [command] [options]
Run without a command to open the interactive installer.

Manage skills:
  available [query]             Browse or search the catalog
  sets                          List skill sets
  list                          List installed skills
  tui                           Open the interactive installer
  install [name|@set]...        Install skills or sets; omit names for the TUI
  uninstall <name>              Remove an installed skill
  status [name]                 Show local and upstream changes
  diff [name] [--local]         Preview upstream changes or your local edits
  update [name|--all]           Update without overwriting local edits
  update <name|--all> --discard-local [--yes]
                                Overwrite local edits without a backup

Contribute per file (preview first; checkout edits and untracked ignored/cache files stay put).
  Conflicts are skipped; exit 3 means review unresolved paths/packages. Nothing is staged.
  skills-manager config set repo ~/dev/team-skills --from default
  skills-manager contribute default/code-review
  skills-manager contribute --all --from default
  --repo PATH overrides the saved checkout; --yes skips confirmation.

Sources:
  init --source <url-or-path>   Configure a source
  source add NAME --source <url-or-path> [--namespace NAME] [--repo PATH]
  source list                   List configured sources
  source remove NAME            Forget a source; keep installed skills

Other commands:
  doctor                        Check sources and installations
  validate [path]               Validate a skill or catalog
  version                       Show the CLI version

Options:
  --from NAME                   Limit the command to one configured source
  --source URL/PATH             Override the Git source
  --namespace NAME              Select the install directory under ~/.agents/skills/
  --home PATH, --cache PATH     Override the home or cache directory
  --help                        Show this help
`)
}
