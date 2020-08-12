package cmd

import (
	"testing"
	"time"
)

func TestEnableCacheWhenServerDown(t *testing.T) {
	key := "example"
	c := ttlCache(500 * time.Millisecond)
	defer c.Close()

	setLastFailTime(0)

	c.Set(key, 1)
	if _, ok := c.Get(key); !ok {
		t.Fatal("could use cache")
	}

	time.Sleep(time.Second * 1)
	if _, ok := c.Get(key); ok {
		t.Fatal("could expire for ttl 1sec")
	}

	setLastFailTime(1)
	c.Set(key, 1)
	if _, ok := c.Get(key); !ok {
		t.Fatal("could use cache")
	}

	time.Sleep(time.Second * 2)
	if _, ok := c.Get(key); !ok {
		t.Fatal("couldn't use cache when server down")
	}

}
