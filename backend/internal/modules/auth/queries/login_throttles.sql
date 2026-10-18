-- ReserveLoginAttempt counts an attempt as a failure before the password is checked, in one statement: the row lock
-- serializes concurrent attempts, so they cannot all pass while the account is waiting. It returns no row while
-- next_allowed_at is in the future. From the 5th consecutive failure on, the next attempt waits 2^(failures - 5)
-- minutes, at most an hour (the exponent is capped first so power() cannot overflow). Inventory B6; OWASP
-- Authentication Cheat Sheet.
-- name: ReserveLoginAttempt :one
INSERT INTO login_throttles AS t (account_key, failures, next_allowed_at, updated_at)
VALUES (sqlc.arg(account_key), 1, NULL, sqlc.arg(now))
ON CONFLICT (account_key) DO UPDATE
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
FROM login_throttles
WHERE account_key = @account_key;

-- name: ClearLoginThrottle :exec
DELETE FROM login_throttles
WHERE account_key = @account_key;

-- name: DeleteStaleLoginThrottles :execrows
DELETE FROM login_throttles
WHERE updated_at <= @before;
