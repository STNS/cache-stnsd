/*
Copyright © 2020 NAME HERE <EMAIL ADDRESS>

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
	"time"

	"github.com/Songmu/retry"
	"github.com/pyama86/go-cache"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers appliglobalConfig.TLS.CAtions.
This appliglobalConfig.TLS.CAtion is a tool to generate the needed files
to quickly create a Cobra appliglobalConfig.TLS.CAtion.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runServer(); err != nil {
			logrus.Fatal(err)
		}
	},
}

type ErrorResponse struct {
	Code int
}

func runServer() error {
	sf := globalConfig.UnixSocket

	unixListener, err := net.Listen("unix", sf)
	if err != nil {
		return err
	}

	c := cache.New(time.Duration(globalConfig.CacheTTL)*time.Second, 10*time.Minute)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		u, err := url.Parse(globalConfig.ApiEndpoint)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		u.Path = r.URL.Path
		u.RawQuery = r.URL.RawQuery
		cacheKey := r.URL.Path + r.URL.RawQuery

		if globalConfig.Cache {
			body, found := c.Get(cacheKey)
			if found {
				w.Header().Set("STNSD-CACHE", "1")
				switch v := body.(type) {
				case ErrorResponse:
					w.WriteHeader(v.Code)
					return
				case []byte:
					w.WriteHeader(http.StatusOK)
					w.Write(body.([]byte))
					return
				}
			}
		}
		w.Header().Set("STNSD-CACHE", "0")

		err = retry.Retry(uint(globalConfig.RequestRetry), 1*time.Second, func() error {
			req, err := http.NewRequest("GET", u.String(), nil)
			if err != nil {
				return err
			}

			setHeaders(req)
			setBasicAuth(req)

			tc, err := tlsConfig()
			if err != nil {
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
				return err
			}
			defer resp.Body.Close()

			w.WriteHeader(resp.StatusCode)
			switch resp.StatusCode {
			case http.StatusOK:
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					return err
				}
				if globalConfig.Cache {
					c.Set(cacheKey, body, cache.DefaultExpiration)
				}
				w.Write(body)
			default:
				if globalConfig.Cache {
					c.Set(cacheKey, ErrorResponse{Code: resp.StatusCode}, time.Duration(globalConfig.NegativeCacheTTL)*time.Second)
				}
				return nil
			}

			return nil
		})
	})

	server := http.Server{
		Handler: mux,
	}

	defer os.Remove(sf)
	go func() {
		if err := server.Serve(unixListener); err != nil {
			if err.Error() != "http: Server closed" {
				logrus.Fatal(err)
			}
			logrus.Info(err)
		}
	}()

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)
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
	req.Header.Set("User-Agent", fmt.Sprintf("stnsd/%s", version))

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
	serverCmd.PersistentFlags().StringP("unix-socket", "s", "/var/run/stnsd.sock", "unix domain socket file(Env:STNSD_UNIX_SOCKET)")
	viper.BindPFlag("UnixSocket", serverCmd.PersistentFlags().Lookup("unix-socket"))
	rootCmd.AddCommand(serverCmd)
}
