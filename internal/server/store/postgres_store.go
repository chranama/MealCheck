package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chranama/MealCheck/internal/workflow/checker"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type PostgresStore struct {
	db *sql.DB
}

func OpenPostgresStore(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required for postgres store")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	store := &PostgresStore{db: db}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := store.Migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *PostgresStore) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, postgresSchema)
	return err
}

func (s *PostgresStore) CreateRun(ctx context.Context, run Run, queueSize int, inviteTokenID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var queued int
	if err := tx.QueryRowContext(ctx, `select count(*) from runs where status = $1`, StatusQueued).Scan(&queued); err != nil {
		return err
	}
	if queued >= queueSize {
		return ErrQueueFull
	}
	if inviteTokenID != "" {
		result, err := tx.ExecContext(ctx, `
			update invite_tokens
			set used_runs = used_runs + 1,
				last_used_at = $2
			where id = $1
				and revoked_at is null
				and (expires_at is null or expires_at > $2)
				and (max_runs is null or used_runs < max_runs)
		`, inviteTokenID, run.CreatedAt)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count == 0 {
			var usedRuns int
			var maxRuns sql.NullInt64
			statusErr := tx.QueryRowContext(ctx, `
				select used_runs, max_runs
				from invite_tokens
				where id = $1
			`, inviteTokenID).Scan(&usedRuns, &maxRuns)
			if statusErr == nil && maxRuns.Valid && usedRuns >= int(maxRuns.Int64) {
				return ErrInviteRunLimit
			}
			return ErrInviteUnavailable
		}
	}

	_, err = tx.ExecContext(ctx, `
		insert into runs (
			id, case_path, status, decision, risk_level, summary, artifact_dir,
			error_message, created_at, updated_at, started_at, completed_at, expires_at
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, run.ID, run.CasePath, run.Status, nullString(run.Decision), nullString(run.RiskLevel),
		nullString(run.Summary), run.ArtifactDir, nullString(run.Error), run.CreatedAt, run.UpdatedAt,
		run.StartedAt, run.CompletedAt, run.ExpiresAt)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *PostgresStore) GetRun(ctx context.Context, id string) (Run, error) {
	row := s.db.QueryRowContext(ctx, `
		select id, case_path, status, coalesce(decision, ''), coalesce(risk_level, ''),
			coalesce(summary, ''), artifact_dir, coalesce(error_message, ''),
			created_at, updated_at, started_at, completed_at, expires_at
		from runs
		where id = $1 and status <> $2
	`, id, StatusDeleted)
	return scanRun(row)
}

func (s *PostgresStore) ClaimNextRun(ctx context.Context, workerID string, leaseUntil time.Time) (Run, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Run{}, false, err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	row := tx.QueryRowContext(ctx, `
		update runs
		set status = $1,
			started_at = coalesce(started_at, $2),
			updated_at = $2,
			lease_owner = $3,
			lease_expires_at = $4
		where id = (
			select id
			from runs
			where status = $5
			order by created_at
			for update skip locked
			limit 1
		)
		returning id, case_path, status, coalesce(decision, ''), coalesce(risk_level, ''),
			coalesce(summary, ''), artifact_dir, coalesce(error_message, ''),
			created_at, updated_at, started_at, completed_at, expires_at
	`, StatusRunning, now, workerID, leaseUntil, StatusQueued)
	run, err := scanRun(row)
	if errors.Is(err, ErrNotFound) {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Run{}, false, err
	}
	return run, true, nil
}

func (s *PostgresStore) CompleteRun(ctx context.Context, id string, decision checker.DecisionDocument, completedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		update runs
		set status = $1,
			decision = $2,
			risk_level = $3,
			summary = $4,
			completed_at = $5,
			updated_at = $5,
			lease_owner = null,
			lease_expires_at = null
		where id = $6
	`, StatusCompleted, decision.Decision, decision.RiskLevel, decision.Summary, completedAt, id)
	return checkRows(result, err)
}

func (s *PostgresStore) FailRun(ctx context.Context, id string, message string, completedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		update runs
		set status = $1,
			error_message = $2,
			completed_at = $3,
			updated_at = $3,
			lease_owner = null,
			lease_expires_at = null
		where id = $4
	`, StatusFailed, message, completedAt, id)
	return checkRows(result, err)
}

func (s *PostgresStore) DeleteRun(ctx context.Context, id string) (Run, error) {
	run, err := s.GetRun(ctx, id)
	if err != nil {
		return Run{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		update runs
		set status = $1, updated_at = $2
		where id = $3
	`, StatusDeleted, time.Now().UTC(), id)
	if err != nil {
		return Run{}, err
	}
	return run, nil
}

