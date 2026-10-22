// Package auth owns accounts, sessions and sign-in throttling: the users, sessions, sign_in_throttles,
// trusted_devices and audit_events tables
// (schema.sql) and the queries over them (queries/, generated into internal/store by sqlc).
//
// Sessions are opaque (docs/adr/2026-10-18-api-and-auth-baseline.md D4): the cookie holds 32 random bytes, the
// database only their SHA-256, and every request reads the session and its user, so signing out, disabling an account
// or changing a role takes effect on the next request. Passwords follow NIST SP 800-63B-4: normalized with NFKC,
// checked against a list, stored as argon2id.
package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Will413028/fullstack-template/backend/internal/modules/auth/blocklist"
	"github.com/Will413028/fullstack-template/backend/internal/modules/auth/internal/store"
	"github.com/Will413028/fullstack-template/backend/internal/platform/apperr"
	"github.com/Will413028/fullstack-template/backend/internal/platform/authctx"
	"github.com/Will413028/fullstack-template/backend/internal/platform/httpx"
	"github.com/Will413028/fullstack-template/backend/internal/platform/logx"
)

// Roles.
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// Codes clients translate (apperr.Error.Code); changing one is a breaking change.
//
//nolint:gosec // error codes, not credentials
const (
	CodeAccountTaken       = "account_taken"
	CodePasswordTooCommon  = "password_too_common"
	CodeInvalidCredentials = "invalid_credentials"
	CodeLoginThrottled     = "login_throttled"
	CodeLastAdmin          = "last_admin"
	// CodeReauthenticationRequired asks the client to sign in again before a sensitive operation.
	CodeReauthenticationRequired = "reauthentication_required"
)

// User is an account as the API shows it.
type User struct {
	ID         uuid.UUID
	Account    string
	Role       string
	IsDisabled bool
	CreatedAt  time.Time
}

// SessionInfo is a session as the API shows it. ID is its public identifier; the token is never part of it.
type SessionInfo struct {
	ID         uuid.UUID
	CreatedAt  time.Time
	LastUsedAt time.Time
	ExpiresAt  time.Time
}

// Session is a newly issued session: the token goes in the cookie and is never stored.
type Session struct {
	SessionInfo
	Token string
	// MaxAge is the absolute lifetime, for the cookie's Max-Age.
	MaxAge time.Duration
	// Device is set when the sign-in trusted the browser for the first time; it goes in the device cookie.
	Device *Device
}

// UserPage is one page of users, newest first. NextCursor is empty on the last page.
type UserPage struct {
	Users      []User
	NextCursor string
}

// SessionPage is one page of sessions, newest first. NextCursor is empty on the last page.
type SessionPage struct {
	Sessions   []SessionInfo
	NextCursor string
}

// BreachChecker reports whether a password appears in known breaches (Have I Been Pwned). It lives outside the
// module because it speaks HTTP (internal/platform/hibp).
type BreachChecker interface {
	Breached(ctx context.Context, password string) (bool, error)
}

// Config configures the service.
type Config struct {
	SessionAbsolute time.Duration
	SessionIdle     time.Duration
	// Hasher defaults to argon2id with DefaultArgon2Params, at most runtime.NumCPU() at once.
	Hasher PasswordHasher
	// Breach is the optional breached-password check (PASSWORD_HIBP_CHECK); nil skips it.
	Breach BreachChecker
	// Now is the clock for sessions and sign-in throttling; the composition root passes time.Now.
	Now    func() time.Time
	Logger *slog.Logger
}

// Service implements accounts and sessions.
type Service struct {
	pool   *pgxpool.Pool
	cfg    Config
	hasher PasswordHasher
	now    func() time.Time
	logger *slog.Logger
	// dummyHash is verified against when the account does not exist, so an unknown account costs the same hash as
	// a wrong password and the response time does not reveal which it was.
	dummyHash string
}

