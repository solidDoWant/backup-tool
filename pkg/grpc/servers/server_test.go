package servers

import (
	"fmt"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestStartServerListener(t *testing.T) {
	tests := []struct {
		desc                      string
		port                      int
		createConflictingListener bool
		shouldError               bool
	}{
		{
			desc:        "valid port",
			port:        40983,
			shouldError: false,
		},
		{
			desc:        "invalid port",
			port:        -1,
			shouldError: true,
		},
		{
			desc:                      "port already in use",
			port:                      40983,
			createConflictingListener: true,
			shouldError:               true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if tt.createConflictingListener {
				// Create listener to occupy port
				l, err := net.Listen("tcp", fmt.Sprintf(":%d", tt.port))
				assert.NoError(t, err)
				defer l.Close()
			}

			listener, err := startServerListener(tt.port)
			if tt.shouldError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, listener)
			defer listener.Close()

			addr := listener.Addr().(*net.TCPAddr)
			assert.Equal(t, tt.port, addr.Port)
		})
	}
}

func TestRegisterServers(t *testing.T) {
	mockServer := grpc.NewServer()
	registerServers(mockServer)

	// Get list of registered services
	serviceInfo := mockServer.GetServiceInfo()

	// Verify all services are registered
	assert.Contains(t, serviceInfo, "Files")
	assert.Contains(t, serviceInfo, "Postgres")
}

func TestStartServer(t *testing.T) {
	tests := []struct {
		desc                string
		startServerListener func(port int) (net.Listener, string, error)
		shouldError         bool
	}{
		{
			desc: "success",
			startServerListener: func(port int) (net.Listener, string, error) {
				listener, err := net.Listen("tcp", ":0")
				assert.NoError(t, err)
				return listener, listener.Addr().String(), err
			},
		},
		{
			desc: "failure",
			startServerListener: func(port int) (net.Listener, string, error) {
				return nil, "", assert.AnError
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			// Keep a copy of the original functions to restore after test
			originalStartServerListener := startServerListener
			defer func() { startServerListener = originalStartServerListener }()

			originalRegisterServers := registerServers
			defer func() { registerServers = originalRegisterServers }()

			// Replace the functions with the test functions
			var address string
			calledStartServerListener := false
			startServerListener = func(port int) (net.Listener, error) {
				calledStartServerListener = true
				listener, listenerAddress, err := tt.startServerListener(port)
				address = listenerAddress
				return listener, err
			}

			calledRegisterServers := false
			registerServers = func(registrar grpc.ServiceRegistrar) {
				calledRegisterServers = true
				originalRegisterServers(registrar)
			}

			// Start server in goroutine since it blocks
			errChan := make(chan error)
			go func() {
				errChan <- StartServer()
			}()

			// Wait briefly to let server start
			var serverErr error
			select {
			case serverErr = <-errChan:
				// Assume the server has failed to start
			case <-time.After(3 * time.Millisecond):
				// Assume server started successfully
			}

			if tt.shouldError {
				assert.Error(t, serverErr)
				return
			}

			// Verify that all required calls were made
			assert.True(t, calledStartServerListener)
			assert.True(t, calledRegisterServers)

			// Try connecting to verify server is running
			conn, err := net.DialTCP("tcp", nil, net.TCPAddrFromAddrPort(netip.MustParseAddrPort(address)))
			assert.NoError(t, err)
			defer conn.Close()

			assert.NotNil(t, conn)
		})
	}
}
