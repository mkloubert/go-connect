// Copyright © 2026 Marcel Joachim Kloubert <marcel@kloubert.dev>
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package tunnel

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"

	pb "github.com/mkloubert/go-connect/pb"
	"github.com/mkloubert/go-connect/pkg/protocol"
)

// Connector represents the connector side of the tunnel. It connects
// to the broker with a connection ID, then listens on a local port.
// Each accepted local connection triggers an OpenStream to the listener
// side via the broker, creating a bidirectional data tunnel.
type Connector struct {
	brokerAddr     string
	connectionID   string
	localPort      string
	brokerConn     net.Conn
	session        *protocol.Session
	nextStreamID   atomic.Uint32
	streams        map[uint32]net.Conn
	streamsMu      sync.Mutex
	pendingStreams  map[uint32]chan bool
	pendingMu      sync.Mutex
	localListener  net.Listener
	closeCh        chan struct{}
	closeOnce      sync.Once
}

// NewConnector creates a new Connector that will bridge between local
// connections on localPort and the remote listener via the broker.
func NewConnector(brokerAddr, connectionID, localPort string) *Connector {
	return &Connector{
		brokerAddr:    brokerAddr,
		connectionID:  connectionID,
		localPort:     localPort,
		streams:       make(map[uint32]net.Conn),
		pendingStreams: make(map[uint32]chan bool),
		closeCh:       make(chan struct{}),
	}
}

// Start connects to the broker, performs the handshake, sends a
// ConnectRequest, and starts listening for local connections.
func (c *Connector) Start() error {
	conn, err := net.Dial("tcp", c.brokerAddr)
	if err != nil {
		return fmt.Errorf("connector: failed to dial broker at %s: %w", c.brokerAddr, err)
	}

	session, err := protocol.PerformHandshake(conn)
	if err != nil {
		conn.Close()
		return fmt.Errorf("connector: handshake failed: %w", err)
	}

	c.brokerConn = conn
	c.session = session

	// Send ConnectRequest with the connection ID.
	err = c.session.Send(&pb.Envelope{
		Payload: &pb.Envelope_ConnectRequest{
			ConnectRequest: &pb.ConnectRequest{
				ConnectionId: c.connectionID,
			},
		},
	})
	if err != nil {
		conn.Close()
		return fmt.Errorf("connector: failed to send connect request: %w", err)
	}

	// Receive ConnectAck.
	env, err := c.session.Receive()
	if err != nil {
		conn.Close()
		return fmt.Errorf("connector: failed to receive connect ack: %w", err)
	}

	ack := env.GetConnectAck()
	if ack == nil {
		conn.Close()
		return fmt.Errorf("connector: expected ConnectAck, got %T", env.GetPayload())
	}

	if !ack.GetSuccess() {
		conn.Close()
		return fmt.Errorf("connector: connection rejected: %s", ack.GetMessage())
	}

	// Listen on local port for incoming connections.
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", c.localPort))
	if err != nil {
		conn.Close()
		return fmt.Errorf("connector: failed to listen on port %s: %w", c.localPort, err)
	}

	c.localListener = ln

	go c.handleMessages()
	go c.acceptLoop()

	return nil
}

// acceptLoop accepts incoming local connections and spawns a handler
// for each one.
func (c *Connector) acceptLoop() {
	for {
		conn, err := c.localListener.Accept()
		if err != nil {
			select {
			case <-c.closeCh:
				return
			default:
				log.Printf("connector: accept error: %v", err)
				continue
			}
		}

		go c.handleLocalConnection(conn)
	}
}

// handleLocalConnection assigns a stream ID to the local connection,
// sends an OpenStream request to the remote listener, waits for
// OpenStreamAck, and only then starts pumping data.
func (c *Connector) handleLocalConnection(localConn net.Conn) {
	streamID := c.nextStreamID.Add(1)

	// Create a channel to wait for OpenStreamAck
	ackCh := make(chan bool, 1)
	c.pendingMu.Lock()
	c.pendingStreams[streamID] = ackCh
	c.pendingMu.Unlock()

	c.streamsMu.Lock()
	c.streams[streamID] = localConn
	c.streamsMu.Unlock()

	err := c.session.Send(&pb.Envelope{
		Payload: &pb.Envelope_OpenStream{
			OpenStream: &pb.OpenStream{
				StreamId: streamID,
			},
		},
	})
	if err != nil {
		log.Printf("connector: failed to send OpenStream for stream %d: %v", streamID, err)
		localConn.Close()
		c.removeStream(streamID)
		c.removePending(streamID)
		return
	}

	// Wait for OpenStreamAck before starting the data pump
	select {
	case success := <-ackCh:
		if !success {
			log.Printf("connector: stream %d rejected by listener", streamID)
			localConn.Close()
			c.removeStream(streamID)
			return
		}
	case <-c.closeCh:
		localConn.Close()
		c.removeStream(streamID)
		return
	}

	go c.pumpLocalToRemote(streamID, localConn)
}

