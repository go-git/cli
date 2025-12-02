package main

import (
	"errors"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/transport"
	"github.com/go-git/go-git/v6/storage/filesystem"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(updateServerInfoCmd)
}

var updateServerInfoCmd = &cobra.Command{
	Use:   "update-server-info",
	Short: "Update the server info file",
	RunE: func(cmd *cobra.Command, _ []string) error {
		r, err := git.PlainOpen(".")
		if err != nil {
			return err
		}

		store, ok := r.Storer.(*filesystem.Storage)
		if !ok {
			return errors.New("storer does not implement filesystem.Storage")
		}

		return transport.UpdateServerInfo(r.Storer, store.Filesystem())
	},
}
