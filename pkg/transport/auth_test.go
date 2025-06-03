package transport

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestAuthenticator(t *testing.T) {
	t.Run("CreateAuthMessage", func(t *testing.T) {
		auth := &defaultAuthenticator{logger: DefaultLogger()}
		secret := "test-secret"

		msg, err := auth.createAuthMessage(secret)
		if err != nil {
			t.Fatalf("createAuthMessage() error = %v", err)
		}

		// Verify fields are populated
		if msg.Nonce == "" {
			t.Error("Nonce should not be empty")
		}
		if len(msg.Nonce) != 64 { // 32 bytes hex encoded
			t.Errorf("Nonce length = %d, want 64", len(msg.Nonce))
		}
		if msg.Timestamp == 0 {
			t.Error("Timestamp should not be zero")
		}
		if msg.HMAC == "" {
			t.Error("HMAC should not be empty")
		}

		// Verify timestamp is recent
		now := time.Now().Unix()
		if msg.Timestamp < now-1 || msg.Timestamp > now+1 {
			t.Errorf("Timestamp %d not within 1 second of current time %d", msg.Timestamp, now)
		}
	})

	t.Run("VerifyAuthMessage", func(t *testing.T) {
		auth := &defaultAuthenticator{logger: DefaultLogger()}
		secret := "test-secret"

		// Create a valid message
		msg, err := auth.createAuthMessage(secret)
		if err != nil {
			t.Fatalf("createAuthMessage() error = %v", err)
		}

		// Should verify successfully
		err = auth.verifyAuthMessage(msg, secret)
		if err != nil {
			t.Errorf("verifyAuthMessage() error = %v", err)
		}

		// Test with wrong secret
		err = auth.verifyAuthMessage(msg, "wrong-secret")
		if err == nil {
			t.Error("verifyAuthMessage() should fail with wrong secret")
		}

		// Test with expired timestamp
		oldMsg := &AuthMessage{
			Nonce:     msg.Nonce,
			Timestamp: time.Now().Unix() - 400, // 6+ minutes ago
			HMAC:      msg.HMAC,
		}
		err = auth.verifyAuthMessage(oldMsg, secret)
		if err == nil {
			t.Error("verifyAuthMessage() should fail with old timestamp")
		}

		// Test with future timestamp
		futureMsg := &AuthMessage{
			Nonce:     msg.Nonce,
			Timestamp: time.Now().Unix() + 400, // 6+ minutes in future
			HMAC:      msg.HMAC,
		}
		err = auth.verifyAuthMessage(futureMsg, secret)
		if err == nil {
			t.Error("verifyAuthMessage() should fail with future timestamp")
		}
	})

	t.Run("Handshake", func(t *testing.T) {
		secret := "test-secret"
		clientAuth := NewAuthenticator(DefaultLogger())
		serverAuth := NewAuthenticator(DefaultLogger())

		// Create paired connections
		client, server := net.Pipe()
		defer client.Close()
		defer server.Close()

		// Channel to collect results
		type result struct {
			authResult *AuthResult
			err        error
		}
		serverChan := make(chan result, 1)

		// Run server handshake in goroutine
		go func() {
			authResult, err := serverAuth.Handshake(server, secret, true)
			serverChan <- result{authResult, err}
		}()

		// Run client handshake
		clientResult, clientErr := clientAuth.Handshake(client, secret, false)

		// Get server result
		serverResult := <-serverChan

		// Check results
		if clientErr != nil {
			t.Errorf("client handshake error = %v", clientErr)
		}
		if serverResult.err != nil {
			t.Errorf("server handshake error = %v", serverResult.err)
		}

		if clientResult == nil || !clientResult.Success {
			t.Error("client handshake should succeed")
		}
		if serverResult.authResult == nil || !serverResult.authResult.Success {
			t.Error("server handshake should succeed")
		}
	})

	t.Run("HandshakeWithWrongSecret", func(t *testing.T) {
		clientAuth := NewAuthenticator(DefaultLogger())
		serverAuth := NewAuthenticator(DefaultLogger())

		// Create paired connections
		client, server := net.Pipe()
		defer client.Close()
		defer server.Close()

		// Channel to collect results
		type result struct {
			authResult *AuthResult
			err        error
		}
		serverChan := make(chan result, 1)

		// Run server handshake with different secret
		go func() {
			authResult, err := serverAuth.Handshake(server, "server-secret", true)
			serverChan <- result{authResult, err}
		}()

		// Run client handshake
		clientResult, clientErr := clientAuth.Handshake(client, "client-secret", false)

		// Get server result
		serverResult := <-serverChan

		// Both should fail authentication
		if clientErr != nil {
			t.Errorf("client handshake error = %v", clientErr)
		}
		if serverResult.err != nil {
			t.Errorf("server handshake error = %v", serverResult.err)
		}

		// Check auth results
		if clientResult == nil || clientResult.Success {
			t.Error("client auth should fail with wrong secret")
		}
		if serverResult.authResult == nil || serverResult.authResult.Success {
			t.Error("server auth should fail with wrong secret")
		}
	})

	t.Run("HandshakeTimeout", func(t *testing.T) {
		auth := NewAuthenticator(DefaultLogger())

		// Create a connection that doesn't respond
		client, server := net.Pipe()
		server.Close() // Close server side immediately
		defer client.Close()

		// This should timeout
		start := time.Now()
		_, err := auth.Handshake(client, "secret", false)
		duration := time.Since(start)

		if err == nil {
			t.Error("handshake should fail with closed connection")
		}

		// Should timeout within reasonable time
		if duration > HandshakeTimeout+time.Second {
			t.Errorf("handshake took %v, should timeout around %v", duration, HandshakeTimeout)
		}
	})

	t.Run("TLSConfig", func(t *testing.T) {
		auth := &defaultAuthenticator{logger: DefaultLogger()}

		// Test server config
		serverConfig, err := auth.GenerateTLSConfig(true)
		if err != nil {
			t.Fatalf("GenerateTLSConfig(server) error = %v", err)
		}

		if serverConfig.MinVersion != tls.VersionTLS13 {
			t.Error("TLS config should require TLS 1.3")
		}
		if len(serverConfig.Certificates) != 1 {
			t.Error("TLS config should have one certificate")
		}
		if serverConfig.ClientAuth != tls.NoClientCert {
			t.Error("Server should not require client cert")
		}

		// Test client config
		clientConfig, err := auth.GenerateTLSConfig(false)
		if err != nil {
			t.Fatalf("GenerateTLSConfig(client) error = %v", err)
		}

		if !clientConfig.InsecureSkipVerify {
			t.Error("Client should skip certificate verification")
		}
	})
}

// TestAuthMessageSerialization tests JSON serialization of auth messages
func TestAuthMessageSerialization(t *testing.T) {
	msg := &AuthMessage{
		Nonce:     "test-nonce",
		Timestamp: 1234567890,
		HMAC:      "test-hmac",
	}

	// Encode
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	if err := encoder.Encode(msg); err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	// Decode
	var decoded AuthMessage
	decoder := json.NewDecoder(&buf)
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	// Verify
	if decoded.Nonce != msg.Nonce {
		t.Errorf("Nonce = %q, want %q", decoded.Nonce, msg.Nonce)
	}
	if decoded.Timestamp != msg.Timestamp {
		t.Errorf("Timestamp = %d, want %d", decoded.Timestamp, msg.Timestamp)
	}
	if decoded.HMAC != msg.HMAC {
		t.Errorf("HMAC = %q, want %q", decoded.HMAC, msg.HMAC)
	}
}
