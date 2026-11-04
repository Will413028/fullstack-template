// Package rpc is the Connect side of the HTTP layer that every module's handlers share: which caller class a service
// belongs to, the checks a service must pass before it is mounted, the interceptors (error conversion, panic
// recovery, the admin check), error conversion for the net/http layer, and request helpers (page tokens, ids).
// Decisions: docs/adr/2026-11-03-api-contract-protobuf-connect.md.
package rpc

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"unicode"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Class is who may call a service. Authorization follows the service, so it is decided once, by the service's name.
type Class int

const (
	// Public is <Module>PublicService: anyone, no session needed.
	Public Class = iota + 1
	// App is <Module>AppService: any signed-in user.
	App
	// Admin is <Module>AdminService: a signed-in admin.
	Admin
)

func (c Class) String() string {
	switch c {
	case Public:
		return "public"
	case App:
		return "app"
	case Admin:
		return "admin"
	}
	return "unknown"
}

// ClassOf returns the class a service's full name gives it, false for a name that ends in none of the suffixes.
func ClassOf(service protoreflect.FullName) (Class, bool) {
	name := string(service.Name())
	switch {
	case strings.HasSuffix(name, "AdminService"):
		return Admin, true
	case strings.HasSuffix(name, "AppService"):
		return App, true
	case strings.HasSuffix(name, "PublicService"):
		return Public, true
	}
	return 0, false
}

// Service is one Connect service to mount: its descriptor and the generated constructor (New<Name>Handler bound to
// an implementation).
type Service struct {
	Desc protoreflect.ServiceDescriptor
	New  func(opts ...connect.HandlerOption) (string, http.Handler)
}

// Mux serves the mounted services at /<service>/<method>, behind the interceptors, and knows each procedure's class.
type Mux struct {
	mux     *http.ServeMux
	classes map[string]Class
}

// NewMux checks every service and mounts it. A service fails the check, and the server does not start, when its
// name gives no class, when it has a streaming method (the interceptors wrap unary calls only, so the admin check
// would not run), or when a method other than Get* or List* is marked NO_SIDE_EFFECTS: such a method is served on
// GET, which CrossOriginProtection does not check. adminRole is the role Admin services require.
func NewMux(logger *slog.Logger, adminRole string, services []Service) (*Mux, error) {
	m := &Mux{mux: http.NewServeMux(), classes: map[string]Class{}}
	var errs []error
	for _, s := range services {
		class, err := check(s.Desc)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		methods := s.Desc.Methods()
		for i := range methods.Len() {
			m.classes[Procedure(methods.Get(i))] = class
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	opts := connect.WithInterceptors(convertErrors(logger), recoverPanics(logger), requireAdmin(logger, adminRole, m.classes))
	for _, s := range services {
		path, h := s.New(opts)
		m.mux.Handle(path, h)
	}
	return m, nil
}

// check returns the class of a service that may be mounted.
func check(svc protoreflect.ServiceDescriptor) (Class, error) {
	class, ok := ClassOf(svc.FullName())
	if !ok {
		return 0, fmt.Errorf("service %s: the name must end in PublicService, AppService or AdminService", svc.FullName())
	}
	var errs []error
	methods := svc.Methods()
	for i := range methods.Len() {
		md := methods.Get(i)
		if md.IsStreamingClient() || md.IsStreamingServer() {
			errs = append(errs, fmt.Errorf("%s: streaming methods are not supported", Procedure(md)))
		}
		if SideEffectFree(md) && !isRead(md.Name()) {
			errs = append(errs, fmt.Errorf("%s: only Get and List methods may be NO_SIDE_EFFECTS (served on GET, which has no CSRF check)", Procedure(md)))
		}
	}
	return class, errors.Join(errs...)
}

// isRead reports whether a method name is Get<X> or List<X>, X starting with a word of its own: GetOrder is a read,
// Getaway and Listen are not, and neither is GetOrCreateThing (Or plus another verb).
func isRead(name protoreflect.Name) bool {
	startsWord := func(s string) bool { return s != "" && unicode.IsUpper(rune(s[0])) }
	for _, verb := range []string{"Get", "List"} {
		if rest, ok := strings.CutPrefix(string(name), verb); ok && startsWord(rest) {
			after, compound := strings.CutPrefix(rest, "Or")
			return !compound || !startsWord(after)
		}
	}
	return false
}

// SideEffectFree reports whether a method is marked idempotency_level = NO_SIDE_EFFECTS, which lets clients call it
// with GET.
func SideEffectFree(md protoreflect.MethodDescriptor) bool {
	opts, _ := md.Options().(*descriptorpb.MethodOptions)
	return opts.GetIdempotencyLevel() == descriptorpb.MethodOptions_NO_SIDE_EFFECTS
}

// Procedure is a method's Connect procedure, /<service full name>/<method>.
func Procedure(md protoreflect.MethodDescriptor) string {
	return "/" + string(md.Parent().FullName()) + "/" + string(md.Name())
}

// Class returns the class of a mounted procedure, false for one that is not mounted.
func (m *Mux) Class(procedure string) (Class, bool) {
	c, ok := m.classes[procedure]
	return c, ok
}

// ServeHTTP serves a request whose path is a procedure.
func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) { m.mux.ServeHTTP(w, r) }
