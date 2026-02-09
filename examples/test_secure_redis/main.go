package main

import (
	"context"
	"crypto/tls"
	_ "crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	_ "os"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

func getRedisAddr() string {
	return os.Getenv("REDIS_ADDRESS")
}

func createTLSConfig() (*tls.Config, error) {
	// Load CA certificate
	//caCert, err := os.ReadFile("docker-examples/redis/tls/ca.crt")
	//if err != nil {
	//	return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	//}

	//caCertPool := x509.NewCertPool()
	//if !caCertPool.AppendCertsFromPEM(caCert) {
	//	return nil, fmt.Errorf("failed to append CA certificate")
	//}

	// Load client certificate and key
	//cert, err := tls.LoadX509KeyPair("docker-examples/redis/tls/redis.crt", "docker-examples/redis/tls/redis.key")
	//if err != nil {
	//	return nil, fmt.Errorf("failed to load client certificate: %w", err)
	//}

	return &tls.Config{
		//Certificates:       []tls.Certificate{cert},
		//RootCAs:            caCertPool,
		//ServerName:         "redis", // Must match CN in certificate
		//MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
	}, nil
}
func createRedisClientOptTLS() asynq.RedisClientOpt {
	password := os.Getenv("REDIS_PASSWORD")
	if password == "" {
		panic("REDIS_PASSWORD environment variable is not set")
	}

	// Check if TLS is enabled
	useTLS := os.Getenv("REDIS_USE_TLS")

	baseOpt := asynq.RedisClientOpt{
		Addr:         getRedisAddr(),
		Password:     password,
		DB:           0,
		DialTimeout:  10 * time.Second,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		PoolSize:     10,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	if useTLS == "true" {
		tlsConfig, err := createTLSConfig()
		if err != nil {
			panic(fmt.Sprintf("Failed to create TLS config: %v", err))
		}
		baseOpt.TLSConfig = tlsConfig
	}

	return baseOpt
}

func createRedisClientOpt() asynq.RedisClientOpt {
	password := os.Getenv("REDIS_PASSWORD")
	if password == "" {
		panic("REDIS_PASSWORD environment variable is not set")
	}

	return asynq.RedisClientOpt{
		Addr:         getRedisAddr(),
		Password:     password,
		DB:           0,
		DialTimeout:  10 * time.Second,
		ReadTimeout:  30 * time.Second, // Increased to handle timeout issues
		WriteTimeout: 30 * time.Second,
		PoolSize:     10,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
			//ClientAuth:         tls.NoClientCert,
			//ClientCAs:          nil,
		},
	}
}

// Test 1: Check Redis Connection
func testRedisConnection(redisOpt asynq.RedisClientOpt) error {
	fmt.Println("🔍 Test 1: Testing Redis connection...")

	// Create a raw Redis client to test connection
	var rdb *redis.Client
	useTLS := os.Getenv("REDIS_USE_TLS")
	if redisOpt.TLSConfig != nil && useTLS == "true" {
		rdb = redis.NewClient(&redis.Options{
			Addr:      redisOpt.Addr,
			Password:  redisOpt.Password,
			DB:        redisOpt.DB,
			TLSConfig: redisOpt.TLSConfig,
		})
	} else {
		rdb = redis.NewClient(&redis.Options{
			Addr:     redisOpt.Addr,
			Password: redisOpt.Password,
			DB:       redisOpt.DB,
		})
	}
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pong, err := rdb.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("❌ Redis connection failed: %w", err)
	}

	fmt.Printf("✅ Redis connection successful: %s\n", pong)
	return nil
}

// Test 2: Check TLS Configuration (if enabled)
func testTLSConnection(redisOpt asynq.RedisClientOpt) error {
	if redisOpt.TLSConfig == nil || os.Getenv("REDIS_USE_TLS") == "false" {
		fmt.Println("⚠️  Test 2: TLS not configured (skipping)")
		return nil
	}

	fmt.Println("🔍 Test 2: Testing TLS connection...")

	conn, err := tls.Dial("tcp", redisOpt.Addr, redisOpt.TLSConfig)
	if err != nil {
		return fmt.Errorf("❌ TLS connection failed: %w", err)
	}
	defer conn.Close()

	state := conn.ConnectionState()
	fmt.Printf("✅ TLS connection successful\n")
	fmt.Printf("   - Version: %s\n", tlsVersionString(state.Version))
	fmt.Printf("   - Cipher Suite: %s\n", tls.CipherSuiteName(state.CipherSuite))
	fmt.Printf("   - Server Name: %s\n", state.ServerName)

	return nil
}

