package cmd

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/STNS/cache-stnsd/cache_stnsd"
	"github.com/STNS/libstns-go/libstns"
)

func TestEnableCacheWhenServerDown(t *testing.T) {

	key := "example1"
	c := ttlCache(&cache_stnsd.Config{CacheTTL: 1})
	defer c.Close()

	c.Set(key, 1)
	if _, ok := c.Get(key); ok != nil {
		t.Fatal("could use cache")
	}
	// 通常はTTLが切れたら失効する
	time.Sleep(time.Millisecond * 1200)
	if _, ok := c.Get(key); ok == nil {
		t.Fatal("could expire for ttl 1.2sec")
	}

	// サーバのステータスが落ちている場合は失効しないように出来る
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() == "/status" {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer ts.Close()

	s, _ := libstns.NewSTNS(ts.URL, &libstns.Options{})
	cache_stnsd.SetExpirationCallback(s, c)

	key = "example2"
	c.Set(key, 1)
	if _, ok := c.Get(key); ok != nil {
		t.Fatal("could use cache")
	}

	time.Sleep(time.Second * 2)
	if _, ok := c.Get(key); ok != nil {
		t.Fatal("couldn't use cache when server down")
	}
}
