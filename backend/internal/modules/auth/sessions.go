package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Will413028/fullstack-template/backend/internal/modules/auth/internal/store"
	"github.com/Will413028/fullstack-template/backend/internal/platform/apperr"
	"github.com/Will413028/fullstack-template/backend/internal/platform/authctx"
	"github.com/Will413028/fullstack-template/backend/internal/platform/httpx"
)

// touchEvery limits how often a session's last_used_at is written: once a minute is enough for a 1-day idle limit
// and keeps reads from writing on every request.
const touchEvery = time.Minute

// throttleTTL is how long a sign_in_throttles row is kept after its last attempt; the longest wait is an hour.
const throttleTTL = 24 * time.Hour

// ErrSessionInvalid is returned for a missing, unknown, expired or revoked session and for a disabled user. Callers
// treat the request as anonymous; the reason is not told apart because the client's remedy is always to sign in.
var ErrSessionInvalid = apperr.New(apperr.Unauthenticated, "session_invalid", "no valid session")

func newToken() string {
	var b [32]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never returns an error
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func (s *Service) newSession(ctx context.Context, q *store.Queries, userID uuid.UUID) (Session, error) {
	now := s.now()
	token := newToken()
	row, err := q.InsertSession(ctx, store.InsertSessionParams{
		TokenHash: hashToken(token), UserID: userID, Now: now, ExpiresAt: now.Add(s.cfg.SessionAbsolute),
	})
	if err != nil {
		return Session{}, fmt.Errorf("auth: create session: %w", err)
	}
	return Session{SessionInfo: SessionInfo(row), Token: token, MaxAge: s.cfg.SessionAbsolute}, nil
}

// Authenticate returns the user of the session token and the session's id. The session must be within its
// absolute and idle limits and its user enabled.
func (s *Service) Authenticate(ctx context.Context, token string) (User, uuid.UUID, error) {
	if token == "" {
		return User{}, uuid.Nil, ErrSessionInvalid
	}
	hash := hashToken(token)
	row, err := s.q().GetSession(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, uuid.Nil, ErrSessionInvalid
	}
	if err != nil {
		return User{}, uuid.Nil, fmt.Errorf("auth: get session: %w", err)
	}
	now := s.now()
	if !now.Before(row.ExpiresAt) || !now.Before(row.LastUsedAt.Add(s.cfg.SessionIdle)) {
		return User{}, uuid.Nil, ErrSessionInvalid
	}
	if row.IsDisabled {
		return User{}, uuid.Nil, ErrSessionInvalid
	}
	// a short idle limit (under two minutes) is touched more often, or a session in use would still lapse
	if now.Sub(row.LastUsedAt) >= min(touchEvery, s.cfg.SessionIdle/2) {
		if err := s.q().TouchSession(ctx, store.TouchSessionParams{Now: now, TokenHash: hash}); err != nil {
			return User{}, uuid.Nil, fmt.Errorf("auth: touch session: %w", err)
		}
	}
	u := User{ID: row.ID, Account: row.Account, Role: row.Role, IsDisabled: row.IsDisabled, CreatedAt: row.CreatedAt}
	return u, row.SessionID, nil
}

// SignIn checks the account and password and issues a new session. previousToken, the session the request
// carried, is deleted: a new token on every sign-in prevents session fixation, and the old one is not left valid.
//
// Throttling (devices.go) counts the attempt before the password is checked, so requests sent at the same time
// cannot all pass a wait that has not been recorded yet; a successful sign-in clears every count for the account.
// Its responses do not reveal whether the account exists. A sign-in from a client without a trusted device for the
// account trusts it from now on (Session.Device).
func (s *Service) SignIn(ctx context.Context, account, password, previousToken string, client Client) (User, Session, error) {
	u, sess, err := s.signIn(ctx, account, password, previousToken, client)
	if err == nil {
		s.logger.InfoContext(ctx, "signed in", "event", "auth.sign_in", "user_id", u.ID)
	}
	return u, sess, err
}

// signInFailed logs a refused sign-in: the reason and, when the account exists, its id. The submitted name is not
// logged: it can be a password typed into the wrong field.
func (s *Service) signInFailed(ctx context.Context, reason string, userID *uuid.UUID) {
	attrs := []any{"event", "auth.sign_in_failed", "reason", reason}
	if userID != nil {
		attrs = append(attrs, "user_id", *userID)
	}
	s.logger.WarnContext(ctx, "sign-in refused", attrs...)
}

func (s *Service) signIn(ctx context.Context, account, password, previousToken string, client Client) (User, Session, error) {
	key := accountKey(account)
	now := s.now()
	deviceID, trusted, err := s.trustedDevice(ctx, client.DeviceToken, key, now)
	if err != nil {
		return User{}, Session{}, err
	}
	if err := s.throttle(ctx, key, client.Address, deviceID, trusted, now); err != nil {
		return User{}, Session{}, err
	}
	u, err := s.verifyCredentials(ctx, key, password)
	if err != nil {
		return User{}, Session{}, err
	}
	var sess Session
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.ClearLoginThrottles(ctx, key); err != nil {
			return fmt.Errorf("auth: clear throttles: %w", err)
		}
		if previousToken != "" {
			if err := q.DeleteSession(ctx, hashToken(previousToken)); err != nil {
				return fmt.Errorf("auth: delete previous session: %w", err)
			}
		}
		if sess, err = s.newSession(ctx, q, u.ID); err != nil {
			return err
		}
		if !trusted {
			sess.Device, err = s.newDevice(ctx, q, u.ID)
		}
		return err
	})
	return u, sess, err
}

