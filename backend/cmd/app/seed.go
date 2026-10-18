package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Will413028/fullstack-template/backend/api"
	"github.com/Will413028/fullstack-template/backend/internal/modules/auth"
	"github.com/Will413028/fullstack-template/backend/internal/platform/apperr"
)

// The environment variable each CreateUserRequest property comes from, for error messages.
var adminInputVars = map[string]string{"account": "ADMIN_ACCOUNT", "password": "ADMIN_PASSWORD"}

// seedAdmin creates the admin account unless it exists (inventory E8, B17). The input is checked against the
// contract's CreateUserRequest, the same rules POST /api/users applies (plan I1), and the password against the
// list. An existing account is left exactly as it is: seed-admin never promotes or changes a user.
func seedAdmin(ctx context.Context, svc *auth.Service, account, password string, out io.Writer) error {
	if err := validateAdminInput(ctx, account, password); err != nil {
		return err
	}
	created, err := svc.EnsureAdmin(ctx, account, password)
	if e, ok := apperr.As(err); ok && e.Kind == apperr.Validation {
		return fmt.Errorf("ADMIN_PASSWORD: %s (%s)", e.Detail, e.Code)
	}
	if err != nil {
		return err
	}
	if created {
		_, _ = fmt.Fprintf(out, "admin account %q created\n", strings.ToLower(account))
	} else {
		_, _ = fmt.Fprintf(out, "account %q already exists; nothing changed\n", strings.ToLower(account))
	}
	return nil
}

// validateAdminInput checks the input against CreateUserRequest from the embedded contract. Messages name the
// variable and the rule, never the value.
func validateAdminInput(ctx context.Context, account, password string) error {
	loader := openapi3.NewLoader()
	loader.Context = ctx
	doc, err := loader.LoadFromData(api.Spec)
	if err != nil {
		return fmt.Errorf("load api/openapi.yaml: %w", err)
	}
	ref := doc.Components.Schemas["CreateUserRequest"]
	if ref == nil || ref.Value == nil {
		return errors.New("api/openapi.yaml has no CreateUserRequest schema")
	}
	err = ref.Value.VisitJSON(map[string]any{"account": account, "password": password}, openapi3.MultiErrors())
	if err == nil {
		return nil
	}
	var problems []string
	var multi openapi3.MultiError
	if !errors.As(err, &multi) {
		multi = openapi3.MultiError{err}
	}
	for _, e := range multi {
		var schemaErr *openapi3.SchemaError
		if !errors.As(e, &schemaErr) {
			return err
		}
		field := strings.Join(schemaErr.JSONPointer(), "/")
		if v, ok := adminInputVars[field]; ok {
			field = v
		}
		problems = append(problems, field+": "+schemaErr.Reason)
	}
	slices.Sort(problems)
	return errors.New("invalid admin account: " + strings.Join(problems, "; "))
}
