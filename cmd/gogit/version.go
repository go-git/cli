package main

import (
	"fmt"
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
			fmt.Fprintf(cmd.OutOrStdout(), "default-hash: sha1\n")
		}

		return nil
	},
	DisableFlagsInUseLine: true,
}
