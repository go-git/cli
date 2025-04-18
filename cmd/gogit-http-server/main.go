package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"github.com/go-git/go-billy/v5/osfs"
	githttp "github.com/go-git/go-git/v6/backend/http"
	"github.com/go-git/go-git/v6/plumbing/transport"
	"github.com/spf13/cobra"
)

var (
	port   int
	prefix string
)

func init() {
	rootCmd.Flags().IntVarP(&port, "port", "p", 8080, "Port to run the HTTP server on")
	rootCmd.Flags().StringVarP(&prefix, "prefix", "", "", "Prefix for the HTTP server routes")
}

var rootCmd = &cobra.Command{
	Use:   "gogit-http-server [options] <directory>",
	Short: "Run a Go Git HTTP server",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		directory := args[0]
		addr := fmt.Sprintf(":%d", port)
		abs, err := filepath.Abs(directory)
		if err != nil {
			return fmt.Errorf("failed to get absolute path: %w", err)
		}

		log.Printf("Using absolute path: %q", abs)
		logger := log.Default()
		loader := transport.NewFilesystemLoader(osfs.New(abs, osfs.WithBoundOS()), false)
		gitmw := githttp.NewBackend(loader, &githttp.BackendOptions{
			ErrorLog: logger,
			Prefix:   prefix,
		})

		handler := LoggingMiddleware(logger, gitmw)
		log.Printf("Starting server on %q for directory %q", addr, directory)
		if err := http.ListenAndServe(addr, handler); !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
