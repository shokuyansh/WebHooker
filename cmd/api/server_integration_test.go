//go:build integration

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/lib/pq"
)

func TestIntegrationServerShutdown(t *testing.T) {
	if os.Getenv("WEBHOOKER_TEST_SERVER_CHILD") == "1" {
		os.Args = []string{"webhooker", "-port=" + os.Getenv("WEBHOOKER_TEST_SERVER_PORT"), "-db-dsn=" + os.Getenv("WEBHOOKER_TEST_SERVER_DSN"), "-environment=test"}
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
		main()
		os.Exit(0)
	}
	_, db := integrationApplication(t)
	var schema string
	if err := db.QueryRow("SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	dsn := os.Getenv("WEBHOOKER_TEST_DSN")
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		var err error
		dsn, err = pq.ParseURL(dsn)
		if err != nil {
			t.Fatal(err)
		}
	}
	// The child process uses only the parent's isolated test schema.
	dsn += " search_path=" + schema
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestIntegrationServerShutdown$")
	cmd.Env = append(os.Environ(), "WEBHOOKER_TEST_SERVER_CHILD=1", "WEBHOOKER_TEST_SERVER_PORT="+fmt.Sprint(port), "WEBHOOKER_TEST_SERVER_DSN="+dsn)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	joined := false
	t.Cleanup(func() {
		if !joined {
			cmd.Process.Kill()
			<-exited
		}
	})
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/healthcheckup", port)
	probe := &http.Client{Timeout: 200 * time.Millisecond, Transport: &http.Transport{}}
	defer probe.CloseIdleConnections()
	ready := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		select {
		case err := <-exited:
			joined = true
			t.Fatalf("server exited before ready: %v\n%s", err, &output)
		default:
		}
		resp, err := probe.Get(url)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		t.Fatal("server did not become ready")
	}
	// Health checks must remain exempt even with the default limiter enabled.
	for i := 0; i < 10; i++ {
		resp, err := probe.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("health check status=%d", resp.StatusCode)
		}
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-exited:
		joined = true
		if err != nil {
			t.Fatalf("shutdown error=%v\n%s", err, &output)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("graceful shutdown did not finish")
	}
	if !strings.Contains(output.String(), "stopped server") {
		t.Fatalf("shutdown incomplete: %s", &output)
	}
	if _, err := db.Exec("SELECT 1"); err != nil {
		t.Fatalf("child affected parent database pool: %v", err)
	}
}
