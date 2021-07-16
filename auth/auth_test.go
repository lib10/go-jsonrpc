package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/filecoin-project/go-jsonrpc"
)

type TestAuthPerm struct {
	Ignore     func(ctx context.Context) error                               `perm:"-"`
	AuthVerify func(ctx context.Context, token string) ([]Permission, error) `perm:"read"`

	// function call testing
	Add     func(ctx context.Context, a, b int) (int, error) `perm:"admin"`
	Todo    func(ctx context.Context) error                  `perm:"admin"`
	ChanSub func(ctx context.Context) (<-chan bool, error)   `perm:"admin"`
}

// Example for out of the perm control.
func (t *TestAuthPerm) Outside(ctx context.Context, p1 string) error {
	return errors.New("TODO")
}

type TestAuthImpl struct {
}

func (impl *TestAuthImpl) AuthVerify(ctx context.Context, token string) ([]Permission, error) {
	// TODO: auth token
	return []Permission{"admin", "read"}, nil
}

//func (impl *TestAuthImpl) Outside(ctx context.Context, p1 string) error {
// nothing to do for testing
//	return nil
//}

func (impl *TestAuthImpl) Add(ctx context.Context, a, b int) (int, error) {
	return a + b, nil
}
func (impl *TestAuthImpl) Todo(ctx context.Context) error {
	return errors.New("TODO")
}
func (impl *TestAuthImpl) ChanSub(ctx context.Context) (<-chan bool, error) {
	ch := make(chan bool, 1)
	ch <- true
	return ch, nil
}

func TestAuthProxy(t *testing.T) {
	allPerm := []Permission{"admin", "read"}
	defPerm := []Permission{"read"}

	inst := &TestAuthImpl{}
	perm := &TestAuthPerm{}

	// testing implement
	if err := ReflectPerm(allPerm, defPerm, inst, perm); err != nil {
		t.Fatal(err)
	}

	// testing unimplement
	emptyStruct := struct{}{}
	err := ReflectPerm(allPerm, defPerm, &emptyStruct, perm)
	if err == nil {
		t.Fatal("expect implement error")
	}
	if strings.Index(err.Error(), "is not implemented") < 0 {
		t.Fatal("expect 'is not implemented' error")
	}

	// testing rpc
	ctx := context.TODO()
	rpcServer := jsonrpc.NewServer()
	rpcServer.Register("Testing", perm)
	m := http.NewServeMux()
	handler := &Handler{Verify: inst.AuthVerify, Next: rpcServer.ServeHTTP}
	m.Handle("/rpc", handler)

	testServ := httptest.NewTLSServer(m)
	defer testServ.Close()

	client := &TestAuthPerm{}
	headers := http.Header{}
	headers.Add("Authorization", "Bearer "+"todo")
	closer, err := jsonrpc.NewMergeClient(ctx, "wss://"+testServ.Listener.Addr().String()+"/rpc",
		"Testing",
		[]interface{}{client}, headers,
		func(c *jsonrpc.Config) {
			c.Insecure = true
		},
	)
	defer closer()

	added, err := client.Add(ctx, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if added != 3 {
		t.Fatalf("expect 3, but:%d", added)
	}

	if err := client.Todo(ctx); err == nil {
		t.Fatal("expect error")
	} else if err.Error() != "TODO" {
		t.Fatal("expect 'TODO' error")
	}

	ch, err := client.ChanSub(ctx)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case res := <-ch:
		if !res {
			t.Fatal("expect true")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("expect chan not empty")
	}

}
