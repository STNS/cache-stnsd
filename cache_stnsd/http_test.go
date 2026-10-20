package cache_stnsd

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ReneKroon/ttlcache/v2"
	"github.com/STNS/libstns-go/libstns"
)

func writeCAFile(t *testing.T, der []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(p, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func unrelatedCertDER(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "unrelated"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func TestHttp_Request_TLSVerify(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	serverCA := srv.Certificate().Raw
	otherCA := unrelatedCertDER(t)

	tests := []struct {
		name      string
		caDER     []byte
		sslVerify bool
		wantErr   bool
	}{
		{
			name:      "verify with trusted ca",
			caDER:     serverCA,
			sslVerify: true,
		},
		{
			name:      "verify with unrelated ca",
			caDER:     otherCA,
			sslVerify: true,
			wantErr:   true,
		},
		{
			name:      "skip verify with unrelated ca",
			caDER:     otherCA,
			sslVerify: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				ApiEndpoint:    srv.URL + "/v1",
				SSLVerify:      tt.sslVerify,
				Cache:          false,
				RequestRetry:   1,
				RequestTimeout: 3,
				TLS:            libstns.TLS{CA: writeCAFile(t, tt.caDER)},
			}
			cache := ttlcache.NewCache()
			defer cache.Close()

			h, err := NewHttp(config, cache, "test")
			if err != nil {
				t.Fatal(err)
			}

			_, res, err := h.Request("status", "")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Request() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && res.StatusCode != http.StatusOK {
				t.Errorf("StatusCode = %d, want %d", res.StatusCode, http.StatusOK)
			}
		})
	}
}
