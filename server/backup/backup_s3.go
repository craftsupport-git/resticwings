package backup

import (
	"context"
	"io"
	"os"

	"emperror.dev/errors"
	"github.com/juju/ratelimit"
	"github.com/restic/restic/lib/restic"
	"github.com/restic/restic/lib/repository"
	"github.com/restic/restic/lib/restic/backend/s3"

	"github.com/pterodactyl/wings/config"
	"github.com/pterodactyl/wings/remote"
	"github.com/pterodactyl/wings/server/filesystem"
)

type S3Backup struct {
	Backup
}

var _ BackupInterface = (*S3Backup)(nil)

func NewS3(client remote.Client, uuid string, ignore string) *S3Backup {
	return &S3Backup{
		Backup{
			client:  client,
			Uuid:    uuid,
			Ignore:  ignore,
			adapter: S3BackupAdapter,
		},
	}
}

// Remove removes a backup from the system.
func (s *S3Backup) Remove() error {
	return os.Remove(s.Path())
}

// WithLogContext attaches additional context to the log output for this backup.
func (s *S3Backup) WithLogContext(c map[string]interface{}) {
	s.logContext = c
}

// Generate creates a new backup on the disk, moves it into the S3 bucket via
// the provided presigned URL, and then deletes the backup from the disk.
func (s *S3Backup) Generate(ctx context.Context, fsys *filesystem.Filesystem, ignore string) (*ArchiveDetails, error) {
	defer s.Remove()

	// Initialize the restic repository
	repo, err := repository.NewRepository(s.Path(), s3.New(s.Path()))
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

	ad, err := s.Details(ctx, nil)
	if err != nil {
		return nil, errors.WrapIf(err, "backup: failed to get archive details for S3 backup")
	}
	return ad, nil
}

// Restore will read from the provided reader assuming that it is a gzipped
// tar reader. When a file is encountered in the archive the callback function
// will be triggered. If the callback returns an error the entire process is
// stopped, otherwise this function will run until all files have been written.
//
// This restoration uses a workerpool to use up to the number of CPUs available
// on the machine when writing files to the disk.
func (s *S3Backup) Restore(ctx context.Context, r io.Reader, callback RestoreCallback) error {
	// Initialize the restic repository
	repo, err := repository.NewRepository(s.Path(), s3.New(s.Path()))
	if err != nil {
		return err
	}

	// Find the latest snapshot
	snapshot, err := repo.FindLatestSnapshot(ctx, nil)
	if err != nil {
		return err
	}

	// Restore the snapshot
	err = repo.RestoreSnapshot(ctx, snapshot, s.Path(), callback)
	if err != nil {
		return err
	}

	return nil
}

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
			adapter: S3BackupAdapter,
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
	repo, err := repository.NewRepository(b.Path(), s3.New(b.Path()))
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
	repo, err := repository.NewRepository(b.Path(), s3.New(b.Path()))
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
