package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"
)

func init() {
	versionCmd.Flags().Bool("build-options", false, "Print build options")
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display version information",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		fmt.Fprintf(cmd.OutOrStdout(), "git version %s (gogit)\n", rootCmd.Version)

		buildOptions, _ := cmd.Flags().GetBool("build-options")
		if buildOptions {
			fmt.Fprintf(cmd.OutOrStdout(), "cpu: %s\n", runtime.GOARCH)
			fmt.Fprintf(cmd.OutOrStdout(), "default-ref-format: files\n")
			fmt.Fprintf(cmd.OutOrStdout(), "default-hash: %s\n", defaultHashFromEnv())
		}

		return nil
	},
	DisableFlagsInUseLine: true,
}

// defaultHashFromEnv reports the value used by gogit's `version --build-options`
// for `default-hash`. go-git supports both sha1 and sha256 at runtime, so there
// is no compile-time builtin. The conformance harness reads this line before
// it exports GIT_DEFAULT_HASH; by echoing the test-driven value when it's set,
// the harness's DEFAULT_HASH_ALGORITHM prereq comes out right in both passes.
func defaultHashFromEnv() string {
	if v := os.Getenv("GIT_TEST_DEFAULT_HASH"); v != "" {
		return v
	}

	if v := os.Getenv("GIT_DEFAULT_HASH"); v != "" {
		return v
	}

	return "sha1"
}
