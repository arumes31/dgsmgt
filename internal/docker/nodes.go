package docker

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"sync"

	"dgsmgt/internal/models"

	"github.com/docker/docker/client"
)

// Manager holds one Docker service per node. Node ID 0 is the local host
// (env-configured socket); additional nodes are remote Docker hosts over
// TCP (optionally mutual TLS).
type Manager struct {
	mu       sync.RWMutex
	services map[uint]*Service
}

func NewManager(local *Service) *Manager {
	return &Manager{services: map[uint]*Service{0: local}}
}

// Local returns the local (node 0) service.
func (m *Manager) Local() *Service {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.services[0]
}

// For returns the service for a node ID.
func (m *Manager) For(nodeID uint) (*Service, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if svc, ok := m.services[nodeID]; ok {
		return svc, nil
	}
	return nil, fmt.Errorf("node %d not connected", nodeID)
}

// Register connects to a node and stores its client.
func (m *Manager) Register(node models.Node) error {
	opts := []client.Opt{
		client.WithHost(node.Address),
		client.WithAPIVersionNegotiation(),
	}

	if node.TLSCert != "" && node.TLSKey != "" {
		cert, err := tls.X509KeyPair([]byte(node.TLSCert), []byte(node.TLSKey))
		if err != nil {
			return fmt.Errorf("node tls keypair: %w", err)
		}
		tlsCfg := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
		if node.TLSCA != "" {
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM([]byte(node.TLSCA)) {
				return fmt.Errorf("node tls: invalid CA PEM")
			}
			tlsCfg.RootCAs = pool
		}
		httpClient := &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
		}
		opts = append(opts, client.WithHTTPClient(httpClient))
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return err
	}

	m.mu.Lock()
	if old, ok := m.services[node.ID]; ok {
		old.Close() // re-registering a node: don't leak the previous client
	}
	m.services[node.ID] = NewServiceWithClient(cli)
	m.mu.Unlock()
	return nil
}

// Remove drops a node's client.
func (m *Manager) Remove(nodeID uint) {
	if nodeID == 0 {
		return
	}
	m.mu.Lock()
	if old, ok := m.services[nodeID]; ok {
		old.Close()
		delete(m.services, nodeID)
	}
	m.mu.Unlock()
}

// NodeIDs lists connected node IDs.
func (m *Manager) NodeIDs() []uint {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]uint, 0, len(m.services))
	for id := range m.services {
		ids = append(ids, id)
	}
	return ids
}
