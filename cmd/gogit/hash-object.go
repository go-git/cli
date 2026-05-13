package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/spf13/cobra"
)

var (
	hashObjectWrite     bool
	hashObjectType      string
	hashObjectStdin     bool
	hashObjectLiterally bool
)

func init() {
	hashObjectCmd.Flags().BoolVarP(&hashObjectWrite, "write", "w", false, "Write the object to the object store")
	hashObjectCmd.Flags().StringVarP(&hashObjectType, "type", "t", "blob", "Object type (blob, tag, tree, commit)")
	hashObjectCmd.Flags().BoolVar(&hashObjectStdin, "stdin", false, "Read content from stdin")
	hashObjectCmd.Flags().BoolVar(&hashObjectLiterally, "literally", false, "Skip content validation")
	rootCmd.AddCommand(hashObjectCmd)
}

var hashObjectCmd = &cobra.Command{
	Use:   "hash-object [-w] [-t <type>] [--stdin] [--literally] [<file>]",
	Short: "Compute object ID and optionally create a blob from a file",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		defer r.Close()

		var content []byte

		switch {
		case hashObjectStdin:
			content, err = io.ReadAll(cmd.InOrStdin())
		case len(args) == 1:
			content, err = os.ReadFile(args[0])
		default:
			return errors.New("either --stdin or <file> is required")
		}

		if err != nil {
			return err
		}

		objType, err := parseObjectType(hashObjectType)
		if err != nil {
			return err
		}

		obj := r.Storer.NewEncodedObject()
		obj.SetType(objType)
		obj.SetSize(int64(len(content)))

		w, err := obj.Writer()
		if err != nil {
			return err
		}

		if _, err := w.Write(content); err != nil {
			return err
		}

		if err := w.Close(); err != nil {
			return err
		}

		if hashObjectWrite {
			if _, err := r.Storer.SetEncodedObject(obj); err != nil {
				return err
			}
		}

		fmt.Fprintln(cmd.OutOrStdout(), obj.Hash())

		return nil
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

func parseObjectType(s string) (plumbing.ObjectType, error) {
	switch s {
	case "blob":
		return plumbing.BlobObject, nil
	case "tag":
		return plumbing.TagObject, nil
	case "tree":
		return plumbing.TreeObject, nil
	case "commit":
		return plumbing.CommitObject, nil
	default:
		return plumbing.InvalidObject, fmt.Errorf("unsupported object type %q", s)
	}
}
