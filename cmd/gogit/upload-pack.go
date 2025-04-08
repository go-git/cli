package main

import (
	"context"
	"os"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/transport"
	"github.com/spf13/cobra"
)

var (
	uploadPackStatelessRPC  bool
	uploadPackAdvertiseRefs bool
	uploadPackTimeout       int
)

func init() {
	uploadPackCmd.Flags().BoolVarP(&uploadPackStatelessRPC, "stateless-rpc", "", false, "Use stateless RPC")
	uploadPackCmd.Flags().BoolVarP(&uploadPackAdvertiseRefs, "advertise-refs", "", false, "Advertise refs")
	uploadPackCmd.Flags().IntVarP(&uploadPackTimeout, "timeout", "", 0, "Timeout in seconds")

	rootCmd.AddCommand(uploadPackCmd)
}

var uploadPackCmd = &cobra.Command{
	Use:   "upload-pack [options] <directory>",
	Short: "Run the upload-pack service",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := git.PlainOpen(args[0])
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		if uploadPackTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, time.Duration(uploadPackTimeout)*time.Second)
			defer cancel()
		}

		return transport.UploadPack(
			ctx,
			r.Storer,
			os.Stdin,
			os.Stdout,
			&transport.UploadPackOptions{
				GitProtocol:   os.Getenv("GIT_PROTOCOL"),
				StatelessRPC:  uploadPackStatelessRPC,
				AdvertiseRefs: uploadPackAdvertiseRefs,
			},
		)
	},
}
