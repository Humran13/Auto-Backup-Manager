package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newCheckCmd(a *app) *cobra.Command {
	var readDataSubset string
	cmd := &cobra.Command{
		Use:   "check [job]",
		Short: "Verify repository integrity (lightweight by default)",
		Long: `Verify repository integrity for one job, or all jobs.

Without --read-data-subset this only checks structural consistency (fast,
safe to run often). Pass e.g. --read-data-subset=5% to additionally verify a
rotating slice of actual pack data without downloading the whole repository.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := a.requireConfig()
			ctx := context.Background()

			names := args
			if len(names) == 0 {
				for name := range cfg.Jobs {
					names = append(names, name)
				}
			}

			failed := false
			for _, name := range names {
				r, err := resticRunnerFor(a, name)
				if err != nil {
					fmt.Printf("%s: %v\n", name, err)
					failed = true
					continue
				}
				if err := r.Check(ctx, readDataSubset); err != nil {
					fmt.Printf("%s: CHECK FAILED: %v\n", name, err)
					failed = true
					continue
				}
				fmt.Printf("%s: OK\n", name)
			}
			if failed {
				return fmt.Errorf("one or more repositories failed integrity check")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&readDataSubset, "read-data-subset", "", "also verify a subset of pack data, e.g. \"5%\" or \"1/20\"")
	return cmd
}
