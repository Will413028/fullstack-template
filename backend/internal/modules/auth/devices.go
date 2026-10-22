package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Will413028/fullstack-template/backend/internal/modules/auth/internal/store"
	"github.com/Will413028/fullstack-template/backend/internal/platform/apperr"
)

// Sign-in throttling beyond one client (docs/adr/2026-10-22-account-abuse-and-lifecycle.md D4). Each client, an
// address or a trusted device, has its own backoff (ReserveLoginAttempt). Clients without a trusted device for the
// account also share one count; from untrustedFailureLimit on they are refused until a sign-in succeeds or an admin
// unlocks the account, while the owner's trusted devices still get in (OWASP "Slow Down Online Guessing Attacks with
// Device Cookies").

// untrustedFailureLimit is NIST SP 800-63B 3.2.2's ceiling of consecutive failures.
const untrustedFailureLimit = 100

// deviceTTL is how long a browser stays trusted after it signed in.
const deviceTTL = 180 * 24 * time.Hour

// errAccountLocked refuses a client the account does not trust once untrustedFailureLimit is reached. It reads the
// same as a per-client wait, so it does not tell an attacker which limit they hit; Retry-After is a hint, since the
// lock lasts until a sign-in succeeds, an admin unlocks it, or a day passes without attempts.
var errAccountLocked = &apperr.Error{Kind: apperr.RateLimited, Code: CodeLoginThrottled,
	Detail: "too many failed sign-ins for this account", RetryAfter: time.Hour}

// Client is where a sign-in attempt comes from.
type Client struct {
	// Address identifies the client by network address; the HTTP layer passes the IPv4 address or the IPv6 /64.
	Address string
	// DeviceToken is the device cookie the request carried, "" without one.
	DeviceToken string
}

// Device is a newly trusted browser: the token goes in the device cookie and is never stored.
type Device struct {
	Token  string
	MaxAge time.Duration
}

// trustedDevice returns the id of the device token belongs to when it is unexpired and belongs to the account key.
func (s *Service) trustedDevice(ctx context.Context, token, key string, now time.Time) (uuid.UUID, bool, error) {
	if token == "" {
		return uuid.Nil, false, nil
	}
	row, err := s.q().GetTrustedDevice(ctx, store.GetTrustedDeviceParams{TokenHash: hashToken(token), Now: now})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("auth: get trusted device: %w", err)
	}
	return row.ID, row.Account == key, nil
}

func (s *Service) newDevice(ctx context.Context, q *store.Queries, userID uuid.UUID) (*Device, error) {
	now := s.now()
	token := newToken()
	if err := q.InsertTrustedDevice(ctx, store.InsertTrustedDeviceParams{
		TokenHash: hashToken(token), UserID: userID, Now: now, ExpiresAt: now.Add(deviceTTL),
	}); err != nil {
		return nil, fmt.Errorf("auth: trust device: %w", err)
	}
	return &Device{Token: token, MaxAge: deviceTTL}, nil
}

// SignInLock is whether an account refuses sign-ins from clients it does not trust.
type SignInLock struct {
	Locked   bool
	Failures int64
}

// SignInLock returns the lock state of the user's account; admins call it.
func (s *Service) SignInLock(ctx context.Context, userID uuid.UUID) (SignInLock, error) {
	u, err := s.q().GetUser(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return SignInLock{}, errUserNotFound
	}
	if err != nil {
		return SignInLock{}, fmt.Errorf("auth: get user: %w", err)
	}
	n, err := s.q().GetUntrustedFailures(ctx, u.Account)
	if errors.Is(err, pgx.ErrNoRows) {
		return SignInLock{}, nil
	}
	if err != nil {
		return SignInLock{}, fmt.Errorf("auth: get failures: %w", err)
	}
	return SignInLock{Locked: n >= untrustedFailureLimit, Failures: n}, nil
}

// UnlockSignIn forgets every failed sign-in counted for the user's account, from all clients; admins call it.
func (s *Service) UnlockSignIn(ctx context.Context, userID uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		u, err := q.GetUser(ctx, userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errUserNotFound
		}
		if err != nil {
			return fmt.Errorf("auth: get user: %w", err)
		}
		if err := q.ClearLoginThrottles(ctx, u.Account); err != nil {
			return fmt.Errorf("auth: clear throttles: %w", err)
		}
		return s.audit(ctx, q, AuditUserUnlocked, userID)
	})
}