// throttle counts the attempt for its client (the trusted device, else the address) and, from a client the account
// does not trust, for the account as a whole; it refuses the attempt while either limit holds.
func (s *Service) throttle(ctx context.Context, key, address string, deviceID uuid.UUID, trusted bool, now time.Time) error {
	clientKey := "ip:" + address
	if trusted {
		clientKey = "device:" + deviceID.String()
	}
	if err := s.reserveAttempt(ctx, key, clientKey, now); err != nil {
		if e, ok := apperr.As(err); ok && e.Kind == apperr.RateLimited {
			s.signInFailed(ctx, "throttled", nil)
		}
		return err
	}
	if trusted {
		return nil
	}
	n, err := s.q().CountUntrustedAttempt(ctx, store.CountUntrustedAttemptParams{AccountKey: key, Now: now})
	if err != nil {
		return fmt.Errorf("auth: count attempt: %w", err)
	}
	if n > untrustedFailureLimit {
		s.signInFailed(ctx, "account_locked", nil)
		return errAccountLocked
	}
	return nil
}

// verifyCredentials returns the enabled user whose account key and password match. An unknown account costs the
// same hash as a wrong password, and every refusal is the same errInvalidCredentials.
func (s *Service) verifyCredentials(ctx context.Context, key, password string) (User, error) {
	normalized := normalizePassword(password)
	row, err := s.q().GetLoginUser(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := s.hasher.Verify(ctx, normalized, s.dummyHash); err != nil {
			return User{}, err
		}
		s.signInFailed(ctx, "unknown_account", nil)
		return User{}, errInvalidCredentials
	}
	if err != nil {
		return User{}, fmt.Errorf("auth: get user: %w", err)
	}
	ok, err := s.hasher.Verify(ctx, normalized, row.PasswordHash)
	if err != nil {
		return User{}, err
	}
	if !ok || row.IsDisabled {
		reason := "invalid_credentials"
		if ok {
			reason = "disabled"
		}
		s.signInFailed(ctx, reason, &row.ID)
		return User{}, errInvalidCredentials
	}
	return User{ID: row.ID, Account: row.Account, Role: row.Role, IsDisabled: row.IsDisabled, CreatedAt: row.CreatedAt}, nil
}

func (s *Service) reserveAttempt(ctx context.Context, key, clientKey string, now time.Time) error {
	_, err := s.q().ReserveLoginAttempt(ctx, store.ReserveLoginAttemptParams{AccountKey: key, ClientKey: clientKey, Now: now})
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("auth: reserve login attempt: %w", err)
	}
	wait := time.Second
	next, err := s.q().GetLoginThrottle(ctx, store.GetLoginThrottleParams{AccountKey: key, ClientKey: clientKey})
	if err == nil && next != nil && next.Sub(now) > wait {
		wait = next.Sub(now)
	}
	return &apperr.Error{Kind: apperr.RateLimited, Code: CodeLoginThrottled,
		Detail: "too many failed sign-ins from this client", RetryAfter: wait}
}

// SignOut deletes the session token belongs to; an unknown or empty token is not an error.
func (s *Service) SignOut(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if err := s.q().DeleteSession(ctx, hashToken(token)); err != nil {
		return fmt.Errorf("auth: delete session: %w", err)
	}
	attrs := []any{"event", "auth.sign_out"}
	if c, ok := authctx.From(ctx); ok {
		attrs = append(attrs, "user_id", c.ID)
	}
	s.logger.InfoContext(ctx, "signed out", attrs...)
	return nil
}

