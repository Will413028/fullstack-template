package auth

import (
	"errors"
	"fmt"
	"regexp"
	"unicode/utf8"

	"github.com/Will413028/template-go-nextjs/backend/internal/platform/apperr"
)

// Limits on account names and passwords (decision D2 in docs/adr/2026-11-03-api-contract-protobuf-connect.md: limits
// live in the module, so sign-up over HTTP, seed-admin and recover-admin apply the same ones). Lengths count code
// points. users.account's CHECK in schema.sql holds the same account length.
const (
	minAccount  = 3
	maxAccount  = 50
	minPassword = 15 // NIST SP 800-63B-4: at least 15 when the password is the only factor
	maxPassword = 128
)

// accountPattern is the characters an account name may use.
var accountPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// checkNewAccount checks the account name and password of an account being created or recovered. Every invalid
// field is reported (apperr.Fields), as account and password.
func checkNewAccount(account, password string) error {
	var errs []error
	switch n := utf8.RuneCountInString(account); {
	case n == 0:
		errs = append(errs, apperr.Invalid("account", "required", "account is required"))
	case n < minAccount:
		errs = append(errs, apperr.Invalid("account", "too_short", fmt.Sprintf("account must be at least %d characters", minAccount)))
	case n > maxAccount:
		errs = append(errs, apperr.Invalid("account", "too_long", fmt.Sprintf("account must be at most %d characters", maxAccount)))
	case !accountPattern.MatchString(account):
		errs = append(errs, apperr.Invalid("account", "pattern", "account may use only letters, digits, _ . and -"))
	}
	switch n := utf8.RuneCountInString(password); {
	case n == 0:
		errs = append(errs, apperr.Invalid("password", "required", "password is required"))
	case n < minPassword:
		errs = append(errs, apperr.Invalid("password", "too_short", fmt.Sprintf("password must be at least %d characters", minPassword)))
	case n > maxPassword:
		errs = append(errs, apperr.Invalid("password", "too_long", fmt.Sprintf("password must be at most %d characters", maxPassword)))
	}
	return errors.Join(errs...)
}

// checkSignIn checks the input of a sign-in: present and within the longest account and password that can exist, so
// a rule tightened later does not lock out an account created before it.
func checkSignIn(account, password string) error {
	var errs []error
	switch n := utf8.RuneCountInString(account); {
	case n == 0:
		errs = append(errs, apperr.Invalid("account", "required", "account is required"))
	case n > maxAccount:
		errs = append(errs, apperr.Invalid("account", "too_long", fmt.Sprintf("account must be at most %d characters", maxAccount)))
	}
	switch n := utf8.RuneCountInString(password); {
	case n == 0:
		errs = append(errs, apperr.Invalid("password", "required", "password is required"))
	case n > maxPassword:
		errs = append(errs, apperr.Invalid("password", "too_long", fmt.Sprintf("password must be at most %d characters", maxPassword)))
	}
	return errors.Join(errs...)
}
