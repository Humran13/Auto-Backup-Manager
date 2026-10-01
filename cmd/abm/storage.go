package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Humran13/Auto-Backup-Manager/internal/config"
	"github.com/Humran13/Auto-Backup-Manager/internal/paths"
	"github.com/Humran13/Auto-Backup-Manager/internal/rclone"
)

func rcloneRunner() *rclone.Runner {
	return &rclone.Runner{ConfigPath: paths.RcloneConfigFile()}
}

func newStorageCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "storage",
		Short: "Manage backup storage destinations",
	}
	cmd.AddCommand(newStorageListCmd(a), newStorageTestCmd(a), newStorageAddCmd(a))
	return cmd
}

func newStorageListCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured storage destinations",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := a.requireConfig()
			for _, s := range cfg.Storage {
				immutable := ""
				if s.Immutable {
					immutable = " (immutable)"
				}
				fmt.Printf("%-20s type=%-10s remote=%s%s\n", s.Name, s.Type, s.RcloneRemote, immutable)
			}
			return nil
		},
	}
}

func newStorageTestCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "test <name>",
		Short: "Test connectivity to a configured storage destination",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := a.requireConfig()
			var target *config.Storage
			for i := range cfg.Storage {
				if cfg.Storage[i].Name == args[0] {
					target = &cfg.Storage[i]
				}
			}
			if target == nil {
				return fmt.Errorf("no such storage %q", args[0])
			}
			if target.Type == config.StorageLocal {
				path := target.Options["path"]
				if _, err := os.Stat(path); err != nil {
					return fmt.Errorf("local path %q not reachable: %w", path, err)
				}
				fmt.Printf("%s: OK (local path %s reachable)\n", target.Name, path)
				return nil
			}
			if err := rcloneRunner().Test(context.Background(), target.RcloneRemote); err != nil {
				return fmt.Errorf("%s: FAILED: %w", target.Name, err)
			}
			fmt.Printf("%s: OK\n", target.Name)
			return nil
		},
	}
}

func newStorageAddCmd(a *app) *cobra.Command {
	var (
		name, storageType, remote, path                      string
		endpoint, region, accessKey, secretKey                string
		host, user, keyFile                                   string
		port                                                  int
		immutable                                             bool
	)
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a storage destination",
		Long: `Add a storage destination.

For Google Drive / OneDrive / Dropbox (OAuth providers), first run
'rclone config' with --config ` + paths.RcloneConfigFile() + ` to complete
browser (or, on a headless VPS, 'rclone authorize') authentication and create
the named remote, then run 'abm storage add --type gdrive --name X --remote X'
to register it here. For S3 and SFTP this command creates the rclone remote
for you non-interactively.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			st := config.StorageType(storageType)
			s := config.Storage{Name: name, Type: st, RcloneRemote: remote, Immutable: immutable, Options: map[string]string{}}

			ctx := context.Background()
			r := rcloneRunner()

			switch st {
			case config.StorageS3:
				if s.RcloneRemote == "" {
					s.RcloneRemote = name
				}
				if err := r.CreateS3Remote(ctx, rclone.S3Config{
					Name: s.RcloneRemote, Endpoint: endpoint, Region: region, AccessKey: accessKey, SecretKey: secretKey,
				}); err != nil {
					return fmt.Errorf("creating S3 remote: %w", err)
				}
				s.Options["bucket_endpoint"] = endpoint
			case config.StorageSFTP:
				if s.RcloneRemote == "" {
					s.RcloneRemote = name
				}
				if err := r.CreateSFTPRemote(ctx, rclone.SFTPConfig{
					Name: s.RcloneRemote, Host: host, Port: port, User: user, KeyFile: keyFile, RemotePath: path,
				}); err != nil {
					return fmt.Errorf("creating SFTP remote: %w", err)
				}
			case config.StorageLocal:
				if path == "" {
					return fmt.Errorf("--path is required for local storage")
				}
				s.Options["path"] = path
			case config.StorageGoogleDrive, config.StorageOneDrive, config.StorageDropbox:
				if s.RcloneRemote == "" {
					return fmt.Errorf("--remote is required: create it first with 'rclone config --config %s'", paths.RcloneConfigFile())
				}
				remotes, err := r.ListRemotes(ctx)
				if err != nil {
					return fmt.Errorf("checking rclone remotes: %w", err)
				}
				found := false
				for _, existing := range remotes {
					if existing == s.RcloneRemote {
						found = true
					}
				}
				if !found {
					return fmt.Errorf("rclone remote %q does not exist yet; run 'rclone config --config %s' first", s.RcloneRemote, paths.RcloneConfigFile())
				}
			default:
				return fmt.Errorf("unknown storage type %q", storageType)
			}

			cfg := a.cfg
			if cfg == nil {
				return fmt.Errorf("no configuration found. Run 'abm setup' first")
			}
			cfg.Storage = append(cfg.Storage, s)
			if err := config.Save(paths.ConfigFile(), cfg); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}
			fmt.Printf("storage %q added\n", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "storage destination name")
	cmd.Flags().StringVar(&storageType, "type", "", "gdrive|onedrive|dropbox|s3|sftp|local")
	cmd.Flags().StringVar(&remote, "remote", "", "rclone remote name (existing, for OAuth providers)")
	cmd.Flags().StringVar(&path, "path", "", "local filesystem path (local) or default remote path (sftp)")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "S3 endpoint URL")
	cmd.Flags().StringVar(&region, "region", "", "S3 region")
	cmd.Flags().StringVar(&accessKey, "access-key", "", "S3 access key")
	cmd.Flags().StringVar(&secretKey, "secret-key", "", "S3 secret key")
	cmd.Flags().StringVar(&host, "host", "", "SFTP host")
	cmd.Flags().IntVar(&port, "port", 22, "SFTP port")
	cmd.Flags().StringVar(&user, "user", "", "SFTP username")
	cmd.Flags().StringVar(&keyFile, "key-file", "", "SFTP private key file (preferred over a password)")
	cmd.Flags().BoolVar(&immutable, "immutable", false, "mark this destination as Object-Lock/WORM capable")
	return cmd
}