// Test 3: Enqueue a Simple Task
func testEnqueueTask(client *asynq.Client) error {
	fmt.Println("🔍 Test 3: Testing task enqueue...")

	payload := map[string]interface{}{
		"message": "Hello, Asynq!",
		"test_id": time.Now().Unix(),
	}

	payloadBytes, _ := json.Marshal(payload)
	task := asynq.NewTask("test:simple", payloadBytes)

	info, err := client.Enqueue(task)
	if err != nil {
		return fmt.Errorf("❌ Failed to enqueue task: %w", err)
	}

	fmt.Printf("✅ Task enqueued successfully\n")
	fmt.Printf("   - Task ID: %s\n", info.ID)
	fmt.Printf("   - Queue: %s\n", info.Queue)
	fmt.Printf("   - Max Retry: %d\n", info.MaxRetry)

	return nil
}

// Test 4: Schedule a Delayed Task
func testScheduleTask(client *asynq.Client) error {
	fmt.Println("🔍 Test 4: Testing scheduled task...")

	payload := map[string]interface{}{
		"message":      "Scheduled task",
		"scheduled_at": time.Now().Add(10 * time.Second).Unix(),
	}

	payloadBytes, _ := json.Marshal(payload)
	task := asynq.NewTask("test:scheduled", payloadBytes)

	info, err := client.Enqueue(task, asynq.ProcessIn(10*time.Second))
	if err != nil {
		return fmt.Errorf("❌ Failed to schedule task: %w", err)
	}

	fmt.Printf("✅ Task scheduled successfully\n")
	fmt.Printf("   - Task ID: %s\n", info.ID)
	fmt.Printf("   - Next Process At: %s\n", info.NextProcessAt)

	return nil
}

// Test 5: Check Queue Stats
func testQueueStats(inspector *asynq.Inspector) error {
	fmt.Println("🔍 Test 5: Testing queue statistics...")

	queues, err := inspector.Queues()
	if err != nil {
		return fmt.Errorf("❌ Failed to get queues: %w", err)
	}

	fmt.Printf("✅ Found %d queue(s)\n", len(queues))

	for _, qname := range queues {
		stats, err := inspector.GetQueueInfo(qname)
		if err != nil {
			fmt.Printf("   ⚠️  Could not get stats for queue '%s': %v\n", qname, err)
			continue
		}

		fmt.Printf("   📊 Queue: %s\n", qname)
		fmt.Printf("      - Pending: %d\n", stats.Pending)
		fmt.Printf("      - Active: %d\n", stats.Active)
		fmt.Printf("      - Scheduled: %d\n", stats.Scheduled)
		fmt.Printf("      - Retry: %d\n", stats.Retry)
		fmt.Printf("      - Archived: %d\n", stats.Archived)
	}

	return nil
}

// Test 6: Process a Task (Handler Test)
func testTaskHandler(redisOpt asynq.RedisClientOpt) error {
	fmt.Println("🔍 Test 6: Testing task processing...")

	// Create a test handler
	processed := make(chan bool, 1)

	mux := asynq.NewServeMux()
	mux.HandleFunc("test:handler", func(ctx context.Context, t *asynq.Task) error {
		var payload map[string]interface{}
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			return err
		}
		fmt.Printf("   ✅ Handler processed task: %v\n", payload)
		processed <- true
		return nil
	})

	// Create a temporary server
	srv := asynq.NewServer(
		redisOpt,
		asynq.Config{
			Concurrency: 1,
			Queues: map[string]int{
				"default": 1,
			},
			ShutdownTimeout: 5 * time.Second,
		},
	)

	// Start server in goroutine
	go func() {
		if err := srv.Run(mux); err != nil {
			fmt.Printf("   ⚠️  Server error: %v\n", err)
		}
	}()

	// Give server time to start
	time.Sleep(1 * time.Second)

	// Enqueue a test task
	client := asynq.NewClient(redisOpt)
	defer client.Close()

	payload := map[string]interface{}{"test": "handler"}
	payloadBytes, _ := json.Marshal(payload)
	task := asynq.NewTask("test:handler", payloadBytes)

	if _, err := client.Enqueue(task); err != nil {
		srv.Shutdown()
		return fmt.Errorf("❌ Failed to enqueue test task: %w", err)
	}

	// Wait for processing or timeout
	select {
	case <-processed:
		fmt.Println("✅ Task processed successfully by handler")
	case <-time.After(5 * time.Second):
		srv.Shutdown()
		return fmt.Errorf("❌ Task processing timeout")
	}

	srv.Shutdown()
	return nil
}

