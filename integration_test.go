package main

import (
	"context"
	"encoding/json"
	"flag"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/k0kubun/pp"
	_ "github.com/lib/pq"
)

var (
	integration  = flag.Bool("integration", false, "run http integration tests")
	testEndpoint = "http://unix"
)

type httpBinOrgResponse struct {
	Args struct {
		A string `json:"a"`
	} `json:"args"`
	Headers struct {
		AcceptEncoding string `json:"Accept-Encoding"`
		Host           string `json:"Host"`
		UserAgent      string `json:"User-Agent"`
		XAmznTraceID   string `json:"X-Amzn-Trace-Id"`
	} `json:"headers"`
	Origin string `json:"origin"`
	URL    string `json:"url"`
}

func TestMain(m *testing.M) {
	flag.Parse()
	result := m.Run()
	os.Exit(result)
}

func TestHTTPGet(t *testing.T) {
	if !*integration {
		t.Skip()
	}

	client := http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", "/tmp/stnsd.sock")
			},
		},
	}

	req, _ := http.NewRequest("GET", testEndpoint+"/get?a=b", nil)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	response := httpBinOrgResponse{}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}

	if response.Args.A != "b" {
		t.Errorf("HTTP Get returned wrong response body: got %v want b", response.Args.A)
	}

	if res.StatusCode != http.StatusOK {
		t.Errorf("HTTP Get returned wrong status code: got %v want %v",
			res.StatusCode, http.StatusOK)
	}

	if res.Header.Get("STNSD-CACHE") != "0" {
		t.Errorf("HTTP Get returned wrong cache flg: got %v expected %v",
			res.Header.Get("STNSD-CACHE"), 0)
	}

	req, _ = http.NewRequest("GET", testEndpoint+"/get?a=b", nil)
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	if res.Header.Get("STNSD-CACHE") != "1" {
		t.Errorf("HTTP Get returned wrong cache flg: got %v expected %v",
			res.Header.Get("STNSD-CACHE"), 1)
	}

	req, _ = http.NewRequest("GET", testEndpoint+"/response-headers?user-highest-id=1&user-lowest-id=2&group-highest-id=3&group-lowest-id=4", nil)
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	if res.Header.Get("User-Highest-Id") != "1" ||
		res.Header.Get("User-Lowest-Id") != "2" ||
		res.Header.Get("Group-Highest-Id") != "3" ||
		res.Header.Get("Group-Lowest-Id") != "4" {
		pp.Println(res.Header)
		t.Error("HTTP Get returned wrong header")
	}
}
