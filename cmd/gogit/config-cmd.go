package main

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"strings"

	gitconfig "github.com/go-git/cli/internal/plumbing/format/config"
	"github.com/spf13/cobra"
)

// Git's documented exit statuses for git-config.
const (
	exitNotFound      = 1   // the key was not found
	exitInvalidKey    = 1   // the section or key is invalid
	exitCannotWrite   = 4   // the requested config file cannot be written
	exitUnsetMissing  = 5   // unset of a key that does not exist
	exitCannotReplace = 5   // single value cannot replace several
	exitFatal         = 128 // the config file could not be read
	exitUsage         = 129 // the command invocation is malformed
	exitLockFailure   = 255 // the config lock could not be acquired
)

// configOpts holds the flags shared by `config`, `config get`, `config set`
// and `config unset`. Each command owns its own instance so the modern and
// legacy spellings stay independently parseable.
type configOpts struct {
	file    string
	fileSet bool
	local   bool
	global  bool
	system  bool

	all  bool
	path bool
}

var (
	legacyOpts configOpts
	getOpts    configOpts
	setOpts    configOpts
	unsetOpts  configOpts

	legacyGet        bool
	legacyGetAll     bool
	legacyAdd        bool
	legacyUnset      bool
	legacyUnsetAll   bool
	legacyReplaceAll bool
)

// registerLocation adds the flags that choose which configuration file to act
// on. They are mutually exclusive.
func (o *configOpts) registerLocation(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&o.file, "file", "f", "", "Use the given config file instead of the repository config")
	cmd.Flags().BoolVar(&o.local, "local", false, "Use the repository config file")
	cmd.Flags().BoolVar(&o.global, "global", false, "Use the global (per-user) config file")
	cmd.Flags().BoolVar(&o.system, "system", false, "Use the system-wide config file")
	cmd.MarkFlagsMutuallyExclusive("file", "local", "global", "system")
}

func init() {
	for _, cmd := range []*cobra.Command{configCmd, configGetCmd, configSetCmd, configUnsetCmd} {
		cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
			return configInvocationError(err)
		})
		cmd.Flags().SetInterspersed(false)
	}

	legacyOpts.registerLocation(configCmd)
	configCmd.Flags().BoolVar(&legacyOpts.path, "path", false,
		"Canonicalize the value as a path, expanding a leading ~")
	configCmd.Flags().BoolVar(&legacyGet, "get", false, "Get the value for the given key")
	configCmd.Flags().BoolVar(&legacyGetAll, "get-all", false, "Get all values for the given key")
	configCmd.Flags().BoolVar(&legacyAdd, "add", false, "Add a new value without altering existing ones")
	configCmd.Flags().BoolVar(&legacyUnset, "unset", false, "Remove the value for the given key")
	configCmd.Flags().BoolVar(&legacyUnsetAll, "unset-all", false, "Remove all occurrences of the key")
	configCmd.Flags().BoolVar(&legacyReplaceAll, "replace-all", false, "Replace all values for the given key")
	configCmd.MarkFlagsMutuallyExclusive("get", "get-all", "add", "unset", "unset-all", "replace-all")

	getOpts.registerLocation(configGetCmd)
	configGetCmd.Flags().BoolVar(&getOpts.all, "all", false, "Show all values for the key")
	configGetCmd.Flags().BoolVar(&getOpts.path, "path", false,
		"Canonicalize the value as a path, expanding a leading ~")

	setOpts.registerLocation(configSetCmd)
	configSetCmd.Flags().BoolVar(&setOpts.all, "all", false, "Replace all values for the key")

	unsetOpts.registerLocation(configUnsetCmd)
	configUnsetCmd.Flags().BoolVar(&unsetOpts.all, "all", false, "Remove all values for the key")

	configCmd.AddCommand(configGetCmd, configSetCmd, configUnsetCmd)
	rootCmd.AddCommand(configCmd)
}

