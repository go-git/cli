package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v6/plumbing/client"
	"github.com/go-git/go-git/v6/plumbing/transport"
	"github.com/go-git/go-git/v6/plumbing/transport/ssh"
	"github.com/go-git/go-git/v6/utils/trace"
	"github.com/spf13/cobra"
	gossh "golang.org/x/crypto/ssh"
)

var rootCmd = &cobra.Command{
	Use:     "gogit [<args>] <command>",
	Short:   "gogit is a Git CLI that uses go-git as its backend.",
	Version: "0.1.0-gogit",
	RunE: func(_ *cobra.Command, _ []string) error {
		// Real git exits 1 when invoked with no subcommand. test-lib.sh
		// relies on this to detect a working git binary. We deliberately
		// omit printing usage so summary-mode harness output stays clean
		// during test-lib's sanity checks.
		os.Exit(1)

		return nil
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

// envToTarget maps what environment variables can be used
// to enable specific trace targets.
var envToTarget = map[string]trace.Target{
	"GIT_TRACE":             trace.General,
	"GIT_TRACE_PACKET":      trace.Packet,
	"GIT_TRACE_SSH":         trace.SSH,
	"GIT_TRACE_PERFORMANCE": trace.Performance,
}

func init() {
	// Set up tracing
	var target trace.Target

	for k, v := range envToTarget {
		if ok, _ := strconv.ParseBool(os.Getenv(k)); ok {
			target |= v
		}
	}

	trace.SetTarget(target)
}

func init() {
	rootCmd.PersistentFlags().StringArrayVarP(&configOverridesRaw, "config", "c", nil,
		"Override a configuration value for the duration of this command (key=value, may be repeated)")
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if cmd == configCmd || cmd.Parent() == configCmd {
			if err := validateNoSystem(); err != nil {
				return err
			}
		}

		return applyConfigOverridesFromFlags()
	}
}

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--exec-path" || strings.HasPrefix(arg, "--exec-path=") {
			exe, err := os.Executable()
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}

			fmt.Println(filepath.Dir(exe))

			return
		}
	}

	err := rootCmd.Execute()
	if err != nil {
		// Some commands need git's own exit statuses, and git stays
		// silent for several of them, so gitExitError carries both the
		// code and whether anything is printed.
		var gerr *gitExitError
		if errors.As(err, &gerr) {
			if gerr.msg != "" {
				fmt.Fprintln(os.Stderr, gerr.msg)
			}

			os.Exit(gerr.code)
		}

		var rerr *transport.RemoteError
		if errors.As(err, &rerr) {
			fmt.Fprintln(os.Stderr, rerr)
		} else {
			fmt.Fprintln(os.Stderr, err)
		}

		os.Exit(1)
	}
}

func defaultClientOptions(u *url.URL) []client.Option {
	if u == nil {
		return nil
	}

	switch u.Scheme {
	case "file", "git":
		// Do nothing.
	case "ssh":
		if u.User == nil {
			return nil
		}

		a, err := ssh.NewSSHAgentAuth(u.User.Username())
		if err != nil {
			return nil
		}

		switch u.Host {
		case "localhost", "127.0.0.1":
			// Ignore host key verification for localhost.
			a.HostKeyCallback = gossh.InsecureIgnoreHostKey()
		}

		return []client.Option{client.WithSSHAuth(a)}
	case "http", "https":
	}

	return nil
}
