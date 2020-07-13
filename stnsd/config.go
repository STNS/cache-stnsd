package stnsd

import "github.com/BurntSushi/toml"

type Config struct {
	ApiEndpoint      string            `toml:"api_endpoint"`
	AuthToken        string            `toml:"auth_token"`
	User             string            `toml:"user"`
	Password         string            `toml:"password"`
	SSLVerify        bool              `toml:"ssl_verify"`
	HttpProxy        string            `toml:"http_proxy"`
	QueryWrapper     string            `toml:"query_wrapper"`
	RequestTimeout   int               `toml:"request_timeout"`
	RequestRetry     int               `toml:"request_retry"`
	RequestLocktime  int               `toml:"request_locktime"`
	Cache            bool              `toml:"ssl_verify"`
	CacheTTL         int               `toml:"cache_ttl"`
	NegativeCacheTTL int               `toml:"negative_cache_ttl"`
	HttpHeaders      map[string]string `toml:"http_headers"`
	TlS              TLS               `toml:"tls"`
}

type TLS struct {
	CA   string `toml:"ca"`
	Cert string `toml:"cert"`
	Key  string `toml:"key"`
}

func defaultConfig(config *Config) {
	config.ApiEndpoint = "http://localhost:1104/v1"
	config.CacheTTL = 600
	config.NegativeCacheTTL = 60
	config.SSLVerify = true
	config.Cache = true
	config.RequestTimeout = 10
	config.RequestRetry = 3
	config.RequestLocktime = 60
}

func LoadConfig(filePath string) (*Config, error) {
	var config Config

	defaultConfig(&config)
	_, err := toml.DecodeFile(filePath, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}