// removePending removes a pending stream channel from the map.
func (c *Connector) removePending(streamID uint32) {
	c.pendingMu.Lock()
	delete(c.pendingStreams, streamID)
	c.pendingMu.Unlock()
}

// pumpLocalToRemote reads data from the local connection in chunks and
// sends it as Data envelopes to the remote listener via the broker. On
// EOF or error, it sends a CloseStream message.
func (c *Connector) pumpLocalToRemote(streamID uint32, localConn net.Conn) {
	buf := make([]byte, chunkSize)

	for {
		select {
		case <-c.closeCh:
			return
		default:
		}

		n, err := localConn.Read(buf)
		if n > 0 {
			sendErr := c.session.Send(&pb.Envelope{
				Payload: &pb.Envelope_Data{
					Data: &pb.Data{
						StreamId: streamID,
						Payload:  append([]byte(nil), buf[:n]...),
					},
				},
			})
			if sendErr != nil {
				log.Printf("connector: failed to send data for stream %d: %v", streamID, sendErr)
				c.closeStream(streamID)
				return
			}
		}

		if err != nil {
			if err != io.EOF {
				log.Printf("connector: local read error for stream %d: %v", streamID, err)
			}
			// Notify remote side that the stream is closed.
			_ = c.session.Send(&pb.Envelope{
				Payload: &pb.Envelope_CloseStream{
					CloseStream: &pb.CloseStream{
						StreamId: streamID,
					},
				},
			})
			c.removeStream(streamID)
			return
		}
	}
}

// handleMessages continuously receives envelopes from the broker and
// dispatches them based on their payload type.
func (c *Connector) handleMessages() {
	defer c.Close()

	for {
		select {
		case <-c.closeCh:
			return
		default:
		}

		env, err := c.session.Receive()
		if err != nil {
			log.Printf("connector: receive error: %v", err)
			return
		}

		switch {
		case env.GetOpenStreamAck() != nil:
			ack := env.GetOpenStreamAck()
			c.pendingMu.Lock()
			ch, ok := c.pendingStreams[ack.GetStreamId()]
			if ok {
				delete(c.pendingStreams, ack.GetStreamId())
			}
			c.pendingMu.Unlock()

			if ok && ch != nil {
				ch <- ack.GetSuccess()
			}
			if !ack.GetSuccess() {
				log.Printf("connector: OpenStream rejected for stream %d: %s", ack.GetStreamId(), ack.GetMessage())
			}

		case env.GetData() != nil:
			c.handleData(env.GetData())

		case env.GetCloseStream() != nil:
			streamID := env.GetCloseStream().GetStreamId()
			c.closeStream(streamID)

		case env.GetHeartbeat() != nil:
			_ = c.session.Send(&pb.Envelope{
				Payload: &pb.Envelope_Heartbeat{
					Heartbeat: &pb.Heartbeat{},
				},
			})

		case env.GetDisconnect() != nil:
			log.Printf("connector: received disconnect: %s", env.GetDisconnect().GetReason())
			return

		default:
			log.Printf("connector: unexpected message type: %T", env.GetPayload())
		}
	}
}

// handleData writes the received payload to the local stream connection
// identified by the stream ID.
func (c *Connector) handleData(data *pb.Data) {
	c.streamsMu.Lock()
	conn, ok := c.streams[data.GetStreamId()]
	c.streamsMu.Unlock()

	if !ok {
		return
	}

	_, err := conn.Write(data.GetPayload())
	if err != nil {
		log.Printf("connector: failed to write to local stream %d: %v", data.GetStreamId(), err)
		c.closeStream(data.GetStreamId())
	}
}

// closeStream closes the local connection for the given stream ID
// and removes it from the map.
func (c *Connector) closeStream(streamID uint32) {
	c.streamsMu.Lock()
	conn, ok := c.streams[streamID]
	if ok {
		delete(c.streams, streamID)
	}
	c.streamsMu.Unlock()

	if ok && conn != nil {
		conn.Close()
	}
}

// removeStream removes a stream from the map without closing the
// connection (used when the connection is already closed).
func (c *Connector) removeStream(streamID uint32) {
	c.streamsMu.Lock()
	delete(c.streams, streamID)
	c.streamsMu.Unlock()
}

// Close shuts down the connector, closing the local listener, all
// stream connections, and signaling goroutines to stop.
func (c *Connector) Close() {
	c.closeOnce.Do(func() {
		close(c.closeCh)

		// Close the broker connection to unblock any pending reads
		// in the handleMessages goroutine.
		if c.brokerConn != nil {
			c.brokerConn.Close()
		}

		if c.localListener != nil {
			c.localListener.Close()
		}

		c.streamsMu.Lock()
		for id, conn := range c.streams {
			if conn != nil {
				conn.Close()
			}
			delete(c.streams, id)
		}
		c.streamsMu.Unlock()
	})
}

// Done returns a channel that is closed when the connector is shut down.
func (c *Connector) Done() <-chan struct{} {
	return c.closeCh
}
