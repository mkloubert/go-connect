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
	"time"

	pb "github.com/mkloubert/go-connect/pb"
	"github.com/mkloubert/go-connect/pkg/protocol"
)

const (
	// chunkSize is the size of the read buffer for pumping data
	// from local connections to the remote peer via the broker.
	chunkSize = 32 * 1024 // 32 KB

	// DialTimeout is the maximum time to wait for a TCP connection
	// to be established (broker or local service).
	DialTimeout = 10 * time.Second

	// OpenStreamAckTimeout is the maximum time to wait for an
	// OpenStreamAck response from the remote listener before
	// giving up and closing the local connection.
	OpenStreamAckTimeout = 15 * time.Second
)

// Listener represents the listener side of the tunnel. It connects
// to the broker, registers with a connection ID, and when a connector
// opens a stream, it dials the local port to relay data back and forth.
type Listener struct {
	localPort    string
	brokerAddr   string
	brokerConn   net.Conn
	session      *protocol.Session
	connectionID string
	streams      map[uint32]net.Conn
	streamsMu    sync.Mutex
	closeCh      chan struct{}
	closeOnce    sync.Once
}

// NewListener creates a new Listener that will forward traffic between
// the broker and the local service running on localPort.
func NewListener(localPort, brokerAddr, connectionID string) *Listener {
	return &Listener{
		localPort:    localPort,
		brokerAddr:   brokerAddr,
		connectionID: connectionID,
		streams:      make(map[uint32]net.Conn),
		closeCh:      make(chan struct{}),
	}
}

// Start connects to the broker, performs the handshake, registers with
// the connection ID, and launches the message handling goroutine.
func (l *Listener) Start() error {
	conn, err := net.DialTimeout("tcp", l.brokerAddr, DialTimeout)
	if err != nil {
		return fmt.Errorf("listener: failed to dial broker at %s: %w", l.brokerAddr, err)
	}

	session, err := protocol.PerformHandshake(conn)
	if err != nil {
		conn.Close()
		return fmt.Errorf("listener: handshake failed: %w", err)
	}

	l.brokerConn = conn
	l.session = session

	// Send Register message with the connection ID.
	err = l.session.Send(&pb.Envelope{
		Payload: &pb.Envelope_Register{
			Register: &pb.Register{
				ConnectionId: l.connectionID,
			},
		},
	})
	if err != nil {
		conn.Close()
		return fmt.Errorf("listener: failed to send register: %w", err)
	}

	// Receive RegisterAck.
	env, err := l.session.Receive()
	if err != nil {
		conn.Close()
		return fmt.Errorf("listener: failed to receive register ack: %w", err)
	}

	ack := env.GetRegisterAck()
	if ack == nil {
		conn.Close()
		return fmt.Errorf("listener: expected RegisterAck, got %T", env.GetPayload())
	}

	if !ack.GetSuccess() {
		conn.Close()
		return fmt.Errorf("listener: registration rejected: %s", ack.GetMessage())
	}

	go l.handleMessages()

	return nil
}

// handleMessages continuously receives envelopes from the broker and
// dispatches them based on their payload type.
func (l *Listener) handleMessages() {
	defer l.Close()

	for {
		select {
		case <-l.closeCh:
			return
		default:
		}

		env, err := l.session.Receive()
		if err != nil {
			log.Printf("listener: receive error: %v", err)
			return
		}

		switch {
		case env.GetOpenStream() != nil:
			streamID := env.GetOpenStream().GetStreamId()
			go l.handleOpenStream(streamID)

		case env.GetData() != nil:
			l.handleData(env.GetData())

		case env.GetCloseStream() != nil:
			streamID := env.GetCloseStream().GetStreamId()
			l.closeStream(streamID)

		case env.GetHeartbeat() != nil:
			_ = l.session.Send(&pb.Envelope{
				Payload: &pb.Envelope_Heartbeat{
					Heartbeat: &pb.Heartbeat{},
				},
			})

		case env.GetDisconnect() != nil:
			log.Printf("listener: received disconnect: %s", env.GetDisconnect().GetReason())
			return

		default:
			log.Printf("listener: unexpected message type: %T", env.GetPayload())
		}
	}
}