// New returns the service. It hashes one random password to build the dummy hash.
func New(ctx context.Context, pool *pgxpool.Pool, cfg Config) (*Service, error) {
	if cfg.SessionAbsolute <= 0 || cfg.SessionIdle <= 0 || cfg.SessionIdle > cfg.SessionAbsolute {
		return nil, errors.New("auth: session lifetimes must be positive and idle must not exceed absolute")
	}
	if cfg.Now == nil {
		return nil, errors.New("auth: Now is required")
	}
	s := &Service{pool: pool, cfg: cfg, hasher: cfg.Hasher, now: cfg.Now, logger: cfg.Logger}
	if s.hasher == nil {
		s.hasher = NewArgon2Hasher(DefaultArgon2Params, 0)
	}
	if s.logger == nil {
		s.logger = slog.New(slog.DiscardHandler)
	}
	dummy, err := s.hasher.Hash(ctx, rand.Text())
	if err != nil {
		return nil, fmt.Errorf("auth: dummy hash: %w", err)
	}
	s.dummyHash = dummy
	return s, nil
}

func (s *Service) q() *store.Queries { return store.New(s.pool) }

// accountKey is how accounts are stored and looked up: lowercase, so uniqueness and sign-in ignore case.
func accountKey(account string) string { return strings.ToLower(account) }

var errInvalidCredentials = apperr.New(apperr.Unauthenticated, CodeInvalidCredentials, "account or password is incorrect")

// checkNewPassword applies the rules the contract cannot express: the list and, when enabled, the breach check.
// A breach-check failure lets the password through: an outage of the external service must not stop sign-ups.
func (s *Service) checkNewPassword(ctx context.Context, normalized string) error {
	tooCommon := apperr.Invalid("/password", CodePasswordTooCommon, "password is on the list of common or breached passwords")
	if blocklist.Contains(normalized) {
		return tooCommon
	}
	if s.cfg.Breach == nil {
		return nil
	}
	breached, err := s.cfg.Breach.Breached(ctx, normalized)
	if err != nil {
		s.logger.WarnContext(ctx, "breached-password check failed; accepting the password", "error", err)
		return nil
	}
	if breached {
		return tooCommon
	}
	return nil
}

// newPasswordHash checks and hashes a new password. Hashing happens outside any transaction: it takes tens of
// milliseconds and must not hold a connection.
func (s *Service) newPasswordHash(ctx context.Context, password string) (string, error) {
	normalized := normalizePassword(password)
	if err := s.checkNewPassword(ctx, normalized); err != nil {
		return "", err
	}
	return s.hasher.Hash(ctx, normalized)
}

// CreateUser creates a user account and signs it in, in one transaction.
func (s *Service) CreateUser(ctx context.Context, account, password string) (User, Session, error) {
	hash, err := s.newPasswordHash(ctx, password)
	if err != nil {
		return User{}, Session{}, err
	}
	var u User
	var sess Session
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.CreateUser(ctx, store.CreateUserParams{Account: accountKey(account), PasswordHash: hash, Role: RoleUser})
		if isUniqueViolation(err) {
			return apperr.New(apperr.Conflict, CodeAccountTaken, "account is taken")
		}
		if err != nil {
			return fmt.Errorf("auth: create user: %w", err)
		}
		u = User(row)
		sess, err = s.newSession(ctx, q, u.ID)
		return err
	})
	if err == nil {
		s.logger.InfoContext(ctx, "signed up", "event", "auth.sign_up", "user_id", u.ID)
	}
	return u, sess, err
}

// EnsureAdmin creates an admin account unless the account exists, in which case nothing changes and created is
// false. The password goes through the same rules as CreateUser.
func (s *Service) EnsureAdmin(ctx context.Context, account, password string) (created bool, err error) {
	hash, err := s.newPasswordHash(ctx, password)
	if err != nil {
		return false, err
	}
	_, err = s.q().CreateUserIfAbsent(ctx, store.CreateUserIfAbsentParams{
		Account: accountKey(account), PasswordHash: hash, Role: RoleAdmin,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("auth: create admin: %w", err)
	}
	return true, nil
}

var errUserNotFound = apperr.New(apperr.NotFound, "", "user not found")

// ErrNoSuchAdmin is returned by RecoverAdmin for an account that does not exist or is not an admin.
var ErrNoSuchAdmin = errors.New("auth: no admin account with that name")

// RecoverAdmin is the break-glass path for an admin who is disabled, locked out or has lost the password: it sets a
// new password, enables the account, deletes its sessions and clears its sign-in throttle. It changes only an
// existing admin; it never creates or promotes an account.
func (s *Service) RecoverAdmin(ctx context.Context, account, password string) error {
	hash, err := s.newPasswordHash(ctx, password)
	if err != nil {
		return err
	}
	key := accountKey(account)
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		id, err := q.RecoverAdmin(ctx, store.RecoverAdminParams{Account: key, PasswordHash: hash})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoSuchAdmin
		}
		if err != nil {
			return fmt.Errorf("auth: recover admin: %w", err)
		}
		if err := endEverywhere(ctx, q, id); err != nil {
			return err
		}
		if err := q.ClearLoginThrottles(ctx, key); err != nil {
			return fmt.Errorf("auth: clear throttles: %w", err)
		}
		return s.audit(ctx, q, AuditAdminRecovered, id)
	})
}

