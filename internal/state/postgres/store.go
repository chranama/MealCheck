package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chranama/MealCheck/internal/state"
	"github.com/chranama/MealCheck/internal/workflow/checker"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type Store struct {
	db *sql.DB
}

var _ state.Store = (*Store)(nil)

const localModelClaimAdvisoryLockKey int64 = 0x4d434c4d4f44454c

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required for postgres store")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
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

func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, postgresSchema)
	return err
}

func (s *Store) CreateRun(ctx context.Context, run state.Run, queueSize int, inviteTokenID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var queued int
	if err := tx.QueryRowContext(ctx, `select count(*) from runs where status = $1`, state.StatusQueued).Scan(&queued); err != nil {
		return err
	}
	if queued >= queueSize {
		return state.ErrQueueFull
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
				return state.ErrInviteRunLimit
			}
			return state.ErrInviteUnavailable
		}
	}

	_, err = tx.ExecContext(ctx, `
		insert into runs (
			id, case_path, input_mode, status, decision, risk_level, summary, artifact_dir,
			error_message, created_at, updated_at, started_at, completed_at, expires_at
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, run.ID, run.CasePath, run.InputMode, run.Status, nullString(run.Decision), nullString(run.RiskLevel),
		nullString(run.Summary), run.ArtifactDir, nullString(run.Error), run.CreatedAt, run.UpdatedAt,
		run.StartedAt, run.CompletedAt, run.ExpiresAt)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetRun(ctx context.Context, id string) (state.Run, error) {
	row := s.db.QueryRowContext(ctx, `
		select id, case_path, coalesce(input_mode, ''), status, coalesce(decision, ''), coalesce(risk_level, ''),
			coalesce(summary, ''), artifact_dir, coalesce(error_message, ''),
			created_at, updated_at, started_at, completed_at, expires_at
		from runs
		where id = $1 and status <> $2
	`, id, state.StatusDeleted)
	return scanRun(row)
}

func (s *Store) ClaimNextRun(ctx context.Context, workerID string, leaseUntil time.Time) (state.Run, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return state.Run{}, false, err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `select pg_advisory_xact_lock($1)`, localModelClaimAdvisoryLockKey); err != nil {
		return state.Run{}, false, err
	}
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
				and (
					coalesce(input_mode, '') <> $6
					or not exists (
						select 1
						from runs active
						where active.status = $1
							and coalesce(active.input_mode, '') = $6
					)
				)
			order by created_at
			for update skip locked
			limit 1
		)
		returning id, case_path, coalesce(input_mode, ''), status, coalesce(decision, ''), coalesce(risk_level, ''),
			coalesce(summary, ''), artifact_dir, coalesce(error_message, ''),
			created_at, updated_at, started_at, completed_at, expires_at
	`, state.StatusRunning, now, workerID, leaseUntil, state.StatusQueued, state.InputModeLocalModel)
	run, err := scanRun(row)
	if errors.Is(err, state.ErrNotFound) {
		return state.Run{}, false, nil
	}
	if err != nil {
		return state.Run{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return state.Run{}, false, err
	}
	return run, true, nil
}

func (s *Store) MarkRunAwaitingReview(ctx context.Context, id string, summary string, at time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		update runs
		set status = $1,
			summary = $2,
			updated_at = $3,
			lease_owner = null,
			lease_expires_at = null
		where id = $4
	`, state.StatusAwaitingReview, summary, at, id)
	return checkRows(result, err)
}

func (s *Store) StartReviewRun(ctx context.Context, id string, workerID string, leaseUntil time.Time, at time.Time) (state.Run, error) {
	row := s.db.QueryRowContext(ctx, `
		update runs
		set status = $1,
			started_at = coalesce(started_at, $2),
			updated_at = $2,
			lease_owner = $3,
			lease_expires_at = $4
		where id = $5 and status = $6
		returning id, case_path, coalesce(input_mode, ''), status, coalesce(decision, ''), coalesce(risk_level, ''),
			coalesce(summary, ''), artifact_dir, coalesce(error_message, ''),
			created_at, updated_at, started_at, completed_at, expires_at
	`, state.StatusRunning, at, workerID, leaseUntil, id, state.StatusAwaitingReview)
	run, err := scanRun(row)
	if errors.Is(err, state.ErrNotFound) {
		return state.Run{}, state.ErrConflict
	}
	return run, err
}

func (s *Store) CompleteRun(ctx context.Context, id string, decision checker.DecisionDocument, completedAt time.Time) error {
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
	`, state.StatusCompleted, decision.Decision, decision.RiskLevel, decision.Summary, completedAt, id)
	return checkRows(result, err)
}

