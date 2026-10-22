-- ReserveLoginAttempt counts an attempt from one client as a failure before the password is checked, in one
-- statement: the row lock serializes concurrent attempts, so they cannot all pass while the client is waiting. It
-- returns no row while next_allowed_at is in the future. From the 5th consecutive failure on, the next attempt waits
-- 2^(failures - 5) minutes, at most an hour (the exponent is capped first so power() cannot overflow).
-- docs/adr/2026-10-18-api-and-auth-baseline.md D6; OWASP Authentication Cheat Sheet.
-- name: ReserveLoginAttempt :one
INSERT INTO sign_in_throttles AS t (account_key, client_key, failures, next_allowed_at, updated_at)
VALUES (sqlc.arg(account_key), sqlc.arg(client_key), 1, NULL, sqlc.arg(now))
ON CONFLICT (account_key, client_key) DO UPDATE
SET failures = t.failures + 1,
    next_allowed_at = CASE
      WHEN t.failures + 1 >= 5 THEN excluded.updated_at
        + make_interval(mins => LEAST(power(2, LEAST(t.failures + 1 - 5, 6))::integer, 60))
    END,
    updated_at = excluded.updated_at
WHERE t.next_allowed_at IS NULL OR t.next_allowed_at <= excluded.updated_at
RETURNING failures;

-- name: GetLoginThrottle :one
SELECT next_allowed_at
FROM sign_in_throttles
WHERE account_key = @account_key AND client_key = @client_key;

-- CountUntrustedAttempt counts an attempt from a client without a trusted device for the account, across all
-- clients (the row with client_key ''), and returns the count including this one.
-- name: CountUntrustedAttempt :one
INSERT INTO sign_in_throttles AS t (account_key, client_key, failures, next_allowed_at, updated_at)
VALUES (@account_key, '', 1, NULL, @now)
ON CONFLICT (account_key, client_key) DO UPDATE
SET failures = t.failures + 1, updated_at = excluded.updated_at
RETURNING failures;

-- GetUntrustedFailures is the account-wide count CountUntrustedAttempt keeps; no row means none.
-- name: GetUntrustedFailures :one
SELECT failures
FROM sign_in_throttles
WHERE account_key = @account_key AND client_key = '';

-- ClearLoginThrottles forgets every count for the account: after a successful sign-in, an admin unlock or recovery.
-- name: ClearLoginThrottles :exec
DELETE FROM sign_in_throttles
WHERE account_key = @account_key;

-- name: DeleteStaleLoginThrottles :execrows
DELETE FROM sign_in_throttles
WHERE updated_at <= @before;
