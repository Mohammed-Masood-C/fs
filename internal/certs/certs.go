// Package certs provides the TLS certificate fs serves. On first use it
// creates a self-signed certificate and stores it in the user's config
// folder, so that the same certificate (and any browser exception or trust
// granted to it) is reused across restarts.
package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"time"
)

const (
	// organization marks certificates created by fs. Certificates without it
	// were put in place by the user and are never replaced.
	organization = "fs self-signed"
	validFor     = 365 * 24 * time.Hour
	renewBefore  = 30 * 24 * time.Hour
)

// Info describes the certificate returned by Load.
type Info struct {
	CertFile  string // where the certificate is stored; empty if it only lives in memory
	Generated bool   // a new certificate was created during this start
	Custom    bool   // the certificate was provided by the user, not created by fs
}

// Dir returns the folder that holds cert.pem and key.pem.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "fs"), nil
}

// Load returns the certificate to serve for hosts (DNS names and IP
// addresses). A stored certificate is reused unless it was created by fs and
// expires soon or does not cover all hosts, in which case a new one is made.
func Load(hosts []string) (tls.Certificate, Info, error) {
	dir, err := Dir()
	if err != nil {
		// No config folder (unusual): fall back to a certificate in memory.
		cert, _, _, err := create(hosts)
		return cert, Info{Generated: true}, err
	}
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	switch {
	case err == nil:
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return tls.Certificate{}, Info{}, fmt.Errorf("parse %s: %w", certFile, err)
		}
		if !slices.Contains(leaf.Subject.Organization, organization) {
			return cert, Info{CertFile: certFile, Custom: true}, nil
		}
		if usable(leaf, hosts) {
			return cert, Info{CertFile: certFile}, nil
		}
	case !errors.Is(err, fs.ErrNotExist):
		return tls.Certificate{}, Info{}, fmt.Errorf("load certificate from %s: %w\n"+
			"  hint: fix or delete cert.pem and key.pem there, and fs will create a new certificate", dir, err)
	}

	cert, certPEM, keyPEM, err := create(hosts)
	if err != nil {
		return tls.Certificate{}, Info{}, err
	}
	info := Info{Generated: true}
	if err := save(dir, certFile, certPEM, keyFile, keyPEM); err == nil {
		info.CertFile = certFile
	}
	return cert, info, nil
}

// usable reports whether leaf is valid for a while longer and covers hosts.
func usable(leaf *x509.Certificate, hosts []string) bool {
	if time.Now().Add(renewBefore).After(leaf.NotAfter) {
		return false
	}
	for _, h := range hosts {
		if leaf.VerifyHostname(h) != nil {
			return false
		}
	}
	return true
}

// create makes a new self-signed certificate for hosts.
func create(hosts []string) (cert tls.Certificate, certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return cert, nil, nil, fmt.Errorf("generate key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return cert, nil, nil, fmt.Errorf("generate serial number: %w", err)
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{Organization: []string{organization}, CommonName: "fs"},
		NotBefore:             now.Add(-time.Hour), // tolerate small clock differences
		NotAfter:              now.Add(validFor),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return cert, nil, nil, fmt.Errorf("create certificate: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return cert, nil, nil, fmt.Errorf("encode key: %w", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	cert, err = tls.X509KeyPair(certPEM, keyPEM)
	return cert, certPEM, keyPEM, err
}

// save stores the certificate, keeping the private key readable by the
// current user only.
func save(dir, certFile string, certPEM []byte, keyFile string, keyPEM []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		return err
	}
	return os.WriteFile(certFile, certPEM, 0o644)
}
