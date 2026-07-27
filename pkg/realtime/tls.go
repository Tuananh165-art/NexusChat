package realtime

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"strings"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func grpcTLSEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("NEXUSCHAT_GRPC_TLS_ENABLED")), "true")
}

func readCAPool() (*x509.CertPool, error) {
	path := strings.TrimSpace(os.Getenv("NEXUSCHAT_GRPC_TLS_CA_FILE"))
	if path == "" {
		return nil, errors.New("NEXUSCHAT_GRPC_TLS_CA_FILE is required when gRPC TLS is enabled")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(body) {
		return nil, errors.New("gRPC TLS CA file contains no certificates")
	}
	return pool, nil
}

func loadCertificate(certEnv, keyEnv string) (tls.Certificate, error) {
	certFile, keyFile := strings.TrimSpace(os.Getenv(certEnv)), strings.TrimSpace(os.Getenv(keyEnv))
	if certFile == "" || keyFile == "" {
		return tls.Certificate{}, errors.New(certEnv + " and " + keyEnv + " are required when gRPC TLS is enabled")
	}
	return tls.LoadX509KeyPair(certFile, keyFile)
}

func ClientTransportCredentials() (credentials.TransportCredentials, error) {
	if !grpcTLSEnabled() {
		return nil, nil
	}
	pool, err := readCAPool()
	if err != nil {
		return nil, err
	}
	certificate, err := loadCertificate("NEXUSCHAT_GRPC_TLS_CLIENT_CERT_FILE", "NEXUSCHAT_GRPC_TLS_CLIENT_KEY_FILE")
	if err != nil {
		return nil, err
	}
	return credentials.NewTLS(&tls.Config{
		MinVersion:   tls.VersionTLS13,
		RootCAs:      pool,
		Certificates: []tls.Certificate{certificate},
		ServerName:   strings.TrimSpace(os.Getenv("NEXUSCHAT_GRPC_TLS_SERVER_NAME")),
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}), nil
}

func ServerTransportCredentials() (credentials.TransportCredentials, error) {
	if !grpcTLSEnabled() {
		return nil, nil
	}
	pool, err := readCAPool()
	if err != nil {
		return nil, err
	}
	certificate, err := loadCertificate("NEXUSCHAT_GRPC_TLS_SERVER_CERT_FILE", "NEXUSCHAT_GRPC_TLS_SERVER_KEY_FILE")
	if err != nil {
		return nil, err
	}
	return credentials.NewTLS(&tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{certificate},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
	}), nil
}

func MustClientTransportCredentials() credentials.TransportCredentials {
	if !grpcTLSEnabled() {
		return insecure.NewCredentials()
	}
	credentials, err := ClientTransportCredentials()
	if err != nil {
		panic(err)
	}
	return credentials
}

func MustServerTransportCredentials() credentials.TransportCredentials {
	if !grpcTLSEnabled() {
		return nil
	}
	credentials, err := ServerTransportCredentials()
	if err != nil {
		panic(err)
	}
	return credentials
}
