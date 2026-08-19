package mpv

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"
)

func TestIPCClientCorrelatesConcurrentRepliesAndEvents(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	client := newIPCClient(clientConn)
	defer client.Close()
	go func() {
		decoder := json.NewDecoder(bufio.NewReader(serverConn))
		encoder := json.NewEncoder(serverConn)
		requests := make([]ipcRequest, 2)
		_ = decoder.Decode(&requests[0])
		_ = decoder.Decode(&requests[1])
		_ = encoder.Encode(map[string]any{"event": "property-change", "name": "pause", "data": true})
		for i := 1; i >= 0; i-- {
			_ = encoder.Encode(map[string]any{"request_id": requests[i].RequestID, "error": "success", "data": requestProperty(requests[i])})
		}
	}()
	results := make(map[string]float64)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, property := range []string{"first", "second"} {
		property := property
		wg.Add(1)
		go func() {
			defer wg.Done()
			var value float64
			if err := client.command(context.Background(), &value, "get_property", property); err != nil {
				t.Error(err)
			}
			mu.Lock()
			results[property] = value
			mu.Unlock()
		}()
	}
	wg.Wait()
	if results["first"] != 11 || results["second"] != 22 {
		t.Fatalf("results = %#v", results)
	}
	if event := <-client.Events(); event.Event != "property-change" {
		t.Fatalf("event = %#v", event)
	}
	_ = serverConn.Close()
}

func requestProperty(request ipcRequest) float64 {
	if len(request.Command) > 1 && request.Command[1] == "first" {
		return 11
	}
	return 22
}
