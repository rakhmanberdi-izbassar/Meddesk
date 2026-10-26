package postgres

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

func testDatabaseCertificates(t *testing.T) (string, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	root := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Synthetic database CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, root, root, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	root, err = x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(2), DNSNames: []string{"postgres.railway.internal"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, root, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err = x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})), leaf
}

func TestPrivateDatabaseCAKeepsChainAndHostnameVerification(t *testing.T) {
	rootPEM, leaf := testDatabaseCertificates(t)
	for _, host := range []string{"postgres.railway.internal", "wrong.railway.internal"} {
		cfg, err := poolConfig("postgres://user:secret-password@"+host+"/railway?sslmode=verify-full", time.Second, rootPEM)
		if err != nil {
			t.Fatal(err)
		}
		tls := cfg.ConnConfig.TLSConfig
		if tls == nil || tls.InsecureSkipVerify || tls.ServerName != host || len(cfg.ConnConfig.Fallbacks) != 0 {
			t.Fatal("certificate/hostname verification was weakened")
		}
		_, err = leaf.Verify(x509.VerifyOptions{Roots: tls.RootCAs, DNSName: tls.ServerName})
		if (err != nil) != (host != "postgres.railway.internal") {
			t.Fatal("unexpected certificate/hostname verification result")
		}
	}
	otherRoot, _ := testDatabaseCertificates(t)
	cfg, err := poolConfig("postgres://user@postgres.railway.internal/railway?sslmode=verify-full", time.Second, otherRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: cfg.ConnConfig.TLSConfig.RootCAs, DNSName: cfg.ConnConfig.TLSConfig.ServerName}); err == nil {
		t.Fatal("unrelated CA trusted the server certificate")
	}
}

func TestDatabaseCARejectsMalformedBundlesAndWeakerTLS(t *testing.T) {
	rootPEM, _ := testDatabaseCertificates(t)
	for _, mode := range []string{"disable", "allow", "prefer", "require", "verify-ca"} {
		_, err := poolConfig("postgres://user:secret-password@localhost/railway?sslmode="+mode, time.Second, rootPEM)
		if err == nil || strings.Contains(err.Error(), "secret-password") {
			t.Fatal("weaker TLS mode accepted or credentials exposed")
		}
	}
	_, err := poolConfig("postgres://user:secret-password@localhost/railway?sslmode=verify-full", time.Second, "secret-password invalid certificate")
	if err == nil || strings.Contains(err.Error(), "secret-password") {
		t.Fatal("malformed certificate accepted or configuration exposed")
	}
	if _, err := poolConfig("postgres://localhost/railway?sslmode=disable", time.Second, ""); err != nil {
		t.Fatal("existing development configuration changed")
	}
}