// Test 7: Test Retry Logic
func testRetryLogic(client *asynq.Client) error {
	fmt.Println("🔍 Test 7: Testing retry configuration...")

	payload := map[string]interface{}{"test": "retry"}
	payloadBytes, _ := json.Marshal(payload)
	task := asynq.NewTask("test:retry", payloadBytes)

	info, err := client.Enqueue(
		task,
		asynq.MaxRetry(3),
		asynq.Timeout(30*time.Second),
	)
	if err != nil {
		return fmt.Errorf("❌ Failed to enqueue task with retry: %w", err)
	}

	fmt.Printf("✅ Task with retry configured\n")
	fmt.Printf("   - Max Retry: %d\n", info.MaxRetry)
	fmt.Printf("   - Timeout: %v\n", info.Timeout)

	return nil
}

// Test 8: Check Memory and Performance
func testRedisMemory(redisOpt asynq.RedisClientOpt) error {
	fmt.Println("🔍 Test 8: Testing Redis memory usage...")

	var rdb *redis.Client
	if redisOpt.TLSConfig != nil {
		rdb = redis.NewClient(&redis.Options{
			Addr:      redisOpt.Addr,
			Password:  redisOpt.Password,
			DB:        redisOpt.DB,
			TLSConfig: redisOpt.TLSConfig,
		})
	} else {
		rdb = redis.NewClient(&redis.Options{
			Addr:     redisOpt.Addr,
			Password: redisOpt.Password,
			DB:       redisOpt.DB,
		})
	}
	defer rdb.Close()

	ctx := context.Background()
	info, err := rdb.Info(ctx, "memory").Result()
	if err != nil {
		return fmt.Errorf("❌ Failed to get memory info: %w", err)
	}

	fmt.Printf("✅ Redis memory info retrieved\n")
	fmt.Printf("   %s\n", info[:200]) // Print first 200 chars

	return nil
}

// Helper function
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

// Main test runner
func runSanityTests() {
	fmt.Println("🚀 Starting Asynq Sanity Tests...\n")

	// Create Redis client options
	//redisOpt := createRedisClientOptTLS()
	redisOpt := createRedisClientOpt()
	// Test 1: Redis Connection
	if err := testRedisConnection(redisOpt); err != nil {
		fmt.Printf("%v\n\n", err)
		return
	}
	fmt.Println()

	// Test 2: TLS Connection
	if err := testTLSConnection(redisOpt); err != nil {
		fmt.Printf("%v\n\n", err)
	}
	fmt.Println()

	// Create clients
	client := asynq.NewClient(redisOpt)
	defer client.Close()

	inspector := asynq.NewInspector(redisOpt)
	defer inspector.Close()

	// Test 3: Enqueue Task
	if err := testEnqueueTask(client); err != nil {
		fmt.Printf("%v\n\n", err)
	}
	fmt.Println()

	// Test 4: Schedule Task
	if err := testScheduleTask(client); err != nil {
		fmt.Printf("%v\n\n", err)
	}
	fmt.Println()

	// Test 5: Queue Stats
	if err := testQueueStats(inspector); err != nil {
		fmt.Printf("%v\n\n", err)
	}
	fmt.Println()

	// Test 6: Task Handler
	if err := testTaskHandler(redisOpt); err != nil {
		fmt.Printf("%v\n\n", err)
	}
	fmt.Println()

	// Test 7: Retry Logic
	if err := testRetryLogic(client); err != nil {
		fmt.Printf("%v\n\n", err)
	}
	fmt.Println()

	// Test 8: Redis Memory
	if err := testRedisMemory(redisOpt); err != nil {
		fmt.Printf("%v\n\n", err)
	}
	fmt.Println()

	fmt.Println("✨ Sanity tests completed!")
}

func main() {
	runSanityTests()
}
