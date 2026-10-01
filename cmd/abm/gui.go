package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

func newGUICmd(a *app) *cobra.Command {
	var port int
	var open bool
	var noOpen bool
	cmd := &cobra.Command{
		Use:   "gui",
		Short: "Start the local web GUI (binds to 127.0.0.1 only)",
		Long: `Start the local web GUI.

Binds only to 127.0.0.1 -- the GUI is never exposed on a public or LAN
interface by this project. On Windows the default browser opens
automatically; on Linux the URL is printed (and opened too, if a desktop
browser is available via xdg-open). The GUI is a thin layer over the exact
same backup/restore/retention engine as the CLI; nothing it does is only
possible through the GUI.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			addr := fmt.Sprintf("127.0.0.1:%d", port)
			shouldOpen := open && !noOpen

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			return runGUIServer(ctx, a, addr, shouldOpen, a.log)
		},
	}
	cmd.Flags().IntVar(&port, "port", 8765, "port to bind the GUI to (127.0.0.1 only)")
	cmd.Flags().BoolVar(&open, "open", true, "open the default browser automatically")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "never open a browser, just print the URL")
	return cmd
}