// Audit actions (audit_events.action); a new one also goes into the table's CHECK.
const (
	AuditUserDisabled   = "user.disabled"
	AuditUserEnabled    = "user.enabled"
	AuditUserDeleted    = "user.deleted"
	AuditUserUnlocked   = "user.unlocked"
	AuditAdminRecovered = "admin.recovered"
)

// audit records action on target in q's transaction, with the signed-in caller as the actor (none for a command run
// on the host) and the request id when there is one.
func (s *Service) audit(ctx context.Context, q *store.Queries, action string, target uuid.UUID) error {
	var actor *uuid.UUID
	if c, ok := authctx.From(ctx); ok {
		actor = &c.ID
	}
	var requestID *string
	if id := logx.RequestID(ctx); id != "" {
		requestID = &id
	}
	err := q.InsertAuditEvent(ctx, store.InsertAuditEventParams{
		At: s.now(), ActorID: actor, Action: action, TargetID: target, RequestID: requestID,
	})
	if err != nil {
		return fmt.Errorf("auth: audit %s: %w", action, err)
	}
	return nil
}

// ErrLastAdmin refuses to disable or delete the only enabled admin: nobody could administer users afterwards, and
// seed-admin does not change an existing account.
var ErrLastAdmin = apperr.New(apperr.Conflict, CodeLastAdmin, "the only enabled admin cannot be disabled or deleted")

// keepAnAdmin fails with ErrLastAdmin when id is the only enabled admin. It locks the enabled admins until q's
// transaction ends.
func keepAnAdmin(ctx context.Context, q *store.Queries, id uuid.UUID) error {
	admins, err := q.LockEnabledAdmins(ctx)
	if err != nil {
		return fmt.Errorf("auth: lock admins: %w", err)
	}
	if len(admins) == 1 && admins[0] == id {
		return ErrLastAdmin
	}
	return nil
}

// caller is the signed-in user. The HTTP layer requires a session for every operation that calls a method using it,
// so a missing user is a wiring bug, not a client error.
func caller(ctx context.Context) (authctx.User, error) {
	u, ok := authctx.From(ctx)
	if !ok {
		return authctx.User{}, errors.New("auth: no authenticated user in context")
	}
	return u, nil
}

// User returns the user with id to that user or to an admin. Anyone else gets not found, the same as for an id that
// does not exist, so ids reveal nothing about other accounts (docs/adr/2026-10-18-api-and-auth-baseline.md D7).
func (s *Service) User(ctx context.Context, id uuid.UUID) (User, error) {
	c, err := caller(ctx)
	if err != nil {
		return User{}, err
	}
	if c.ID != id && c.Role != RoleAdmin {
		return User{}, errUserNotFound
	}
	row, err := s.q().GetUser(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, errUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("auth: get user: %w", err)
	}
	return User(row), nil
}

// ListUsers returns up to limit users, disabled ones included, after cursor (nil for the first page). Only admins
// may call it; the HTTP layer enforces that before the call (requireRole), as for SetDisabled.
func (s *Service) ListUsers(ctx context.Context, limit int, cursor *string) (UserPage, error) {
	after, err := httpx.After(cursor)
	if err != nil {
		return UserPage{}, err
	}
	rows, err := s.q().ListUsers(ctx, store.ListUsersParams{After: after, RowLimit: httpx.PageRows(limit)})
	if err != nil {
		return UserPage{}, fmt.Errorf("auth: list users: %w", err)
	}
	rows, next := httpx.TrimPage(rows, limit, func(r store.ListUsersRow) uuid.UUID { return r.ID })
	page := UserPage{Users: make([]User, 0, len(rows)), NextCursor: next}
	for _, row := range rows {
		page.Users = append(page.Users, User(row))
	}
	return page, nil
}

