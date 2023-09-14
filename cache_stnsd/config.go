package cache_stnsd

import (
	"os"

	"github.com/BurntSushi/toml"
	"github.com/STNS/libstns-go/libstns"
	"github.com/sirupsen/logrus"
)

type Config struct {
	ApiEndpoint      string            `toml:"api_endpoint"`
	AuthToken        string            `toml:"auth_token"`
	User             string            `toml:"user"`
	Password         string            `toml:"password"`
	SSLVerify        bool              `toml:"ssl_verify"`
	HttpProxy        string            `toml:"http_proxy"`
	HttpKeepalive    bool              `toml:"http_keepalive"`
	RequestTimeout   int               `toml:"request_timeout"`
	RequestRetry     int               `toml:"request_retry"`
	RequestLocktime  int64             `toml:"request_locktime"`
	Cache            bool              `toml:"cache"`
	CacheTTL         int               `toml:"cache_ttl"`
	NegativeCacheTTL int               `toml:"negative_cache_ttl"`
	HttpHeaders      map[string]string `toml:"http_headers"`
	TLS              libstns.TLS       `toml:"tls"`
	PIDFile          string            `toml:"-"`
	LogFile          string            `toml:"-"`
	LogLevel         string            `toml:"-"`
	Cached           Cached            `toml:"cached"`
}

type Cached struct {
	Prefetch   bool   `toml:"prefetch"`
	UnixSocket string `toml:"unix_socket"`
}

func defaultConfig(config *Config) {
	config.ApiEndpoint = "http://localhost:1104/v1"
	config.CacheTTL = 600
	config.NegativeCacheTTL = 60
	config.SSLVerify = true
	config.Cache = true
	config.HttpKeepalive = true
	config.RequestTimeout = 10
	config.RequestRetry = 3
	config.RequestLocktime = 60
	config.Cached.UnixSocket = "/var/run/stnsd.sock"
	config.Cached.Prefetch = true
}

func LoadConfig(filePath string) (*Config, error) {
	var config Config

	defaultConfig(&config)

	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		logrus.Warn(err)
		return &config, nil
	}

	_, err = toml.DecodeFile(filePath, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}
