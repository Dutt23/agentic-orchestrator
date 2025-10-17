package clients

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
)

// Ensure MoverCASClient implements CASClient interface
var _ CASClient = (*MoverCASClient)(nil)

// MoverCASClient implements CASClient interface using the mover service
// Provides access to the Rust mover service via Unix socket
type MoverCASClient struct {
	socketPath string
	connPool   chan net.Conn // Connection pool
	mu         sync.Mutex
}

// Put implements CASClient interface - stores data and returns CAS ID
func (m *MoverCASClient) Put(ctx context.Context, data []byte, mediaType string) (string, error) {
	// TODO: Implement via mover WRITE operation
	// For now, fall back to direct implementation or return error
	return "", fmt.Errorf("Put not yet implemented via mover")
}

// Store implements CASClient interface - generic store method
func (m *MoverCASClient) Store(ctx context.Context, data interface{}) (string, error) {
	// TODO: Implement via mover WRITE operation
	return "", fmt.Errorf("Store not yet implemented via mover")
}

func (m *MoverCASClient) Exists(ctx context.Context, casID string) (bool, error) {
	// Quick check via mover
	data, err := m.Get(ctx, casID)
	return err == nil && data != nil, nil
}

// OpCode represents mover operation types
type OpCode byte

const (
	OpRead   OpCode = 0x01
	OpWrite  OpCode = 0x02
	OpSendZC OpCode = 0x03
	OpRecv   OpCode = 0x04
	OpBatch  OpCode = 0x05
)

// NewMoverCASClient creates a new mover-based CAS client from config
func NewMoverCASClient(config *ClientConfig) (*MoverCASClient, error) {
	socketPath := config.MoverSocket
	if socketPath == "" {
		socketPath = "/tmp/mover.sock"
	}

	// Create connection pool (8 connections)
	pool := make(chan net.Conn, 8)

	return &MoverCASClient{
		socketPath: socketPath,
		connPool:   pool,
	}, nil
}

// getConn gets a connection from pool or creates new one
func (m *MoverCASClient) getConn() (net.Conn, error) {
	select {
	case conn := <-m.connPool:
		return conn, nil
	default:
		// Pool empty, create new connection
		return net.Dial("unix", m.socketPath)
	}
}

// releaseConn returns connection to pool
func (m *MoverCASClient) releaseConn(conn net.Conn) {
	select {
	case m.connPool <- conn:
		// Returned to pool
	default:
		// Pool full, close connection
		conn.Close()
	}
}

// Get implements CASClient interface - fetches data from CAS (zero-copy on mover side)
func (m *MoverCASClient) Get(ctx context.Context, casID string) (interface{}, error) {
	conn, err := m.getConn()
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}
	defer m.releaseConn(conn)

	// Send request
	req := MoverRequest{
		Op:     OpRead,
		ID:     []byte(casID),
		Offset: 0,
		Length: 0,
		Data:   nil,
	}

	if err := req.WriteTo(conn); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Read response
	resp, err := ReadMoverResponse(conn)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.Status != 0x00 {
		return nil, fmt.Errorf("mover returned error: status=%d", resp.Status)
	}

	return resp.Data, nil
}

// write writes data to CAS (write-through) - internal helper
func (m *MoverCASClient) write(casID string, data []byte) error {
	conn, err := m.getConn()
	if err != nil {
		return fmt.Errorf("failed to get connection: %w", err)
	}
	defer m.releaseConn(conn)

	req := MoverRequest{
		Op:     OpWrite,
		ID:     []byte(casID),
		Offset: 0,
		Length: 0,
		Data:   data,
	}

	if err := req.WriteTo(conn); err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	resp, err := ReadMoverResponse(conn)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.Status != 0x00 {
		return fmt.Errorf("write failed: status=%d", resp.Status)
	}

	return nil
}

// MoverRequest represents a request to mover service
type MoverRequest struct {
	Op     OpCode
	ID     []byte
	Offset uint64
	Length uint64
	Data   []byte
}

// WriteTo serializes request to writer (matches Rust protocol)
func (r *MoverRequest) WriteTo(w io.Writer) error {
	// Op code
	if err := binary.Write(w, binary.LittleEndian, r.Op); err != nil {
		return err
	}

	// ID length + ID
	if err := binary.Write(w, binary.LittleEndian, uint16(len(r.ID))); err != nil {
		return err
	}
	if _, err := w.Write(r.ID); err != nil {
		return err
	}

	// Offset and length
	if err := binary.Write(w, binary.LittleEndian, r.Offset); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, r.Length); err != nil {
		return err
	}

	// Data length + data
	if err := binary.Write(w, binary.LittleEndian, uint32(len(r.Data))); err != nil {
		return err
	}
	if _, err := w.Write(r.Data); err != nil {
		return err
	}

	return nil
}

// MoverResponse represents a response from mover service
type MoverResponse struct {
	Status byte
	Data   []byte
}

// ReadMoverResponse reads response from mover (matches Rust protocol)
func ReadMoverResponse(r io.Reader) (*MoverResponse, error) {
	// Read status
	var status byte
	if err := binary.Read(r, binary.LittleEndian, &status); err != nil {
		return nil, err
	}

	// Read data length
	var dataLen uint32
	if err := binary.Read(r, binary.LittleEndian, &dataLen); err != nil {
		return nil, err
	}

	// Read data
	data := make([]byte, dataLen)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}

	return &MoverResponse{
		Status: status,
		Data:   data,
	}, nil
}

// Close closes all connections in pool
func (m *MoverCASClient) Close() error {
	close(m.connPool)
	for conn := range m.connPool {
		conn.Close()
	}
	return nil
}
