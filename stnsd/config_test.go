/*
Copyright © 2020 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package stnsd

import (
	"reflect"
	"testing"
)

func Test_LoadConfig(t *testing.T) {
	type args struct {
		filePath string
	}
	tests := []struct {
		name    string
		args    args
		want    *Config
		wantErr bool
	}{
		{
			name: "ok",
			args: args{
				filePath: "./testdata/full.conf",
			},

			want: &Config{
				ApiEndpoint:      "http://<server-ip>:1104/v1/",
				AuthToken:        "xxxxxxxxxxxxxxx",
				User:             "test_user",
				Password:         "test_password",
				SSLVerify:        true,
				HttpProxy:        "http://your.proxy.com",
				QueryWrapper:     "/usr/local/bin/stns-wrapper",
				RequestTimeout:   1,
				RequestRetry:     2,
				RequestLocktime:  3,
				Cache:            true,
				CacheTTL:         600,
				NegativeCacheTTL: 10,
				HttpHeaders: map[string]string{
					"X-API-TOKEN": "token",
				},
				TLS: TLS{
					CA:   "ca_cert",
					Cert: "example_cert",
					Key:  "example_key",
				},
				UnixSocket: "/var/run/stnsd.sock",
			},
		},
		{
			name: "default ok",
			args: args{
				filePath: "./testdata/empty.conf",
			},

			want: &Config{
				ApiEndpoint:      "http://localhost:1104/v1",
				AuthToken:        "",
				User:             "",
				Password:         "",
				SSLVerify:        true,
				HttpProxy:        "",
				QueryWrapper:     "",
				RequestTimeout:   10,
				RequestRetry:     3,
				RequestLocktime:  60,
				Cache:            true,
				CacheTTL:         600,
				NegativeCacheTTL: 60,
				HttpHeaders:      nil,
				TLS: TLS{
					CA:   "",
					Cert: "",
					Key:  "",
				},
				UnixSocket: "/var/run/stnsd.sock",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadConfig(tt.args.filePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("loadConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("loadConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}
