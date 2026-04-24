package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"

	"github.com/go-git/go-git/v6/plumbing/client"
	"github.com/go-git/go-git/v6/plumbing/transport"
	"github.com/go-git/go-git/v6/plumbing/transport/ssh"
	"github.com/go-git/go-git/v6/utils/trace"
	"github.com/spf13/cobra"
	gossh "golang.org/x/crypto/ssh"
)

var rootCmd = &cobra.Command{
	Use:   "gogit [<args>] <command>",
	Short: "gogit is a Git CLI that uses go-git as its backend.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Usage()
	},
	DisableFlagsInUseLine: true,
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

func main() {
	err := rootCmd.Execute()
	if err != nil {
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
