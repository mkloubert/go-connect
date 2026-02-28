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

package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	// MaxFrameSize is the maximum allowed frame payload size (1 MiB).
	MaxFrameSize = 1 << 20

	// frameLengthSize is the size of the length prefix in bytes (uint32).
	frameLengthSize = 4
)

// WriteFrame writes a length-prefixed frame to w. The frame consists of a
// 4-byte big-endian uint32 length header followed by the payload bytes.
// Returns an error if the payload exceeds MaxFrameSize.
func WriteFrame(w io.Writer, payload []byte) error {
	if len(payload) > MaxFrameSize {
		return fmt.Errorf("payload size %d exceeds maximum frame size %d", len(payload), MaxFrameSize)
	}

	header := make([]byte, frameLengthSize)
	binary.BigEndian.PutUint32(header, uint32(len(payload)))

	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("failed to write frame header: %w", err)
	}

	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return fmt.Errorf("failed to write frame payload: %w", err)
		}
	}

	return nil
}

// ReadFrame reads a length-prefixed frame from r. It reads the 4-byte
// big-endian uint32 length header, validates the size does not exceed
// MaxFrameSize, and then reads the payload bytes.
func ReadFrame(r io.Reader) ([]byte, error) {
	header := make([]byte, frameLengthSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("failed to read frame header: %w", err)
	}

	size := binary.BigEndian.Uint32(header)
	if size > MaxFrameSize {
		return nil, fmt.Errorf("frame size %d exceeds maximum frame size %d", size, MaxFrameSize)
	}

	payload := make([]byte, size)
	if size > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, fmt.Errorf("failed to read frame payload: %w", err)
		}
	}

	return payload, nil
}
