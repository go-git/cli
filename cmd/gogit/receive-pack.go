package main

import (
	"os"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/transport"
	"github.com/spf13/cobra"
)

var (
	receivePackStatelessRPC  bool
	receivePackAdvertiseRefs bool
)

func init() {
	receivePackCmd.Flags().BoolVarP(&receivePackStatelessRPC, "stateless-rpc", "", false, "Use stateless RPC")
	receivePackCmd.Flags().BoolVarP(&receivePackAdvertiseRefs, "advertise-refs", "", false, "Advertise refs")

	rootCmd.AddCommand(receivePackCmd)
}

var receivePackCmd = &cobra.Command{
	Use:   "receive-pack <directory>",
	Short: "Run the receive-pack service",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := git.PlainOpen(args[0])
		if err != nil {
			return err
		}

		return transport.ReceivePack(
			cmd.Context(),
			r.Storer,
			os.Stdin,
			os.Stdout,
			&transport.ReceivePackOptions{
				GitProtocol:   os.Getenv("GIT_PROTOCOL"),
				StatelessRPC:  receivePackStatelessRPC,
				AdvertiseRefs: receivePackAdvertiseRefs,
			},
		)
	},
}
