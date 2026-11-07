package deployment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/lani772/all-in-one/engine/internal/artifact"
)

var ErrProjectNotFound = errors.New("project not found")

type PostgresRepository struct {
	db *sql.DB
}

func OpenPostgres(ctx context.Context, databaseURL string) (*sql.DB, error) {
	if databaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect PostgreSQL: %w", err)
	}
	return db, nil
}

func NewPostgresRepository(db *sql.DB) (*PostgresRepository, error) {
	if db == nil {
		return nil, errors.New("PostgreSQL database is required")
	}
	return &PostgresRepository{db: db}, nil
}

// CommitDeployment serializes publishes for a project by locking its row.
// All metadata writes and the active-deployment pointer switch commit together.
func (r *PostgresRepository) CommitDeployment(ctx context.Context, ownerID string, manifest artifact.Manifest, manifestKey string) error {
	if r == nil || r.db == nil {
		return errors.New("deployment repository is not configured")
	}
	if err := validateCommit(ownerID, manifest, manifestKey); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin deployment transaction: %w", err)
	}
	defer tx.Rollback()

	var actualOwner string
	err = tx.QueryRowContext(ctx,
		`SELECT owner_id FROM projects WHERE id = $1 FOR UPDATE`,
		manifest.ProjectID,
	).Scan(&actualOwner)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && actualOwner != ownerID) {
		return ErrProjectNotFound
	}
	if err != nil {
		return fmt.Errorf("lock project ownership row: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE deployments SET status = 'superseded'
		 WHERE project_id = $1 AND status = 'ready'`,
		manifest.ProjectID,
	)
	if err != nil {
		return fmt.Errorf("supersede previous deployment: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO deployments
		 (id, project_id, owner_id, build_id, status, manifest_key, total_bytes, file_count, created_at)
		 VALUES ($1,$2,$3,$4,'ready',$5,$6,$7,$8)`,
		manifest.DeploymentID, manifest.ProjectID, ownerID, manifest.BuildID,
		manifestKey, manifest.TotalBytes, len(manifest.Files), manifestTime(manifest.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert deployment metadata: %w", err)
	}

	for _, file := range manifest.Files {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO deployment_files
			 (deployment_id, path, object_key, size_bytes, sha256, content_type)
			 VALUES ($1,$2,$3,$4,$5,$6)`,
			manifest.DeploymentID, file.Path, file.ObjectKey, file.Size, file.SHA256, file.ContentType,
		)
		if err != nil {
			return fmt.Errorf("insert deployment file %q: %w", file.Path, err)
		}
	}

	result, err := tx.ExecContext(ctx,
		`UPDATE projects SET current_deployment_id = $1
		 WHERE id = $2 AND owner_id = $3`,
		manifest.DeploymentID, manifest.ProjectID, ownerID,
	)
	if err != nil {
		return fmt.Errorf("switch current deployment: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("verify current deployment update: %w", err)
	}
	if affected != 1 {
		return ErrProjectNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit deployment transaction: %w", err)
	}
	return nil
}

func validateCommit(ownerID string, manifest artifact.Manifest, manifestKey string) error {
	if !validID(ownerID) || !validID(manifest.ProjectID) ||
		!validID(manifest.DeploymentID) || !validID(manifest.BuildID) {
		return artifact.ErrInvalidIdentifier
	}
	expectedManifestKey := path.Join("sites", manifest.ProjectID, "deployments", manifest.DeploymentID, "manifest.json")
	if manifestKey != expectedManifestKey || manifest.Version != 1 || len(manifest.Files) == 0 {
		return errors.New("deployment manifest metadata is invalid")
	}
	if _, err := time.Parse(time.RFC3339Nano, manifest.CreatedAt); err != nil {
		return errors.New("deployment manifest timestamp is invalid")
	}
	var total int64
	seen := make(map[string]struct{}, len(manifest.Files))
	for _, file := range manifest.Files {
		if file.Path == "" || strings.Contains(file.Path, "\\") || path.IsAbs(file.Path) || path.Clean(file.Path) != file.Path || !path.IsLocal(file.Path) {
			return fmt.Errorf("unsafe deployment file path %q", file.Path)
		}
		if _, ok := seen[file.Path]; ok {
			return fmt.Errorf("duplicate deployment file path %q", file.Path)
		}
		seen[file.Path] = struct{}{}
		expectedKey := path.Join("sites", manifest.ProjectID, "deployments", manifest.DeploymentID, "files", file.Path)
		if file.ObjectKey != expectedKey || file.Size < 0 || len(file.SHA256) != 64 || file.ContentType == "" {
			return fmt.Errorf("deployment file metadata is invalid for %q", file.Path)
		}
		for _, c := range file.SHA256 {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				return fmt.Errorf("invalid SHA-256 for %q", file.Path)
			}
		}
		if total > int64(^uint64(0)>>1)-file.Size {
			return errors.New("deployment total size overflows")
		}
		total += file.Size
	}
	if total != manifest.TotalBytes {
		return errors.New("deployment total byte count does not match manifest")
	}
	return nil
}

func validID(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for i, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || (i > 0 && (r == '-' || r == '_')) {
			continue
		}
		return false
	}
	return true
}

func manifestTime(value string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, value)
	return t.UTC()
}
