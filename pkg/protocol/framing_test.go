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
	"bytes"
	"encoding/binary"
	"testing"
)

func TestWriteReadFrame(t *testing.T) {
	payload := []byte("hello, go-connect!")

	var buf bytes.Buffer
	if err := WriteFrame(&buf, payload); err != nil {
		t.Fatalf("WriteFrame failed: %v", err)
	}

	got, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame failed: %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: got %q, want %q", got, payload)
	}
}

func TestWriteReadFrame_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, []byte{}); err != nil {
		t.Fatalf("WriteFrame failed for empty payload: %v", err)
	}

	got, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame failed for empty payload: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("expected empty payload, got %d bytes", len(got))
	}
}

func TestWriteReadFrame_Multiple(t *testing.T) {
	payloads := [][]byte{
		[]byte("first frame"),
		[]byte("second frame with more data"),
		[]byte("third"),
		{0x00, 0x01, 0x02, 0xFF},
	}

	var buf bytes.Buffer
	for i, p := range payloads {
		if err := WriteFrame(&buf, p); err != nil {
			t.Fatalf("WriteFrame failed for frame %d: %v", i, err)
		}
	}

	for i, want := range payloads {
		got, err := ReadFrame(&buf)
		if err != nil {
			t.Fatalf("ReadFrame failed for frame %d: %v", i, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("frame %d mismatch: got %q, want %q", i, got, want)
		}
	}
}

func TestReadFrame_TooLarge(t *testing.T) {
	// Craft a frame header that declares a size exceeding MaxFrameSize.
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, MaxFrameSize+1)

	buf := bytes.NewReader(header)
	_, err := ReadFrame(buf)
	if err == nil {
		t.Fatal("expected error for oversized frame, got nil")
	}
}

func TestWriteFrame_TooLarge(t *testing.T) {
	// Create a payload that exceeds MaxFrameSize.
	payload := make([]byte, MaxFrameSize+1)

	var buf bytes.Buffer
	err := WriteFrame(&buf, payload)
	if err == nil {
		t.Fatal("expected error for oversized payload, got nil")
	}
}
