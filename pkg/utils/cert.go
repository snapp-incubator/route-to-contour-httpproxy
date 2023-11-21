package utils

import (
	"crypto/x509"
	"encoding/pem"
	"strings"
)

func IsWildcardCertificate(cert string) (bool, error) {
	// Decode the first PEM block (end-entity certificate)
	block, _ := pem.Decode([]byte(cert))
	if block == nil {
		return false, nil
	}

	// Parse the end-entity certificate
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false, err
	}

	// Check for wildcard in Common Name (CN)
	if strings.HasPrefix(certificate.Subject.CommonName, "*.") {
		return true, nil
	}

	// Check for wildcards in Subject Alternative Names (SAN)
	for _, dnsName := range certificate.DNSNames {
		if strings.HasPrefix(dnsName, "*.") {
			return true, nil
		}
	}

	return false, nil
}
