package cache_stnsd

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/ReneKroon/ttlcache"
	"github.com/STNS/STNS/v2/model"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/sirupsen/logrus"
	"github.com/thoas/go-funk"
)

type Http struct {
	config  *Config
	cache   *ttlcache.Cache
	version string
}

type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

var lastFailTime int64
var m sync.Mutex

func SetLastFailTime(n int64) {
	m.Lock()
	defer m.Unlock()
	lastFailTime = n
}

func GetLastFailTime() int64 {
	m.Lock()
	defer m.Unlock()
	return lastFailTime
}

func NewHttp(config *Config, cache *ttlcache.Cache, version string) *Http {
	return &Http{
		config:  config,
		cache:   cache,
		version: version,
	}
}

func (h *Http) Request(path string) (bool, *Response, error) {
	supportHeaders := []string{
		"user-highest-id",
		"user-lowest-id",
		"group-highest-id",
		"group-lowest-id",
	}

	if h.config.Cache {
		body, found := h.cache.Get(path)
		if found {
			switch v := body.(type) {
			case Response:
				logrus.Debugf("response from cache:%s", path)
				return true, &v, nil
			}
		}
	}

	lastFailTime := GetLastFailTime()
	if lastFailTime != 0 && lastFailTime+h.config.RequestLocktime > time.Now().Unix() {
		return false, nil, errors.New("now request locking")
	}

	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		logrus.Errorf("make http request error:%s", err.Error())
		return false, nil, err
	}

	h.setHeaders(req)
	h.setBasicAuth(req)

	tc, err := h.tlsConfig()
	if err != nil {
		logrus.Errorf("make tls config error:%s", err.Error())
		return false, nil, err
	}

	tr := &http.Transport{
		TLSClientConfig: tc,
		Dial: (&net.Dialer{
			Timeout: time.Duration(h.config.RequestTimeout) * time.Second,
		}).Dial,
	}

	tr.Proxy = http.ProxyFromEnvironment
	if h.config.HttpProxy != "" {
		proxyUrl, err := url.Parse(h.config.HttpProxy)
		if err == nil {
			tr.Proxy = http.ProxyURL(proxyUrl)
		}
	}
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = h.config.RequestRetry

	client := retryClient.StandardClient()
	client.Transport = tr
	resp, err := client.Do(req)
	if err != nil {
		logrus.Errorf("http request error:%s", err.Error())
		SetLastFailTime(time.Now().Unix())
		return false, nil, err
	}
	SetLastFailTime(0)
	defer resp.Body.Close()

	headers := map[string]string{}
	for k, v := range resp.Header {
		if funk.ContainsString(supportHeaders, strings.ToLower(k)) {
			headers[k] = v[0]
		}
	}

	switch resp.StatusCode {
	case http.StatusOK:
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return false, nil, err
		}

		r := Response{
			StatusCode: resp.StatusCode,
			Body:       body,
			Headers:    headers,
		}
		if h.config.Cache {
			h.cache.Set(path, r)
		}

		return false, &r, nil
	default:
		r := Response{
			StatusCode: resp.StatusCode,
			Headers:    headers,
		}
		if h.config.Cache {
			h.cache.SetWithTTL(path, r, time.Duration(h.config.NegativeCacheTTL)*time.Second)
		}
		return false, &r, nil
	}
}

func (h *Http) RequestURL(requestPath, query string) (*url.URL, error) {
	u, err := url.Parse(h.config.ApiEndpoint)
	if err != nil {
		return nil, err
	}

	u.Path = path.Join(u.Path, requestPath)
	u.RawQuery = query
	return u, nil

}

func (h *Http) setHeaders(req *http.Request) {
	for k, v := range h.config.HttpHeaders {
		req.Header.Add(k, v)
	}

	if h.config.AuthToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("token %s", h.config.AuthToken))
	}
	req.Header.Set("User-Agent", fmt.Sprintf("cache-stnsd/%s", h.version))
}

func (h *Http) setBasicAuth(req *http.Request) {
	if h.config.User != "" && h.config.Password != "" {
		req.SetBasicAuth(h.config.User, h.config.Password)
	}
}

func (h *Http) tlsConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{InsecureSkipVerify: !h.config.SSLVerify}
	if h.config.TLS.CA != "" {
		CA_Pool := x509.NewCertPool()

		severCert, err := ioutil.ReadFile(h.config.TLS.CA)
		if err != nil {
			return nil, err
		}
		CA_Pool.AppendCertsFromPEM(severCert)

		tlsConfig.RootCAs = CA_Pool
	}

	if h.config.TLS.Cert != "" && h.config.TLS.Key != "" {
		x509Cert, err := tls.LoadX509KeyPair(h.config.TLS.Cert, h.config.TLS.Key)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = make([]tls.Certificate, 1)
		tlsConfig.Certificates[0] = x509Cert
	}

	if len(tlsConfig.Certificates) == 0 && tlsConfig.RootCAs == nil {
		tlsConfig = nil
	}

	return tlsConfig, nil
}

func (h *Http) prefetchUserOrGroup(resource string, ug interface{}) error {
	u, err := h.RequestURL(resource, "")
	if err != nil {
		return err
	}

	_, resp, err := h.Request(u.String())
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusOK {
		userGroups := []model.UserGroup{}

		switch v := ug.(type) {
		case []*model.User:
			if err := json.Unmarshal(resp.Body, &v); err != nil {
				return err
			}

			for _, u := range v {
				userGroups = append(userGroups, u)
			}
		case []*model.Group:
			if err := json.Unmarshal(resp.Body, &v); err != nil {
				return err
			}
			for _, u := range v {
				userGroups = append(userGroups, u)
			}
		default:
			return fmt.Errorf("unknown type: %v", reflect.TypeOf(ug))
		}

		for _, val := range userGroups {
			j, err := json.Marshal(val)
			if err != nil {
				return err
			}

			j = []byte("[" + string(j) + "]")

			u, err := h.RequestURL(resource, fmt.Sprintf("name=%s", val.GetName()))
			if err != nil {
				return err
			}

			logrus.Debugf("prefetch: set cache key:%s", u.String())
			h.cache.Set(u.String(),
				Response{
					StatusCode: http.StatusOK,
					Body:       j,
					Headers:    resp.Headers,
				},
			)

			u, err = h.RequestURL(resource, fmt.Sprintf("id=%d", val.GetID()))
			if err != nil {
				return err
			}

			logrus.Debugf("prefetch: set cache key:%s", u.String())
			h.cache.Set(u.String(),
				Response{
					StatusCode: http.StatusOK,
					Body:       j,
					Headers:    resp.Headers,
				},
			)

		}
	}
	return nil
}

func (h *Http) PrefetchUserGroups() {
	users := []*model.User{}
	groups := []*model.Group{}

	if err := h.prefetchUserOrGroup("users", users); err != nil {
		logrus.Error(err)
	}
	if err := h.prefetchUserOrGroup("groups", groups); err != nil {
		logrus.Error(err)
	}

}
