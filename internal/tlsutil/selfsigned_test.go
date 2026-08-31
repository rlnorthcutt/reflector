package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateSelfSignedProducesUsableCertificate(t *testing.T) {
	cert, err := GenerateSelfSigned()
	if err != nil {
		t.Fatalf("GenerateSelfSigned: %v", err)
	}

	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("x509.ParseCertificate: %v", err)
	}

	if !contains(leaf.DNSNames, "localhost") {
		t.Errorf("DNSNames = %v, want localhost", leaf.DNSNames)
	}
	wantIPs := []string{"127.0.0.1", "::1"}
	for _, want := range wantIPs {
		found := false
		for _, ip := range leaf.IPAddresses {
			if ip.String() == want {
				found = true
			}
		}
		if !found {
			t.Errorf("IPAddresses = %v, want to include %s", leaf.IPAddresses, want)
		}
	}
}

func TestGenerateSelfSignedWorksInARealTLSListener(t *testing.T) {
	cert, err := GenerateSelfSigned()
	if err != nil {
		t.Fatalf("GenerateSelfSigned: %v", err)
	}

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	srv.StartTLS()
	defer srv.Close()

	client := srv.Client()
	client.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify = true

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("body = %q, want ok", body)
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
