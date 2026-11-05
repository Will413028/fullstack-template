package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/Will413028/template-go-nextjs/backend/internal/modules/auth"
	"github.com/Will413028/template-go-nextjs/backend/internal/platform/apperr"
)

// The environment variable each account field comes from, for error messages.
var adminInputVars = map[string]string{"account": "ADMIN_ACCOUNT", "password": "ADMIN_PASSWORD"}

// seedAdmin creates the admin account unless it exists; it is the only way to create one. The auth service checks the
// input with the rules UserPublicService.CreateUser applies (ADR api-contract-protobuf-connect D2). An existing account
// is left exactly as it is: seed-admin never promotes or changes a user.
func seedAdmin(ctx context.Context, svc *auth.Service, account, password string, out io.Writer) error {
	created, err := svc.EnsureAdmin(ctx, account, password)
	if err != nil {
		return adminInputError(err)
	}
	if created {
		_, _ = fmt.Fprintf(out, "admin account %q created\n", strings.ToLower(account))
	} else {
		_, _ = fmt.Fprintf(out, "account %q already exists; nothing changed\n", strings.ToLower(account))
	}
	return nil
}

// recoverAdmin gives an existing admin a new password and enables it (auth.Service.RecoverAdmin): the way back when
// every admin is disabled, locked out or has lost the password. The input is checked like seedAdmin's.
func recoverAdmin(ctx context.Context, svc *auth.Service, account, password string, out io.Writer) error {
	err := svc.RecoverAdmin(ctx, account, password)
	if errors.Is(err, auth.ErrNoSuchAdmin) {
		return fmt.Errorf("no admin account %q: recover-admin changes only an existing admin", strings.ToLower(account))
	}
	if err != nil {
		return adminInputError(err)
	}
	_, _ = fmt.Fprintf(out, "admin account %q recovered: new password set, enabled, signed out everywhere\n", strings.ToLower(account))
	return nil
}

// adminInputError names the variable and the rule of every invalid field the service reported, never the value;
// other errors are returned as they are.
func adminInputError(err error) error {
	fields, ok := apperr.Fields(err)
	if !ok {
		return err
	}
	problems := make([]string, 0, len(fields))
	for _, f := range fields {
		name, known := adminInputVars[f.Field]
		if !known {
			name = f.Field
		}
		problems = append(problems, fmt.Sprintf("%s: %s (%s)", name, f.Detail, f.Code))
	}
	slices.Sort(problems)
	return errors.New("invalid admin account: " + strings.Join(problems, "; "))
}
