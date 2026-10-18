// Package auth owns accounts, sessions and login throttling: the users, sessions and login_throttles tables
// (schema.sql) and the queries over them (queries/, generated into internal/store by sqlc).
//
// Sessions are opaque (plan P6): the cookie holds 32 random bytes, the database only their SHA-256, and every
// request reads the session and its user, so signing out, disabling an account or changing a role takes effect on
// the next request. Passwords follow NIST SP 800-63B-4 (plan P7): normalized with NFKC, checked against a list,
// stored as argon2id.
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
)

// User is an account as the API shows it.
type User struct {
	ID         uuid.UUID
	Account    string
	Role       string
	IsDisabled bool
	CreatedAt  time.Time
}

// Session is a newly issued session: the token goes in the cookie and is never stored.
type Session struct {
	Token     string
	ExpiresAt time.Time
	// MaxAge is the absolute lifetime, for the cookie's Max-Age.
	MaxAge time.Duration
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
	// Now defaults to time.Now.
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
	s := &Service{pool: pool, cfg: cfg, hasher: cfg.Hasher, now: cfg.Now, logger: cfg.Logger}
	if s.hasher == nil {
		s.hasher = NewArgon2Hasher(DefaultArgon2Params, 0)
	}
	if s.now == nil {
		s.now = time.Now
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

// User returns the user with id.
func (s *Service) User(ctx context.Context, id uuid.UUID) (User, error) {
	row, err := s.q().GetUser(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, apperr.New(apperr.NotFound, "", "user not found")
	}
	if err != nil {
		return User{}, fmt.Errorf("auth: get user: %w", err)
	}
	return User(row), nil
}

// SetDisabled disables or re-enables a user. Disabling deletes all of the user's sessions in the same transaction,
// so re-enabling later does not bring them back.
func (s *Service) SetDisabled(ctx context.Context, id uuid.UUID, disabled bool) (User, error) {
	var u User
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.SetUserDisabled(ctx, store.SetUserDisabledParams{ID: id, IsDisabled: disabled})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.New(apperr.NotFound, "", "user not found")
		}
		if err != nil {
			return fmt.Errorf("auth: set disabled: %w", err)
		}
		u = User(row)
		if !disabled {
			return nil
		}
		if err := q.DeleteUserSessions(ctx, id); err != nil {
			return fmt.Errorf("auth: revoke sessions: %w", err)
		}
		return nil
	})
	return u, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
