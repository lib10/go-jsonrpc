package auth

import (
	"context"
	"errors"
	"reflect"

	"golang.org/x/xerrors"
)

type Permission string

type permKey int

var permCtxKey permKey

func WithPerm(ctx context.Context, perms []Permission) context.Context {
	return context.WithValue(ctx, permCtxKey, perms)
}

func HasPerm(ctx context.Context, defaultPerms []Permission, perm Permission) bool {
	callerPerms, ok := ctx.Value(permCtxKey).([]Permission)
	if !ok {
		callerPerms = defaultPerms
	}

	for _, callerPerm := range callerPerms {
		if callerPerm == perm {
			return true
		}
	}
	return false
}
func PermissionedProxy(validPerms, defaultPerms []Permission, in interface{}, out interface{}) {
	if err := ReflectPerm(validPerms, defaultPerms, in, out); err != nil {
		panic(err)
	}
}

func ReflectPerm(validPerms, defaultPerms []Permission, in interface{}, out interface{}) error {
	rint := reflect.ValueOf(out).Elem()
	ra := reflect.ValueOf(in)

	for f := 0; f < rint.NumField(); f++ {
		field := rint.Type().Field(f)
		requiredPerm := Permission(field.Tag.Get("perm"))
		switch requiredPerm {
		case "":
			return errors.New("missing 'perm' tag on " + field.Name) // ok
		case "-":
			continue
		}

		// Validate perm tag
		ok := false
		for _, perm := range validPerms {
			if requiredPerm == perm {
				ok = true
				break
			}
		}
		if !ok {
			return errors.New("unknown 'perm' tag on " + field.Name) // ok
		}

		fn := ra.MethodByName(field.Name)
		if !fn.IsValid() {
			return errors.New(field.Name + " is not implemented")
		}

		rint.Field(f).Set(reflect.MakeFunc(field.Type, func(args []reflect.Value) (results []reflect.Value) {
			ctx := args[0].Interface().(context.Context)
			if HasPerm(ctx, defaultPerms, requiredPerm) {
				return fn.Call(args)
			}

			err := xerrors.Errorf("missing permission to invoke '%s' (need '%s')", field.Name, requiredPerm)
			rerr := reflect.ValueOf(&err).Elem()

			if field.Type.NumOut() == 2 {
				return []reflect.Value{
					reflect.Zero(field.Type.Out(0)),
					rerr,
				}
			} else {
				return []reflect.Value{rerr}
			}
		}))

	}
	return nil
}