// SetDisabled disables or re-enables a user. Disabling deletes all of the user's sessions in the same transaction,
// so re-enabling later does not bring them back, and is refused for the only enabled admin (ErrLastAdmin).
func (s *Service) SetDisabled(ctx context.Context, id uuid.UUID, disabled bool) (User, error) {
	var u User
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if disabled {
			if err := keepAnAdmin(ctx, q, id); err != nil {
				return err
			}
		}
		row, err := q.SetUserDisabled(ctx, store.SetUserDisabledParams{ID: id, IsDisabled: disabled})
		if errors.Is(err, pgx.ErrNoRows) {
			return errUserNotFound
		}
		if err != nil {
			return fmt.Errorf("auth: set disabled: %w", err)
		}
		u = User(row)
		if !disabled {
			return s.audit(ctx, q, AuditUserEnabled, id)
		}
		if err := endEverywhere(ctx, q, id); err != nil {
			return err
		}
		return s.audit(ctx, q, AuditUserDisabled, id)
	})
	return u, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// ReauthenticateWithin is how recently the session must have signed in for an operation that cannot be undone, such
// as deleting the account: a stolen or forgotten session alone is not enough (OWASP ASVS 5.0 V6, re-authentication
// before sensitive operations).
const ReauthenticateWithin = 10 * time.Minute

// ErrReauthenticationRequired asks the client to sign in again (POST /sessions) and retry.
var ErrReauthenticationRequired = apperr.New(apperr.Forbidden, CodeReauthenticationRequired,
	"sign in again within the last 10 minutes to do this")

// DeleteOwnAccount deletes the caller's account and everything it owns. The session the request carries
// (sessionID) must have been issued within ReauthenticateWithin, and the only enabled admin cannot delete itself.
func (s *Service) DeleteOwnAccount(ctx context.Context, sessionID uuid.UUID) error {
	c, err := s.recentlySignedIn(ctx, sessionID)
	if err != nil {
		return err
	}
	return s.deleteUser(ctx, c.ID)
}

// DeleteUser deletes a user and everything it owns; admins call it (internal/httpapi requireRole). Like
// DeleteOwnAccount it needs a session issued within ReauthenticateWithin, and the only enabled admin cannot be
// deleted.
func (s *Service) DeleteUser(ctx context.Context, sessionID, id uuid.UUID) error {
	if _, err := s.recentlySignedIn(ctx, sessionID); err != nil {
		return err
	}
	return s.deleteUser(ctx, id)
}

// recentlySignedIn returns the caller when sessionID, the caller's current session, was issued within
// ReauthenticateWithin: an operation that cannot be undone needs the password again, not just a session.
func (s *Service) recentlySignedIn(ctx context.Context, sessionID uuid.UUID) (authctx.User, error) {
	c, err := caller(ctx)
	if err != nil {
		return authctx.User{}, err
	}
	created, err := s.q().GetSessionCreatedAt(ctx, store.GetSessionCreatedAtParams{ID: sessionID, UserID: c.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return authctx.User{}, ErrSessionInvalid
	}
	if err != nil {
		return authctx.User{}, fmt.Errorf("auth: get session: %w", err)
	}
	if s.now().Sub(created) > ReauthenticateWithin {
		return authctx.User{}, ErrReauthenticationRequired
	}
	return c, nil
}

func (s *Service) deleteUser(ctx context.Context, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := keepAnAdmin(ctx, q, id); err != nil {
			return err
		}
		n, err := q.DeleteUser(ctx, id)
		if err != nil {
			return fmt.Errorf("auth: delete user: %w", err)
		}
		if n == 0 {
			return errUserNotFound
		}
		return s.audit(ctx, q, AuditUserDeleted, id)
	})
}
