/*
Copyright © 2020 pyama86 www.kazu.com@gmail.com

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by appliglobalConfig.TLS.CAble law or agreed to in writing, software
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
		if globalConfig.LogFile != "" {
			f, err := os.OpenFile(globalConfig.LogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
			if err != nil {
				logrus.Fatal("error opening file :" + err.Error())
			}
			logrus.SetOutput(f)
		}

		switch globalConfig.LogLevel {
		case "debug":
			logrus.SetLevel(logrus.DebugLevel)
		case "info":
			logrus.SetLevel(logrus.InfoLevel)
		case "warn":
			logrus.SetLevel(logrus.WarnLevel)
		case "error":
			logrus.SetLevel(logrus.ErrorLevel)
		}

		if err := runServer(); err != nil {
			logrus.Fatal(err)
		}
	},
}

type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

func runServer() error {
	supportHeaders := []string{
		"user-highest-id",
		"user-lowest-id",
		"group-highest-id",
		"group-lowest-id",
	}

	sf := globalConfig.UnixSocket
	pidfile.SetPidfilePath(globalConfig.PIDFile)

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
	c := cache.New(time.Duration(globalConfig.CacheTTL)*time.Second, 10*time.Minute)
	var m sync.Mutex
	var lastFailTime int64

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		m.Lock()
		if lastFailTime != 0 && lastFailTime+globalConfig.RequestLocktime > time.Now().Unix() {
			logrus.Warnf("now duaring locktime until:%d", lastFailTime+globalConfig.RequestLocktime)
			w.WriteHeader(http.StatusInternalServerError)
			m.Unlock()
			return
		}
		m.Unlock()

		u, err := url.Parse(globalConfig.ApiEndpoint)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		u.Path = path.Join(u.Path, r.URL.Path)
		u.RawQuery = r.URL.RawQuery
		cacheKey := u.String()

		if globalConfig.Cache {
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
		w.Header().Set("STNSD-CACHE", "0")

		err = retry.Retry(uint(globalConfig.RequestRetry), 1*time.Second, func() error {
			req, err := http.NewRequest("GET", u.String(), nil)
			if err != nil {
				logrus.Errorf("make http request error:%s", err.Error())
				return err
			}

			setHeaders(req)
			setBasicAuth(req)

			tc, err := tlsConfig()
			if err != nil {
				logrus.Errorf("make tls config error:%s", err.Error())
				return err
			}

			tr := &http.Transport{
				TLSClientConfig: tc,
				Dial: (&net.Dialer{
					Timeout: time.Duration(globalConfig.RequestTimeout) * time.Second,
				}).Dial,
			}

			tr.Proxy = http.ProxyFromEnvironment
			if globalConfig.HttpProxy != "" {
				proxyUrl, err := url.Parse(globalConfig.HttpProxy)
				if err == nil {
					tr.Proxy = http.ProxyURL(proxyUrl)
				}
			}

			client := &http.Client{Transport: tr}
			resp, err := client.Do(req)
			if err != nil {
				logrus.Errorf("http request error:%s", err.Error())
				return err
			}
			defer resp.Body.Close()

			switch resp.StatusCode {
			case http.StatusOK:
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					return err
				}
				logrus.Debugf("status ok and response from origin:%s", cacheKey)
				if globalConfig.Cache {
					headers := map[string]string{}
					for k, v := range resp.Header {
						if funk.ContainsString(supportHeaders, strings.ToLower(k)) {
							headers[k] = v[0]
							w.Header().Set(k, v[0])
						}
					}
					c.Set(cacheKey,
						Response{
							StatusCode: resp.StatusCode,
							Body:       body,
							Headers:    headers,
						},
						cache.DefaultExpiration)
				}
				w.WriteHeader(resp.StatusCode)
				w.Write(body)
			default:
				logrus.Infof("status error %d and response from origin:%s", resp.StatusCode, cacheKey)
				if globalConfig.Cache {
					c.Set(cacheKey, Response{StatusCode: resp.StatusCode}, time.Duration(globalConfig.NegativeCacheTTL)*time.Second)
				}
				w.WriteHeader(resp.StatusCode)
			}

			return nil
		})
		if err != nil {
			m.Lock()
			logrus.Warn("starting locktime")
			lastFailTime = time.Now().Unix()
			m.Unlock()
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	server := http.Server{
		Handler: mux,
	}

	defer os.Remove(sf)
	go func() {
		if err := server.Serve(unixListener); err != nil {
			if err.Error() != "http: Server closed" {
				logrus.Error(err)
			} else {
				logrus.Info(err)
			}
		}
	}()

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGINT)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	logrus.Info("starting shutdown stnsd")
	if err := server.Shutdown(ctx); err != nil {
		return err
	}
	return nil

}

func setHeaders(req *http.Request) {
	for k, v := range globalConfig.HttpHeaders {
		req.Header.Add(k, v)
	}
	req.Header.Set("User-Agent", fmt.Sprintf("cache-stnsd/%s", version))
}

func setBasicAuth(req *http.Request) {
	if globalConfig.User != "" && globalConfig.Password != "" {
		req.SetBasicAuth(globalConfig.User, globalConfig.Password)
	}
}
func tlsConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{InsecureSkipVerify: !globalConfig.SSLVerify}
	if globalConfig.TLS.CA != "" {
		CA_Pool := x509.NewCertPool()

		severCert, err := ioutil.ReadFile(globalConfig.TLS.CA)
		if err != nil {
			return nil, err
		}
		CA_Pool.AppendCertsFromPEM(severCert)

		tlsConfig.RootCAs = CA_Pool
	}

	if globalConfig.TLS.Cert != "" && globalConfig.TLS.Key != "" {
		x509Cert, err := tls.LoadX509KeyPair(globalConfig.TLS.Cert, globalConfig.TLS.Key)
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
	cobra.OnInitialize(initConfig)
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	viper.SetEnvPrefix("Stnsd")
	viper.AutomaticEnv() // read in environment variables that match
	config, err := cache_stnsd.LoadConfig(cfgFile)
	if err != nil {
		logrus.Fatal(err)
	}
	if err := viper.Unmarshal(&config); err != nil {
		logrus.Fatal(err)
	}
	globalConfig = config
}