func (s *Store) FailRun(ctx context.Context, id string, message string, completedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		update runs
		set status = $1,
			error_message = $2,
			completed_at = $3,
			updated_at = $3,
			lease_owner = null,
			lease_expires_at = null
		where id = $4
	`, state.StatusFailed, message, completedAt, id)
	return checkRows(result, err)
}

func (s *Store) DeleteRun(ctx context.Context, id string) (state.Run, error) {
	run, err := s.GetRun(ctx, id)
	if err != nil {
		return state.Run{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		update runs
		set status = $1, updated_at = $2
		where id = $3
	`, state.StatusDeleted, time.Now().UTC(), id)
	if err != nil {
		return state.Run{}, err
	}
	return run, nil
}

func (s *Store) ExpiredRuns(ctx context.Context, now time.Time, limit int) ([]state.Run, error) {
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
		returning id, case_path, coalesce(input_mode, ''), status, coalesce(decision, ''), coalesce(risk_level, ''),
			coalesce(summary, ''), artifact_dir, coalesce(error_message, ''),
			created_at, updated_at, started_at, completed_at, expires_at
	`, state.StatusDeleted, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []state.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *Store) AppendEvent(ctx context.Context, runID, eventType, message string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		insert into run_events (run_id, event_type, message, created_at)
		values ($1, $2, $3, $4)
	`, runID, eventType, message, at)
	return err
}

func (s *Store) ListEvents(ctx context.Context, runID string, afterID int64) ([]state.RunEvent, error) {
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

	var events []state.RunEvent
	for rows.Next() {
		var event state.RunEvent
		if err := rows.Scan(&event.ID, &event.RunID, &event.Type, &event.Message, &event.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) Stats(ctx context.Context) (state.StoreStats, error) {
	rows, err := s.db.QueryContext(ctx, `
		select status, count(*)
		from runs
		where status in ($1, $2)
		group by status
	`, state.StatusQueued, state.StatusRunning)
	if err != nil {
		return state.StoreStats{}, err
	}
	defer rows.Close()

	var stats state.StoreStats
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return state.StoreStats{}, err
		}
		switch status {
		case state.StatusQueued:
			stats.Queued = count
		case state.StatusRunning:
			stats.Running = count
		}
	}
	return stats, rows.Err()
}

func (s *Store) CreateInviteToken(ctx context.Context, invite state.InviteToken) error {
	_, err := s.db.ExecContext(ctx, `
		insert into invite_tokens (
			id, secret_hash, label, created_at, expires_at, revoked_at,
			max_runs, used_runs, last_used_at
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, invite.ID, invite.SecretHash, invite.Label, invite.CreatedAt, invite.ExpiresAt,
		invite.RevokedAt, nullInt(invite.MaxRuns), invite.UsedRuns, invite.LastUsedAt)
	if isUniqueViolation(err) {
		return state.ErrConflict
	}
	return err
}

func (s *Store) GetInviteToken(ctx context.Context, id string) (state.InviteToken, error) {
	row := s.db.QueryRowContext(ctx, `
		select id, secret_hash, label, created_at, expires_at, revoked_at,
			max_runs, used_runs, last_used_at
		from invite_tokens
		where id = $1
	`, id)
	return scanInviteToken(row)
}

func (s *Store) ListInviteTokens(ctx context.Context) ([]state.InviteToken, error) {
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

	var invites []state.InviteToken
	for rows.Next() {
		invite, err := scanInviteToken(rows)
		if err != nil {
			return nil, err
		}
		invites = append(invites, invite)
	}
	return invites, rows.Err()
}

func (s *Store) RevokeInviteToken(ctx context.Context, id string, at time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		update invite_tokens
		set revoked_at = $2
		where id = $1
	`, id, at.UTC())
	return checkRows(result, err)
}

func (s *Store) Close() error {
	return s.db.Close()
}

type runScanner interface {
	Scan(dest ...any) error
}

func scanRun(scanner runScanner) (state.Run, error) {
	var run state.Run
	err := scanner.Scan(
		&run.ID,
		&run.CasePath,
		&run.InputMode,
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
		return state.Run{}, state.ErrNotFound
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
		return state.ErrNotFound
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

func scanInviteToken(scanner runScanner) (state.InviteToken, error) {
	var invite state.InviteToken
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
		return state.InviteToken{}, state.ErrNotFound
	}
	if err != nil {
		return state.InviteToken{}, err
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
	input_mode text not null default '',
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

alter table runs add column if not exists input_mode text not null default '';

create index if not exists runs_status_created_idx on runs (status, created_at);
create index if not exists runs_status_input_mode_created_idx on runs (status, input_mode, created_at);
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