// handleOpenStream dials the local port, stores the connection in the
// streams map, sends an OpenStreamAck, and starts pumping data from
// the local connection to the remote peer.
func (l *Listener) handleOpenStream(streamID uint32) {
	localConn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", l.localPort), DialTimeout)
	if err != nil {
		log.Printf("listener: failed to dial local port %s for stream %d: %v", l.localPort, streamID, err)
		_ = l.session.Send(&pb.Envelope{
			Payload: &pb.Envelope_OpenStreamAck{
				OpenStreamAck: &pb.OpenStreamAck{
					StreamId: streamID,
					Success:  false,
					Message:  fmt.Sprintf("failed to connect to local port: %v", err),
				},
			},
		})
		return
	}

	l.streamsMu.Lock()
	l.streams[streamID] = localConn
	l.streamsMu.Unlock()

	_ = l.session.Send(&pb.Envelope{
		Payload: &pb.Envelope_OpenStreamAck{
			OpenStreamAck: &pb.OpenStreamAck{
				StreamId: streamID,
				Success:  true,
				Message:  "stream opened",
			},
		},
	})

	go l.pumpLocalToRemote(streamID, localConn)
}

// pumpLocalToRemote reads data from the local connection in chunks and
// sends it as Data envelopes to the remote peer via the broker. On EOF
// or error, it sends a CloseStream message.
func (l *Listener) pumpLocalToRemote(streamID uint32, localConn net.Conn) {
	buf := make([]byte, chunkSize)

	for {
		select {
		case <-l.closeCh:
			return
		default:
		}

		n, err := localConn.Read(buf)
		if n > 0 {
			sendErr := l.session.Send(&pb.Envelope{
				Payload: &pb.Envelope_Data{
					Data: &pb.Data{
						StreamId: streamID,
						Payload:  append([]byte(nil), buf[:n]...),
					},
				},
			})
			if sendErr != nil {
				log.Printf("listener: failed to send data for stream %d: %v", streamID, sendErr)
				l.closeStream(streamID)
				return
			}
		}

		if err != nil {
			if err != io.EOF {
				log.Printf("listener: local read error for stream %d: %v", streamID, err)
			}
			// Notify remote side that the stream is closed.
			_ = l.session.Send(&pb.Envelope{
				Payload: &pb.Envelope_CloseStream{
					CloseStream: &pb.CloseStream{
						StreamId: streamID,
					},
				},
			})
			l.removeStream(streamID)
			return
		}
	}
}

// handleData writes the received payload to the local stream connection
// identified by the stream ID.
func (l *Listener) handleData(data *pb.Data) {
	l.streamsMu.Lock()
	conn, ok := l.streams[data.GetStreamId()]
	l.streamsMu.Unlock()

	if !ok {
		return
	}

	_, err := conn.Write(data.GetPayload())
	if err != nil {
		log.Printf("listener: failed to write to local stream %d: %v", data.GetStreamId(), err)
		l.closeStream(data.GetStreamId())
	}
}

// closeStream closes the local connection for the given stream ID,
// removes it from the map, and notifies the remote side.
func (l *Listener) closeStream(streamID uint32) {
	l.streamsMu.Lock()
	conn, ok := l.streams[streamID]
	if ok {
		delete(l.streams, streamID)
	}
	l.streamsMu.Unlock()

	if ok && conn != nil {
		conn.Close()
	}
}

// removeStream removes a stream from the map without closing the
// connection (used when the connection is already closed).
func (l *Listener) removeStream(streamID uint32) {
	l.streamsMu.Lock()
	delete(l.streams, streamID)
	l.streamsMu.Unlock()
}

// Close shuts down the listener, closing all stream connections and
// signaling the message handler to stop.
func (l *Listener) Close() {
	l.closeOnce.Do(func() {
		close(l.closeCh)

		// Close the broker connection to unblock any pending reads
		// in the handleMessages goroutine.
		if l.brokerConn != nil {
			l.brokerConn.Close()
		}

		l.streamsMu.Lock()
		for id, conn := range l.streams {
			if conn != nil {
				conn.Close()
			}
			delete(l.streams, id)
		}
		l.streamsMu.Unlock()
	})
}

// Done returns a channel that is closed when the listener is shut down.
func (l *Listener) Done() <-chan struct{} {
	return l.closeCh
}