func (s *PostgresStore) ExpiredRuns(ctx context.Context, now time.Time, limit int) ([]Run, error) {
	rows, err := s.db.QueryContext(ctx, `
		update runs
		set status = $1, updated_at = $2
		where id in (
			select id
			from runs
			where status <> $1 and expires_at <= $2
			order by expires_at
			limit $3
		)
		returning id, case_path, status, coalesce(decision, ''), coalesce(risk_level, ''),
			coalesce(summary, ''), artifact_dir, coalesce(error_message, ''),
			created_at, updated_at, started_at, completed_at, expires_at
	`, StatusDeleted, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *PostgresStore) AppendEvent(ctx context.Context, runID, eventType, message string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		insert into run_events (run_id, event_type, message, created_at)
		values ($1, $2, $3, $4)
	`, runID, eventType, message, at)
	return err
}

func (s *PostgresStore) ListEvents(ctx context.Context, runID string, afterID int64) ([]RunEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, run_id, event_type, message, created_at
		from run_events
		where run_id = $1 and id > $2
		order by id
	`, runID, afterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []RunEvent
	for rows.Next() {
		var event RunEvent
		if err := rows.Scan(&event.ID, &event.RunID, &event.Type, &event.Message, &event.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *PostgresStore) Stats(ctx context.Context) (StoreStats, error) {
	rows, err := s.db.QueryContext(ctx, `
		select status, count(*)
		from runs
		where status in ($1, $2)
		group by status
	`, StatusQueued, StatusRunning)
	if err != nil {
		return StoreStats{}, err
	}
	defer rows.Close()

	var stats StoreStats
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return StoreStats{}, err
		}
		switch status {
		case StatusQueued:
			stats.Queued = count
		case StatusRunning:
			stats.Running = count
		}
	}
	return stats, rows.Err()
}

func (s *PostgresStore) CreateInviteToken(ctx context.Context, invite InviteToken) error {
	_, err := s.db.ExecContext(ctx, `
		insert into invite_tokens (
			id, secret_hash, label, created_at, expires_at, revoked_at,
			max_runs, used_runs, last_used_at
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, invite.ID, invite.SecretHash, invite.Label, invite.CreatedAt, invite.ExpiresAt,
		invite.RevokedAt, nullInt(invite.MaxRuns), invite.UsedRuns, invite.LastUsedAt)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

func (s *PostgresStore) GetInviteToken(ctx context.Context, id string) (InviteToken, error) {
	row := s.db.QueryRowContext(ctx, `
		select id, secret_hash, label, created_at, expires_at, revoked_at,
			max_runs, used_runs, last_used_at
		from invite_tokens
		where id = $1
	`, id)
	return scanInviteToken(row)
}

func (s *PostgresStore) ListInviteTokens(ctx context.Context) ([]InviteToken, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, secret_hash, label, created_at, expires_at, revoked_at,
			max_runs, used_runs, last_used_at
		from invite_tokens
		order by created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invites []InviteToken
	for rows.Next() {
		invite, err := scanInviteToken(rows)
		if err != nil {
			return nil, err
		}
		invites = append(invites, invite)
	}
	return invites, rows.Err()
}

func (s *PostgresStore) RevokeInviteToken(ctx context.Context, id string, at time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		update invite_tokens
		set revoked_at = $2
		where id = $1
	`, id, at.UTC())
	return checkRows(result, err)
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

type runScanner interface {
	Scan(dest ...any) error
}

func scanRun(scanner runScanner) (Run, error) {
	var run Run
	err := scanner.Scan(
		&run.ID,
		&run.CasePath,
		&run.Status,
		&run.Decision,
		&run.RiskLevel,
		&run.Summary,
		&run.ArtifactDir,
		&run.Error,
		&run.CreatedAt,
		&run.UpdatedAt,
		&run.StartedAt,
		&run.CompletedAt,
		&run.ExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	return run, err
}

func checkRows(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func nullString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func nullInt(value *int) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*value), Valid: true}
}

func scanInviteToken(scanner runScanner) (InviteToken, error) {
	var invite InviteToken
	var expiresAt sql.NullTime
	var revokedAt sql.NullTime
	var maxRuns sql.NullInt64
	var lastUsedAt sql.NullTime
	err := scanner.Scan(
		&invite.ID,
		&invite.SecretHash,
		&invite.Label,
		&invite.CreatedAt,
		&expiresAt,
		&revokedAt,
		&maxRuns,
		&invite.UsedRuns,
		&lastUsedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return InviteToken{}, ErrNotFound
	}
	if err != nil {
		return InviteToken{}, err
	}
	if expiresAt.Valid {
		invite.ExpiresAt = &expiresAt.Time
	}
	if revokedAt.Valid {
		invite.RevokedAt = &revokedAt.Time
	}
	if maxRuns.Valid {
		value := int(maxRuns.Int64)
		invite.MaxRuns = &value
	}
	if lastUsedAt.Valid {
		invite.LastUsedAt = &lastUsedAt.Time
	}
	return invite, nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLSTATE 23505")
}

const postgresSchema = `
create table if not exists runs (
	id text primary key,
	case_path text not null,
	status text not null,
	decision text,
	risk_level text,
	summary text,
	artifact_dir text not null,
	error_message text,
	created_at timestamptz not null,
	updated_at timestamptz not null,
	started_at timestamptz,
	completed_at timestamptz,
	expires_at timestamptz not null,
	lease_owner text,
	lease_expires_at timestamptz
);

create index if not exists runs_status_created_idx on runs (status, created_at);
create index if not exists runs_expires_at_idx on runs (expires_at);

create table if not exists run_events (
	id bigserial primary key,
	run_id text not null references runs(id) on delete cascade,
	event_type text not null,
	message text not null,
	created_at timestamptz not null
);

create index if not exists run_events_run_id_id_idx on run_events (run_id, id);

create table if not exists invite_tokens (
	id text primary key,
	secret_hash text not null,
	label text not null,
	created_at timestamptz not null,
	expires_at timestamptz,
	revoked_at timestamptz,
	max_runs integer,
	used_runs integer not null default 0,
	last_used_at timestamptz
);

create index if not exists invite_tokens_active_idx on invite_tokens (revoked_at, expires_at);
`