// endEverywhere deletes every session and trusted device of the user: signing out everywhere, disabling and
// recovering all mean that no browser keeps access or trust.
func endEverywhere(ctx context.Context, q *store.Queries, userID uuid.UUID) error {
	if err := q.DeleteUserSessions(ctx, userID); err != nil {
		return fmt.Errorf("auth: delete sessions: %w", err)
	}
	if err := q.DeleteUserTrustedDevices(ctx, userID); err != nil {
		return fmt.Errorf("auth: forget devices: %w", err)
	}
	return nil
}

// SignOutAll deletes every session of the user and forgets the user's trusted devices.
func (s *Service) SignOutAll(ctx context.Context, userID uuid.UUID) error {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return endEverywhere(ctx, store.New(tx), userID)
	})
	if err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "signed out everywhere", "event", "auth.sign_out_all", "user_id", userID)
	return nil
}

var errSessionNotFound = apperr.New(apperr.NotFound, "", "session not found")

// Sessions returns up to limit of the caller's live sessions after cursor (nil for the first page).
func (s *Service) Sessions(ctx context.Context, limit int, cursor *string) (SessionPage, error) {
	c, err := caller(ctx)
	if err != nil {
		return SessionPage{}, err
	}
	after, err := httpx.After(cursor)
	if err != nil {
		return SessionPage{}, err
	}
	now := s.now()
	rows, err := s.q().ListUserSessions(ctx, store.ListUserSessionsParams{
		UserID: c.ID, Now: now, IdleBefore: now.Add(-s.cfg.SessionIdle), After: after, RowLimit: httpx.PageRows(limit),
	})
	if err != nil {
		return SessionPage{}, fmt.Errorf("auth: list sessions: %w", err)
	}
	rows, next := httpx.TrimPage(rows, limit, func(r store.ListUserSessionsRow) uuid.UUID { return r.ID })
	page := SessionPage{Sessions: make([]SessionInfo, 0, len(rows)), NextCursor: next}
	for _, row := range rows {
		page.Sessions = append(page.Sessions, SessionInfo(row))
	}
	return page, nil
}

// Session returns the caller's live session with id. Another user's session is not found, the same as one that
// does not exist or has expired (docs/adr/2026-10-18-api-and-auth-baseline.md D7).
func (s *Service) Session(ctx context.Context, id uuid.UUID) (SessionInfo, error) {
	c, err := caller(ctx)
	if err != nil {
		return SessionInfo{}, err
	}
	now := s.now()
	row, err := s.q().GetUserSession(ctx, store.GetUserSessionParams{
		ID: id, UserID: c.ID, Now: now, IdleBefore: now.Add(-s.cfg.SessionIdle),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionInfo{}, errSessionNotFound
	}
	if err != nil {
		return SessionInfo{}, fmt.Errorf("auth: get session: %w", err)
	}
	return SessionInfo(row), nil
}

// RevokeSession deletes the caller's live session with id, with the same not-found rule as Session.
func (s *Service) RevokeSession(ctx context.Context, id uuid.UUID) error {
	c, err := caller(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	n, err := s.q().DeleteUserSession(ctx, store.DeleteUserSessionParams{
		ID: id, UserID: c.ID, Now: now, IdleBefore: now.Add(-s.cfg.SessionIdle),
	})
	if err != nil {
		return fmt.Errorf("auth: revoke session: %w", err)
	}
	if n == 0 {
		return errSessionNotFound
	}
	return nil
}

// CleanupCounts is how many rows Cleanup deleted.
type CleanupCounts struct {
	Sessions, Throttles, Devices int64
}

// Cleanup deletes expired sessions and trusted devices and sign_in_throttles rows untouched for a day; serve runs it
// hourly.
func (s *Service) Cleanup(ctx context.Context) (CleanupCounts, error) {
	now := s.now()
	q := s.q()
	var c CleanupCounts
	var err error
	if c.Sessions, err = q.DeleteExpiredSessions(ctx, store.DeleteExpiredSessionsParams{
		Now: now, IdleBefore: now.Add(-s.cfg.SessionIdle),
	}); err != nil {
		return c, fmt.Errorf("auth: delete expired sessions: %w", err)
	}
	if c.Throttles, err = q.DeleteStaleLoginThrottles(ctx, now.Add(-throttleTTL)); err != nil {
		return c, fmt.Errorf("auth: delete stale throttles: %w", err)
	}
	if c.Devices, err = q.DeleteExpiredTrustedDevices(ctx, now); err != nil {
		return c, fmt.Errorf("auth: delete expired devices: %w", err)
	}
	return c, nil
}
