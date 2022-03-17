package cache_stnsd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"time"

	"github.com/ReneKroon/ttlcache/v2"
	"github.com/STNS/STNS/v2/model"
	"github.com/STNS/libstns-go/libstns"
	"github.com/sirupsen/logrus"
)

type Http struct {
	config  *Config
	cache   *ttlcache.Cache
	client  *libstns.STNS
	version string
}

type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

func SetExpirationCallback(client *libstns.STNS, cache *ttlcache.Cache) {
	cache.SetCheckExpirationCallback(
		func(key string, value interface{}) bool {
			res, err := client.Request("status", "")
			if err != nil {
				return false
			}
			return res.StatusCode == http.StatusOK
		},
	)

}
func NewHttp(config *Config, cache *ttlcache.Cache, version string) (*Http, error) {
	client, err := libstns.NewSTNS(config.ApiEndpoint, &libstns.Options{
		AuthToken:      config.AuthToken,
		User:           config.User,
		Password:       config.Password,
		SkipSSLVerify:  config.SSLVerify,
		HttpProxy:      config.HttpProxy,
		HttpKeepalive:  config.HttpKeepalive,
		RequestTimeout: config.RequestTimeout,
		RequestRetry:   config.RequestRetry,
		HttpHeaders:    config.HttpHeaders,
		TLS:            config.TLS,
	})
	SetExpirationCallback(client, cache)
	if err != nil {
		return nil, err
	}

	return &Http{
		config:  config,
		cache:   cache,
		client:  client,
		version: version,
	}, nil
}

func (h *Http) Request(path, query string) (bool, *libstns.Response, error) {
	cacheKey, err := h.cacheKey(path, query)
	if err != nil {
		return false, nil, err
	}
	if h.config.Cache {
		body, found := h.cache.Get(cacheKey)
		if found == nil {
			switch v := body.(type) {
			case libstns.Response:
				logrus.Debugf("response from cache:%s", path)
				return true, &v, nil
			}
		}
	}

	res, err := h.client.Request(path, query)
	if err != nil && res == nil {
		logrus.Errorf("make http request error:%s", err.Error())
		return false, nil, err
	}

	switch res.StatusCode {
	case http.StatusOK:
		if h.config.Cache {
			h.cache.Set(cacheKey, *res)
		}

		return false, res, nil
	default:
		if h.config.Cache {
			h.cache.SetWithTTL(cacheKey, *res, time.Duration(h.config.NegativeCacheTTL)*time.Second)
		}
		return false, res, nil
	}
}

func (h *Http) prefetchUserOrGroup(resource string, ug interface{}) error {
	resp, err := h.client.Request(resource, "")
	if err != nil && resp == nil {
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

			j = append([]byte(`[`), j...)
			j = append(j, []byte(`]`)...)

			cacheKey, err := h.cacheKey(resource, fmt.Sprintf("name=%s", val.GetName()))
			if err != nil {
				return err
			}

			logrus.Debugf("prefetch: set cache key:%s", cacheKey)
			h.cache.Set(cacheKey,
				Response{
					StatusCode: http.StatusOK,
					Body:       j,
					Headers:    resp.Headers,
				},
			)

			cacheKey, err = h.cacheKey(resource, fmt.Sprintf("id=%d", val.GetID()))
			if err != nil {
				return err
			}

			logrus.Debugf("prefetch: set cache key:%s", cacheKey)
			h.cache.Set(cacheKey,
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

func (h *Http) cacheKey(requestPath, query string) (string, error) {
	u, err := url.Parse(h.config.ApiEndpoint)
	if err != nil {
		return "", err
	}

	u.Path = path.Join(u.Path, requestPath)
	u.RawQuery = query
	return u.String(), nil

}
