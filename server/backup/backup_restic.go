package backup

import (
	"context"
	"io"
	"os"

	"emperror.dev/errors"
	"github.com/restic/restic/lib/restic"
	"github.com/restic/restic/lib/repository"
	"github.com/restic/restic/lib/restic/backend/local"

	"github.com/pterodactyl/wings/remote"
	"github.com/pterodactyl/wings/server/filesystem"
)

type ResticBackup struct {
	Backup
}

var _ BackupInterface = (*ResticBackup)(nil)

func NewRestic(client remote.Client, uuid string, ignore string) *ResticBackup {
	return &ResticBackup{
		Backup{
			client:  client,
			Uuid:    uuid,
			Ignore:  ignore,
			adapter: LocalBackupAdapter,
		},
	}
}

// LocateRestic finds the restic backup for a server and returns the local path.
func LocateRestic(client remote.Client, uuid string) (*ResticBackup, os.FileInfo, error) {
	b := NewRestic(client, uuid, "")
	st, err := os.Stat(b.Path())
	if err != nil {
		return nil, nil, err
	}

	if st.IsDir() {
		return nil, nil, errors.New("invalid archive, is directory")
	}

	return b, st, nil
}

// Remove removes a restic backup from the system.
func (b *ResticBackup) Remove() error {
	return os.Remove(b.Path())
}

// WithLogContext attaches additional context to the log output for this restic backup.
func (b *ResticBackup) WithLogContext(c map[string]interface{}) {
	b.logContext = c
}

// Generate generates a restic backup of the selected files and pushes it to the
// defined location for this instance.
func (b *ResticBackup) Generate(ctx context.Context, fsys *filesystem.Filesystem, ignore string) (*ArchiveDetails, error) {
	// Initialize the restic repository
	repo, err := repository.NewRepository(b.Path(), local.New(b.Path()))
	if err != nil {
		return nil, err
	}

	// Create a new snapshot
	snapshot, err := restic.NewSnapshot([]string{fsys.Path()}, nil, nil)
	if err != nil {
		return nil, err
	}

	// Save the snapshot to the repository
	err = repo.SaveSnapshot(ctx, snapshot)
	if err != nil {
		return nil, err
	}

	ad, err := b.Details(ctx, nil)
	if err != nil {
		return nil, errors.WrapIf(err, "backup: failed to get archive details for restic backup")
	}
	return ad, nil
}

// Restore will walk over the restic archive and call the callback function for each
// file encountered.
func (b *ResticBackup) Restore(ctx context.Context, _ io.Reader, callback RestoreCallback) error {
	// Initialize the restic repository
	repo, err := repository.NewRepository(b.Path(), local.New(b.Path()))
	if err != nil {
		return err
	}

	// Find the latest snapshot
	snapshot, err := repo.FindLatestSnapshot(ctx, nil)
	if err != nil {
		return err
	}

	// Restore the snapshot
	err = repo.RestoreSnapshot(ctx, snapshot, b.Path(), callback)
	if err != nil {
		return err
	}

	return nil
}

// Mount mounts a restic backup to the specified mount point.
func (b *ResticBackup) Mount(ctx context.Context, mountPoint string) error {
	// Initialize the restic repository
	repo, err := repository.NewRepository(b.Path(), local.New(b.Path()))
	if err != nil {
		return err
	}

	// Find the latest snapshot
	snapshot, err := repo.FindLatestSnapshot(ctx, nil)
	if err != nil {
		return err
	}

	// Mount the snapshot
	err = repo.MountSnapshot(ctx, snapshot, mountPoint)
	if err != nil {
		return err
	}

	return nil
}
