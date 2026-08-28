package main

import (
	"context"
	"dgsmgt/internal/config"
	"dgsmgt/internal/docker"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"go.uber.org/zap"
)

const (
	defaultSocketPath = "/run/dgsmgt/docker-proxy.sock"
	clientUID         = 65532
	clientGID         = 65532
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer func() { _ = logger.Sync() }()

	policy, err := docker.NewCreationPolicy(
		config.CommaSeparated(os.Getenv("ALLOWED_IMAGES")),
		config.CommaSeparated(os.Getenv("ALLOWED_VOLUME_ROOTS")),
	)
	if err != nil {
		logger.Fatal("Invalid Docker creation policy", zap.Error(err))
	}
	service, err := docker.NewServiceWithPolicy(policy)
	if err != nil {
		logger.Fatal("Failed to initialize Docker service", zap.Error(err))
	}

	socketPath := os.Getenv("DOCKER_PROXY_SOCKET")
	if socketPath == "" {
		socketPath = defaultSocketPath
	}
	listener, err := listen(socketPath)
	if err != nil {
		logger.Fatal("Failed to initialize Docker proxy socket", zap.Error(err))
	}
	defer func() { _ = listener.Close() }()

	server := &http.Server{
		Handler:           docker.NewProxyHandler(service),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("Docker proxy failed", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Docker proxy shutdown failed", zap.Error(err))
	}
}

func listen(socketPath string) (net.Listener, error) {
	if filepath.Clean(socketPath) != defaultSocketPath {
		return nil, fmt.Errorf("docker proxy socket must be %s", defaultSocketPath)
	}
	// Use only the compiled-in path after validating the caller's value.
	socketPath = defaultSocketPath
	directory := filepath.Dir(defaultSocketPath)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, err
	}
	if err := os.Remove(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chown(socketPath, clientUID, clientGID); err != nil {
		return nil, errors.Join(err, listener.Close())
	}
	// #nosec G302 -- the group write bit is required for the unprivileged app to connect to this Unix socket.
	if err := os.Chmod(socketPath, 0o660); err != nil {
		return nil, errors.Join(err, listener.Close())
	}
	return listener, nil
}
