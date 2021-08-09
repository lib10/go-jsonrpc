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

	// test rpc function
	Add     func(ctx context.Context, a, b int) (int, error) `perm:"admin"`
	Todo    func(ctx context.Context) error                  `perm:"admin"`
	ChanSub func(ctx context.Context) (<-chan bool, error)   `perm:"admin"`
}

// Example for out of the perm control.
func (t *TestAuthPerm) Outside(ctx context.Context, p1 string) error {
	return errors.New("TODO")
}

type TestAuthEmbed struct {
	*TestAuthPerm
	Todo  func(ctx context.Context) error `perm:"admin"` // replace
	Todo1 func(ctx context.Context) error `perm:"admin"` // new implement
}
type TestAuthNoEmbed struct {
	TestAuthPerm *TestAuthPerm
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

type TestAuthImplEmbed struct {
	*TestAuthImpl
}

func (impl *TestAuthImplEmbed) Todo(ctx context.Context) error {
	return errors.New("REPLACED")
}
func (impl *TestAuthImplEmbed) Todo1(ctx context.Context) error {
	return errors.New("TODO")
}

func TestAuthProxy(t *testing.T) {
	allPerm := []Permission{"admin", "read"}
	defPerm := []Permission{"read"}

	// test implement
	if err := ReflectPerm(allPerm, defPerm, &TestAuthImpl{}, &TestAuthPerm{}); err != nil {
		t.Fatal(err)
	}

	// test unimplement
	emptyStruct := struct{}{}
	err := ReflectPerm(allPerm, defPerm, &emptyStruct, &TestAuthPerm{})
	if err == nil {
		t.Fatal("expect implement error")
	}
	if strings.Index(err.Error(), "is not implemented") < 0 {
		t.Fatal("expect 'is not implemented' error")
	}

	// test embed
	if err := ReflectPerm(allPerm, defPerm, &TestAuthImplEmbed{TestAuthImpl: &TestAuthImpl{}}, &TestAuthEmbed{TestAuthPerm: &TestAuthPerm{}}); err != nil {
		t.Fatal(err)
	}
	// test no embed
	err = ReflectPerm(allPerm, defPerm, &TestAuthImpl{}, &TestAuthNoEmbed{TestAuthPerm: &TestAuthPerm{}})
	if err == nil {
		t.Fatal("expect error for no tag")
	}
	if strings.Index(err.Error(), "missing 'perm' tag") < 0 {
		t.Fatalf("expect missing 'perm' tag, but %s", err.Error())
	}

	// testing rpc
	ctx := context.TODO()
	//exportApi := &TestAuthPerm{}
	exportApi := &TestAuthEmbed{TestAuthPerm: &TestAuthPerm{}}
	//exportInst := &TestAuthImpl{}
	exportInst := &TestAuthImplEmbed{TestAuthImpl: &TestAuthImpl{}}

	rpcServer := jsonrpc.NewServer()

	// Associate instance and template for rpc server
	if err := ReflectPerm(allPerm, defPerm, exportInst, exportApi); err != nil {
		t.Fatal(err)
	}
	// register the exportApi to rpc server
	rpcServer.Register("Testing", exportApi)

	m := http.NewServeMux()
	handler := &Handler{Verify: exportInst.AuthVerify, Next: rpcServer.ServeHTTP}
	m.Handle("/rpc", handler)

	testServ := httptest.NewTLSServer(m)
	defer testServ.Close()

	for _, proto := range []string{"wss://", "https://"} {
		//client := &TestAuthPerm{}
		client := &TestAuthEmbed{TestAuthPerm: &TestAuthPerm{}}
		headers := http.Header{}
		headers.Add("Authorization", "Bearer "+"todo")
		closer, err := jsonrpc.NewMergeClient(ctx, proto+testServ.Listener.Addr().String()+"/rpc",
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
		} else if err.Error() != "REPLACED" {
			t.Fatal("expect 'REPLACED' error, but " + err.Error())
		}

		if err := client.Todo1(ctx); err == nil {
			t.Fatal("expect error")
		} else if err.Error() != "TODO" {
			t.Fatal("expect 'TODO' error, but " + err.Error())
		}

		// only support websocket mode
		if proto == "wss://" {
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
	}
}
