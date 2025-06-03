package transport

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"time"
)

// AuthMessage is the authentication message exchanged during handshake
type AuthMessage struct {
	Nonce     string `json:"nonce"`
	Timestamp int64  `json:"timestamp"`
	HMAC      string `json:"hmac"`
}

// AuthResult contains the result of authentication
type AuthResult struct {
	Success bool
	Error   error
}

// Authenticator handles the authentication handshake
type Authenticator interface {
	// Handshake performs shared secret authentication
	Handshake(conn net.Conn, secret string, isServer bool) (*AuthResult, error)

	// GenerateTLSConfig creates TLS config for the connection
	GenerateTLSConfig(isServer bool) (*tls.Config, error)
}

// defaultAuthenticator implements the Authenticator interface
type defaultAuthenticator struct {
	logger Logger
}

// NewAuthenticator creates a new authenticator
func NewAuthenticator(logger Logger) Authenticator {
	if logger == nil {
		logger = DefaultLogger()
	}
	return &defaultAuthenticator{
		logger: logger,
	}
}

// Handshake performs the authentication handshake
func (a *defaultAuthenticator) Handshake(conn net.Conn, secret string, isServer bool) (*AuthResult, error) {
	// Set deadline for handshake
	if err := conn.SetDeadline(time.Now().Add(HandshakeTimeout)); err != nil {
		return nil, fmt.Errorf("failed to set handshake deadline: %w", err)
	}

	// Generate our auth message
	authMsg, err := a.createAuthMessage(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth message: %w", err)
	}

	// Create channels for concurrent send/receive
	type result struct {
		msg *AuthMessage
		err error
	}
	recvChan := make(chan result, 1)

	// Start receiving in goroutine
	go func() {
		var remoteAuth AuthMessage
		decoder := json.NewDecoder(io.LimitReader(conn, 1024)) // Limit auth message size
		err := decoder.Decode(&remoteAuth)
		recvChan <- result{msg: &remoteAuth, err: err}
	}()

	// Send our auth message
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(authMsg); err != nil {
		return nil, fmt.Errorf("failed to send auth message: %w", err)
	}

	// Wait for remote auth message
	recv := <-recvChan
	if recv.err != nil {
		return nil, fmt.Errorf("failed to receive auth message: %w", recv.err)
	}

	// Verify the remote auth message
	if err := a.verifyAuthMessage(recv.msg, secret); err != nil {
		a.logger.Error("authentication failed", "error", err)
		// Return generic error to remote
		return &AuthResult{Success: false, Error: ErrAuthFailed}, nil
	}

	// Clear the deadline
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return nil, fmt.Errorf("failed to clear deadline: %w", err)
	}

	a.logger.Info("authentication successful", "isServer", isServer)
	return &AuthResult{Success: true}, nil
}

// createAuthMessage creates an authentication message
func (a *defaultAuthenticator) createAuthMessage(secret string) (*AuthMessage, error) {
	// Generate random nonce
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	nonceStr := hex.EncodeToString(nonce)
	timestamp := time.Now().Unix()

	// Create HMAC
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(fmt.Sprintf("%s%d%s", nonceStr, timestamp, secret)))
	hmacStr := hex.EncodeToString(h.Sum(nil))

	return &AuthMessage{
		Nonce:     nonceStr,
		Timestamp: timestamp,
		HMAC:      hmacStr,
	}, nil
}

// verifyAuthMessage verifies an authentication message
func (a *defaultAuthenticator) verifyAuthMessage(msg *AuthMessage, secret string) error {
	// Check timestamp (5 minute window)
	now := time.Now().Unix()
	if msg.Timestamp < now-300 || msg.Timestamp > now+300 {
		return fmt.Errorf("timestamp outside acceptable window")
	}

	// Verify HMAC
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(fmt.Sprintf("%s%d%s", msg.Nonce, msg.Timestamp, secret)))
	expectedHMAC := hex.EncodeToString(h.Sum(nil))

	if !hmac.Equal([]byte(msg.HMAC), []byte(expectedHMAC)) {
		return fmt.Errorf("HMAC verification failed")
	}

	return nil
}

// GenerateTLSConfig creates a TLS configuration with ephemeral certificates
func (a *defaultAuthenticator) GenerateTLSConfig(isServer bool) (*tls.Config, error) {
	// Generate ephemeral certificate
	cert, err := a.generateCertificate()
	if err != nil {
		return nil, fmt.Errorf("failed to generate certificate: %w", err)
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
	}

	if isServer {
		config.ClientAuth = tls.NoClientCert
	} else {
		config.InsecureSkipVerify = true // We've already authenticated
	}

	return config, nil
}

// generateCertificate creates an ephemeral self-signed certificate
func (a *defaultAuthenticator) generateCertificate() (tls.Certificate, error) {
	// Generate RSA key
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"linkpearl"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour), // Valid for 24 hours
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:              []string{"localhost"},
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Create TLS certificate
	cert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  priv,
	}

	return cert, nil
}