var configCmd = &cobra.Command{
	Use:   "config [<options>] <key> [<value>]",
	Short: "Get or set repository configuration",
	Long: "Get or set configuration values.\n\n" +
		"The modern forms are `config get <key>`, `config set <key> <value>` and\n" +
		"`config unset <key>`. The legacy flag spellings (--get, --add, --unset-all\n" +
		"and a bare `config <key> [<value>]`) are also accepted.",
	Args:                  configArgs(cobra.RangeArgs(1, 2)),
	RunE:                  runConfigLegacy,
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

var configGetCmd = &cobra.Command{
	Use:   "get [<options>] <key>",
	Short: "Print the value of a configuration key",
	Args:  configArgs(cobra.ExactArgs(1)),
	RunE: func(cmd *cobra.Command, args []string) error {
		getOpts.fileSet = cmd.Flags().Changed("file")

		return runConfigGet(&getOpts, args[0])
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

var configSetCmd = &cobra.Command{
	Use:   "set [<options>] <key> <value>",
	Short: "Set the value of a configuration key",
	Args:  configArgs(cobra.ExactArgs(2)),
	RunE: func(cmd *cobra.Command, args []string) error {
		setOpts.fileSet = cmd.Flags().Changed("file")

		return runConfigWrite(&setOpts, args[0], args[1], writeSet)
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

var configUnsetCmd = &cobra.Command{
	Use:   "unset [<options>] <key>",
	Short: "Remove a configuration key",
	Args:  configArgs(cobra.ExactArgs(1)),
	RunE: func(cmd *cobra.Command, args []string) error {
		unsetOpts.fileSet = cmd.Flags().Changed("file")

		return runConfigWrite(&unsetOpts, args[0], "", writeUnset)
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

// runConfigLegacy dispatches the pre-subcommand spellings of git config.
func runConfigLegacy(cmd *cobra.Command, args []string) error {
	legacyOpts.fileSet = cmd.Flags().Changed("file")

	switch {
	case legacyGet, legacyGetAll:
		if len(args) != 1 {
			return usageError("--get takes exactly one key")
		}

		legacyOpts.all = legacyGetAll

		return runConfigGet(&legacyOpts, args[0])

	case legacyAdd:
		if len(args) != 2 {
			return usageError("--add takes a key and a value")
		}

		return runConfigWrite(&legacyOpts, args[0], args[1], writeAdd)

	case legacyReplaceAll:
		if len(args) != 2 {
			return usageError("--replace-all takes a key and a value")
		}

		legacyOpts.all = true

		return runConfigWrite(&legacyOpts, args[0], args[1], writeSet)

	case legacyUnset, legacyUnsetAll:
		if len(args) != 1 {
			return usageError("--unset takes exactly one key")
		}

		legacyOpts.all = legacyUnsetAll

		return runConfigWrite(&legacyOpts, args[0], "", writeUnset)

	case len(args) == 2:
		return runConfigWrite(&legacyOpts, args[0], args[1], writeSet)

	default:
		return runConfigGet(&legacyOpts, args[0])
	}
}

func runConfigGet(o *configOpts, rawKey string) error {
	key, err := parseConfigKey(rawKey)
	if err != nil {
		return err
	}

	sources, err := readSources(o)
	if err != nil {
		return err
	}

	values, err := effectiveConfigValues(sources, key)
	if err != nil {
		return err
	}

	if len(values) == 0 {
		// git prints nothing and exits 1 for a key that is not set. This is
		// distinct from a key whose value is the empty string, which still
		// prints one empty line and exits 0.
		return &gitExitError{code: exitNotFound}
	}

	if !o.all {
		values = values[len(values)-1:]
	}

	for _, v := range values {
		if o.path {
			if v, err = expandPath(v); err != nil {
				return err
			}
		}

		fmt.Println(v)
	}

	return nil
}

type writeMode int

const (
	writeSet writeMode = iota
	writeAdd
	writeUnset
)

func runConfigWrite(o *configOpts, rawKey, value string, mode writeMode) error {
	key, err := parseConfigKey(rawKey)
	if err != nil {
		return err
	}

	target, err := writeTarget(o)
	if err != nil {
		return err
	}

	err = gitconfig.UpdateFile(target.path, func(f *gitconfig.File) error {
		switch mode {
		case writeAdd:
			return f.Add(key, value)

		case writeUnset:
			if !o.all && len(f.Values(key)) > 1 {
				fmt.Fprintf(os.Stderr, "warning: %s has multiple values\n", rawKey)

				return &gitExitError{
					code: exitCannotReplace,
					msg:  fmt.Sprintf("error: cannot unset multiple values for %s; use --all", rawKey),
				}
			}

			n, err := f.UnsetAll(key)
			if err == nil && n == 0 {
				return &gitExitError{code: exitUnsetMissing}
			}

			return err

		case writeSet:
			if o.all {
				return f.ReplaceAll(key, value)
			}

			err := f.Set(key, value)
			if !errors.Is(err, gitconfig.ErrMultipleValues) {
				return err
			}

			fmt.Fprintf(os.Stderr, "warning: %s has multiple values\n", rawKey)

			return &gitExitError{
				code: exitCannotReplace,
				msg: fmt.Sprintf("error: cannot overwrite multiple values with a single value\n"+
					"       Use --add or --all to change %s.", rawKey),
			}
		}

		return nil
	})
	if err != nil {
		return configWriteError(target, err)
	}

	return nil
}

// parseConfigKey converts Git's key diagnostics into exit-1 errors.
func parseConfigKey(raw string) (gitconfig.Key, error) {
	key, err := gitconfig.ParseKey(raw)
	if err != nil {
		return gitconfig.Key{}, &gitExitError{code: exitInvalidKey, msg: "error: " + err.Error()}
	}

	return key, nil
}

func configReadError(cf configFile, err error) error {
	var perr *gitconfig.ParseError
	if errors.As(err, &perr) {
		return &gitExitError{
			code: exitFatal,
			msg:  fmt.Sprintf("fatal: bad config line %d in file %s", perr.Line, cf.display),
		}
	}

	return err
}

func configWriteError(cf configFile, err error) error {
	var lockErr *gitconfig.LockError
	if errors.As(err, &lockErr) {
		detail := lockErr.Err.Error()
		if errors.Is(lockErr.Err, os.ErrExist) {
			detail = "File exists"
		}

		return &gitExitError{
			code: exitLockFailure,
			msg:  fmt.Sprintf("error: could not lock config file %s: %s", cf.display, detail),
		}
	}

	return configReadError(cf, err)
}

func usageError(msg string) error {
	return &gitExitError{code: exitUsage, msg: "error: " + msg}
}

func configArgs(validate cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := validateNoSystem(); err != nil {
			return err
		}

		if err := validate(cmd, args); err != nil {
			return configInvocationError(err)
		}

		if err := cmd.ValidateFlagGroups(); err != nil {
			return configInvocationError(err)
		}

		return nil
	}
}

func configInvocationError(err error) error {
	if envErr := validateNoSystem(); envErr != nil {
		return envErr
	}

	return usageError(err.Error())
}

// expandPath applies --path canonicalization: a leading ~ becomes the user's
// home directory. Any other value is returned unchanged.
func expandPath(v string) (string, error) {
	if !strings.HasPrefix(v, "~") {
		return v, nil
	}

	if v == "~" || strings.HasPrefix(v, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}

		if v == "~" {
			return home, nil
		}

		return home + "/" + v[2:], nil
	}

	name, suffix, hasSlash := strings.Cut(v[1:], "/")

	account, err := user.Lookup(name)
	if err == nil && account.HomeDir != "" {
		if !hasSlash {
			return account.HomeDir, nil
		}

		return account.HomeDir + "/" + suffix, nil
	}

	return "", &gitExitError{
		code: exitFatal,
		msg:  fmt.Sprintf("fatal: failed to expand user dir in: '%s'", v),
	}
}
