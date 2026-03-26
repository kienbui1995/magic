package magic_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	magic "github.com/kienbui1995/magic/sdk/go"
)

func TestClientHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()
	c := magic.NewClient(srv.URL, "")
	if err := c.Health(); err != nil {
		t.Fatal(err)
	}
}
