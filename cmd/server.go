/*
Copyright © 2020 pyama86 www.kazu.com@gmail.com

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by appliconfig.TLS.CAble law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ReneKroon/ttlcache"
	"github.com/STNS/cache-stnsd/cache_stnsd"
	"github.com/facebookgo/pidfile"
	"github.com/thoas/go-funk"

	"github.com/Songmu/retry"
	"github.com/pyama86/go-cache"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "starting cache-stnsd server",
	Long: `It is starting cache-stnsd command.
you can set runing config to /etc/stns/client/stns.conf.
	`,
	Run: func(cmd *cobra.Command, args []string) {
		var config *cache_stnsd.Config
		viper.SetEnvPrefix("Stnsd")
		viper.AutomaticEnv()
		config, err := cache_stnsd.LoadConfig(cfgFile)
		if err != nil {
			logrus.Fatal(err)
		}
		if err := viper.Unmarshal(&config); err != nil {
			logrus.Fatal(err)
		}

		if config.LogFile != "" {
			f, err := os.OpenFile(config.LogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
			if err != nil {
				logrus.Fatal("error opening file :" + err.Error())
			}
			logrus.SetOutput(f)
		}

		switch config.LogLevel {
		case "debug":
			logrus.SetLevel(logrus.DebugLevel)
		case "info":
			logrus.SetLevel(logrus.InfoLevel)
		case "warn":
			logrus.SetLevel(logrus.WarnLevel)
		case "error":
			logrus.SetLevel(logrus.ErrorLevel)
		}

		if err := runServer(config); err != nil {
			logrus.Fatal(err)
		}
	},
}

type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

var lastFailTime int64
var m sync.Mutex

func setLastFailTime(n int64) {
	m.Lock()
	defer m.Unlock()
	lastFailTime = n
}

func getLastFailTime() int64 {
	m.Lock()
	defer m.Unlock()
	return lastFailTime
}

func ttlCache(ttl time.Duration) *ttlcache.Cache {
	c := ttlcache.NewCache()
	c.SetTTL(ttl)
	c.SkipTtlExtensionOnHit(true)
	// cache to expire when network is ok.
	c.SetCheckExpirationCallback(
		func(key string, value interface{}) bool {
			return getLastFailTime() == 0
		},
	)
	return c
}

// return (status_codee, header, body, error)
func httpRequest(path string, config *cache_stnsd.Config) (int, map[string]string, []byte, error) {
	supportHeaders := []string{
		"user-highest-id",
		"user-lowest-id",
		"group-highest-id",
		"group-lowest-id",
	}

	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		logrus.Errorf("make http request error:%s", err.Error())
		return 0, nil, nil, err
	}

	setHeaders(req, config)
	setBasicAuth(req, config)

	tc, err := tlsConfig(config)
	if err != nil {
		logrus.Errorf("make tls config error:%s", err.Error())
		return 0, nil, nil, err
	}

	tr := &http.Transport{
		TLSClientConfig: tc,
		Dial: (&net.Dialer{
			Timeout: time.Duration(config.RequestTimeout) * time.Second,
		}).Dial,
	}

	tr.Proxy = http.ProxyFromEnvironment
	if config.HttpProxy != "" {
		proxyUrl, err := url.Parse(config.HttpProxy)
		if err == nil {
			tr.Proxy = http.ProxyURL(proxyUrl)
		}
	}

	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		logrus.Errorf("http request error:%s", err.Error())
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return 0, nil, nil, err
		}
		headers := map[string]string{}
		for k, v := range resp.Header {
			if funk.ContainsString(supportHeaders, strings.ToLower(k)) {
				headers[k] = v[0]
			}
		}

		return resp.StatusCode, headers, body, nil
	default:
		return resp.StatusCode, nil, nil, nil
	}
}
func runServer(config *cache_stnsd.Config) error {
	sf := config.UnixSocket
	pidfile.SetPidfilePath(config.PIDFile)

	unixListener, err := net.Listen("unix", sf)
	if err != nil {
		return err
	}

	if err := pidfile.Write(); err != nil {
		return err
	}

	defer func() {
		if err := os.Remove(pidfile.GetPidfilePath()); err != nil {
			logrus.Errorf("Error removing %s: %s", pidfile.GetPidfilePath(), err)
		}
	}()

	c := ttlCache(time.Duration(config.CacheTTL) * time.Second)
	defer c.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		u, err := url.Parse(config.ApiEndpoint)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		u.Path = path.Join(u.Path, r.URL.Path)
		u.RawQuery = r.URL.RawQuery
		cacheKey := u.String()

		if config.Cache {
			body, found := c.Get(cacheKey)
			if found {
				w.Header().Set("STNSD-CACHE", "1")
				switch v := body.(type) {
				case Response:
					logrus.Debugf("response from cache:%s", cacheKey)
					w.WriteHeader(v.StatusCode)
					if v.StatusCode == http.StatusOK {
						w.Write(v.Body)
						for k, vv := range v.Headers {
							w.Header().Set(k, vv)
						}
					}
					return
				}
			}
		}

		lastFailTime := getLastFailTime()
		if lastFailTime != 0 && lastFailTime+config.RequestLocktime > time.Now().Unix() {
			logrus.Warnf("now duaring locktime until:%d", lastFailTime+config.RequestLocktime)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("STNSD-CACHE", "0")

		err = retry.Retry(uint(config.RequestRetry), 1*time.Second, func() error {
			statusCode, headers, body, err := httpRequest(u.String(), config)
			if err != nil {
				return err
			}

			if statusCode == http.StatusOK {
				for k, v := range headers {
					w.Header().Set(k, v)
				}
				if config.Cache {
					c.SetWithTTL(cacheKey,
						Response{
							StatusCode: statusCode,
							Body:       body,
							Headers:    headers,
						},
						cache.DefaultExpiration)
				}
				w.WriteHeader(statusCode)
				w.Write(body)
			} else {
				if config.Cache {
					c.SetWithTTL(cacheKey, Response{StatusCode: statusCode}, time.Duration(config.NegativeCacheTTL)*time.Second)
				}
				w.WriteHeader(statusCode)
			}

			return nil
		})

		if err != nil {
			logrus.Warn("starting locktime")
			setLastFailTime(time.Now().Unix())
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			setLastFailTime(0)
		}
	})

	server := http.Server{
		Handler: mux,
	}

	go func() {
		quit := make(chan os.Signal)
		signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGINT)
		<-quit
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		logrus.Info("starting shutdown stnsd")
		if err := server.Shutdown(ctx); err != nil {
			fmt.Errorf("shutting down the server: %s", err)
		}
	}()
	defer os.Remove(sf)
	if err := server.Serve(unixListener); err != nil {
		if err.Error() != "http: Server closed" {
			logrus.Error(err)
		} else {
			logrus.Info(err)
		}
	}

	return nil

}

func setHeaders(req *http.Request, config *cache_stnsd.Config) {
	for k, v := range config.HttpHeaders {
		req.Header.Add(k, v)
	}
	req.Header.Set("User-Agent", fmt.Sprintf("cache-stnsd/%s", version))
}

func setBasicAuth(req *http.Request, config *cache_stnsd.Config) {
	if config.User != "" && config.Password != "" {
		req.SetBasicAuth(config.User, config.Password)
	}
}
func tlsConfig(config *cache_stnsd.Config) (*tls.Config, error) {
	tlsConfig := &tls.Config{InsecureSkipVerify: !config.SSLVerify}
	if config.TLS.CA != "" {
		CA_Pool := x509.NewCertPool()

		severCert, err := ioutil.ReadFile(config.TLS.CA)
		if err != nil {
			return nil, err
		}
		CA_Pool.AppendCertsFromPEM(severCert)

		tlsConfig.RootCAs = CA_Pool
	}

	if config.TLS.Cert != "" && config.TLS.Key != "" {
		x509Cert, err := tls.LoadX509KeyPair(config.TLS.Cert, config.TLS.Key)
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
func init() {
	serverCmd.PersistentFlags().StringP("unix-socket", "s", "/var/run/cache-stnsd.sock", "unix domain socket file(Env:STNSD_UNIX_SOCKET)")
	viper.BindPFlag("UnixSocket", serverCmd.PersistentFlags().Lookup("unix-socket"))

	serverCmd.PersistentFlags().StringP("pid-file", "p", "/var/run/cache-stnsd.pid", "pid file")
	viper.BindPFlag("PIDFile", serverCmd.PersistentFlags().Lookup("pid-file"))

	serverCmd.PersistentFlags().StringP("log-file", "l", "/var/log/cache-stnsd.log", "log file")
	viper.BindPFlag("LogFile", serverCmd.PersistentFlags().Lookup("log-file"))

	serverCmd.PersistentFlags().String("log-level", "info", "log level(debug,info,warn,error)")
	viper.BindPFlag("LogLevel", serverCmd.PersistentFlags().Lookup("log-level"))

	rootCmd.AddCommand(serverCmd)
}
