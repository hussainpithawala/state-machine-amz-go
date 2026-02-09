package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	fmt.Println("🔐 Testing Redis TLS Connection...")

	// Load CA certificate
	caCert, err := os.ReadFile("redis/tls/ca.crt")
	if err != nil {
		panic(fmt.Sprintf("Failed to read CA cert: %v", err))
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		panic("Failed to parse CA certificate")
	}

	// Load client certificate
	cert, err := tls.LoadX509KeyPair("redis/tls/redis.crt", "redis/tls/redis.key")
	if err != nil {
		panic(fmt.Sprintf("Failed to load client cert: %v", err))
	}

	// Create TLS config
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
		ServerName:   "localhost",
	}

	// Create Redis client with TLS
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6380",
		Password: "redispassword",
		DB:       0,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	})
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test PING
	fmt.Println("\n1️⃣ Testing PING...")
	pong, err := rdb.Ping(ctx).Result()
	if err != nil {
		fmt.Printf("❌ PING failed: %v\n", err)
		return
	}
	fmt.Printf("✅ PING successful: %s\n", pong)

	// Test SET
	fmt.Println("\n2️⃣ Testing SET...")
	err = rdb.Set(ctx, "test_tls_key", "TLS is working!", 60*time.Second).Err()
	if err != nil {
		fmt.Printf("❌ SET failed: %v\n", err)
		return
	}
	fmt.Println("✅ SET successful")

	// Test GET
	fmt.Println("\n3️⃣ Testing GET...")
	val, err := rdb.Get(ctx, "test_tls_key").Result()
	if err != nil {
		fmt.Printf("❌ GET failed: %v\n", err)
		return
	}
	fmt.Printf("✅ GET successful: %s\n", val)

	// Check TLS connection state
	fmt.Println("\n4️⃣ TLS Connection Details...")
	conn, err := tls.Dial("tcp", "localhost:6380", tlsConfig)
	if err != nil {
		fmt.Printf("❌ TLS dial failed: %v\n", err)
		return
	}
	defer conn.Close()

	state := conn.ConnectionState()
	fmt.Printf("✅ TLS Version: %s\n", tlsVersionString(state.Version))
	fmt.Printf("✅ Cipher Suite: %s\n", tls.CipherSuiteName(state.CipherSuite))
	fmt.Printf("✅ Server Name: %s\n", state.ServerName)
	fmt.Printf("✅ Handshake Complete: %v\n", state.HandshakeComplete)

	fmt.Println("\n🎉 All TLS tests passed!")
}

func tlsVersionString(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("Unknown (0x%04X)", version)
	}
}
