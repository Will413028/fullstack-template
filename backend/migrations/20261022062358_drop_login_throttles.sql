-- Contract step (docs/rules/database-design.md): the backend stopped using login_throttles in PR #26 (commit 0fdc95f),
-- which moved sign-in throttling to sign_in_throttles; that release is on main, so no running backend reads it.
-- squawk-ignore ban-drop-table
DROP TABLE "login_throttles";
