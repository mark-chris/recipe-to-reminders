package storage

import (
	"context"
	"errors"
	"io"

	"recipe-to-reminders/internal/models"
)

// Sentinel errors for storage operations.
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict: ETag mismatch")
)

// Store abstracts recipe collection persistence.
type Store interface {
	ReadAll(ctx context.Context) (*models.RecipeCollection, string, error)
	WriteAll(ctx context.Context, collection *models.RecipeCollection, etag string) error
}

// S3API abstracts the subset of S3 operations needed by S3Store.
type S3API interface {
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, string, error)
	PutObject(ctx context.Context, bucket, key string, data []byte, ifMatch string) error
	GetObjectVersion(ctx context.Context, bucket, key, versionID string) (io.ReadCloser, error)
	ListObjectVersions(ctx context.Context, bucket, key string, maxKeys int) ([]ObjectVersion, error)
}

// ObjectVersion represents an S3 object version.
type ObjectVersion struct {
	VersionID string
	IsLatest  bool
}
