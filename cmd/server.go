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
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ReneKroon/ttlcache"
	"github.com/STNS/cache-stnsd/cache_stnsd"
	"github.com/facebookgo/pidfile"

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

func ttlCache(ttl time.Duration) *ttlcache.Cache {
	c := ttlcache.NewCache()
	c.SetTTL(ttl)
	c.SkipTtlExtensionOnHit(true)
	// cache to expire when network is ok.
	c.SetCheckExpirationCallback(
		func(key string, value interface{}) bool {
			return cache_stnsd.GetLastFailTime() == 0
		},
	)
	return c
}

func runServer(config *cache_stnsd.Config) error {
	sf := config.Cached.UnixSocket
	pidfile.SetPidfilePath(config.PIDFile)

	unixListener, err := net.Listen("unix", sf)
	if err != nil {
		return err
	}

	if err := os.Chmod(sf, 0777); err != nil {
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

	cache := ttlCache(time.Duration(config.CacheTTL) * time.Second)
	defer cache.Close()

	chttp := cache_stnsd.NewHttp(
		config,
		cache,
		version,
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		u, err := chttp.RequestURL(r.URL.Path, r.URL.RawQuery)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("STNSD-CACHE", "0")
		isCache, resp, err := chttp.Request(u.String(), false)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if isCache {
			w.Header().Set("STNSD-CACHE", "1")
		}

		if len(resp.Headers) > 0 {
			for k, vv := range resp.Headers {
				w.Header().Set(k, vv)
			}
		}

		w.WriteHeader(resp.StatusCode)
		if resp.StatusCode == http.StatusOK {
			w.Write(resp.Body)
		}
	})

	server := http.Server{
		Handler: mux,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if config.Cache && config.Cached.Prefetch {
		go func() {
			t := time.NewTicker(time.Duration(config.CacheTTL/2) * time.Second)
			defer func() {
				t.Stop()
			}()
			for {
				select {
				case <-t.C:
					logrus.Info("start prefetch")
					chttp.PrefetchUserGroups()
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	go func() {
		quit := make(chan os.Signal)
		signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGINT)
		<-quit
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		logrus.Info("starting shutdown stnsd")
		if err := server.Shutdown(ctx); err != nil {
			logrus.Errorf("shutting down the server: %s", err)
		}
	}()
	defer os.Remove(sf)
	logrus.Info("starting cache-stnsd")
	if err := server.Serve(unixListener); err != nil {
		if err.Error() != "http: Server closed" {
			logrus.Error(err)
		} else {
			logrus.Info("shutdown cache-stnsd")
		}
	}

	return nil

}

func init() {
	serverCmd.PersistentFlags().StringP("unix-socket", "s", "/var/run/cache-stnsd.sock", "unix domain socket file(Env:STNSD_UNIX_SOCKET)")
	viper.BindPFlag("Cached.UnixSocket", serverCmd.PersistentFlags().Lookup("unix-socket"))

	serverCmd.PersistentFlags().StringP("pid-file", "p", "/var/run/cache-stnsd.pid", "pid file")
	viper.BindPFlag("PIDFile", serverCmd.PersistentFlags().Lookup("pid-file"))

	serverCmd.PersistentFlags().StringP("log-file", "l", "/var/log/cache-stnsd.log", "log file")
	viper.BindPFlag("LogFile", serverCmd.PersistentFlags().Lookup("log-file"))

	serverCmd.PersistentFlags().String("log-level", "info", "log level(debug,info,warn,error)")
	viper.BindPFlag("LogLevel", serverCmd.PersistentFlags().Lookup("log-level"))

	rootCmd.AddCommand(serverCmd)
}
