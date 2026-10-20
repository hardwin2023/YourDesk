package signaling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestDirectRetryBusyPreservesTLSAuthentication(t *testing.T) {
	var attempts atomic.Int32
	verified := make(chan error, 1)
	secret := []byte("isolated-direct-retry-fixture")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) <= 2 {
			http.Error(w, "Client busy", http.StatusConflict)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			verified <- err
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		_, data, err := conn.Read(ctx)
		var join Envelope
		if err == nil {
			err = json.Unmarshal(data, &join)
		}
		if err == nil {
			err = join.Verify(secret)
		}
		binding, bindErr := bindingFor(r.TLS)
		if err == nil && (bindErr != nil || join.Binding != binding || join.Kind != KindJoin) {
			err = fmt.Errorf("重試後 TLS exporter／認證工作階段不一致")
		}
		verified <- err
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := DialDirect(ctx, strings.TrimPrefix(server.URL, "https://"), secret)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	select {
	case err := <-verified:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if attempts.Load() != 3 {
		t.Fatalf("attempts=%d", attempts.Load())
	}
}

func TestDirectRetryOnlyBusyAndHonorsCancellation(t *testing.T) {
	for _, status := range []int{http.StatusConflict, http.StatusForbidden, http.StatusTooManyRequests} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				http.Error(w, "unavailable", status)
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			_, err := DialDirect(ctx, strings.TrimPrefix(server.URL, "https://"), []byte("fixture"))
			if err == nil {
				t.Fatal("錯誤回應被接受")
			}
			if status == http.StatusConflict && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(err)
			}
			if attempts.Load() != 1 {
				t.Fatalf("非預期重試：%d", attempts.Load())
			}
		})
	}
}
