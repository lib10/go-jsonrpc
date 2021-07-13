package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/filecoin-project/go-jsonrpc"
)

type TestAuthPerm struct {
	Ignore     func(ctx context.Context) error                `perm:"-"`
	AuthNew    func(ctx context.Context, perm []string) error `perm:"admin"`
	AuthVerify func(ctx context.Context, token string) error  `perm:"read"`
}

// example for out of the perm control.
func (t *TestAuthPerm) Outside(ctx context.Context, p1 string) error {
	return errors.New("TODO")
}

type TestAuthImpl struct {
}

func (impl *TestAuthImpl) AuthNew(ctx context.Context, perm []string) error {
	// nothing to do
	return nil
}

func (impl *TestAuthImpl) AuthVerify(ctx context.Context) error {
	// nothing to do
	return nil
}

//func (impl *TestAuthImpl) Outside(ctx context.Context, p1 string) error {
//	// nothing to do
//	return nil
//}

func TestAuthProxy(t *testing.T) {
	allPerm := []Permission{"admin", "read"}
	defPerm := []Permission{"read"}

	inst := &TestAuthImpl{}
	perm := &TestAuthPerm{}

	if err := PermissionedProxy(allPerm, defPerm, inst, perm); err != nil {
		t.Fatal(err)
	}

	// TODO: register to rpc
	rpcServer := jsonrpc.NewServer()
	rpcServer.Register("Testing", perm)
}
