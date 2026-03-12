package proxy

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// hop-by-hop headers that should not be forwarded (canonical form)
var hopByHop = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

const maxProxyBodySize = 10 << 20      // 10 MB inbound request body limit
const maxResponseBodySize = 50 << 20   // 50 MB outbound response body limit

func ForwardRequest(w http.ResponseWriter, r *http.Request, upstreamURL string, vendorCfg VendorConfig, realKey string, client *http.Client) (int, error) {
	// Limit inbound request body size
	body := http.MaxBytesReader(w, r.Body, maxProxyBodySize)

	upReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, body)
	if err != nil {
		return 0, fmt.Errorf("creating upstream request: %w", err)
	}

	// Parse Connection header for additional hop-by-hop headers
	dynamicHopByHop := make(map[string]bool)
	if conn := r.Header.Get("Connection"); conn != "" {
		for _, h := range strings.Split(conn, ",") {
			dynamicHopByHop[http.CanonicalHeaderKey(strings.TrimSpace(h))] = true
		}
	}

	// Copy headers, skipping hop-by-hop and auth headers
	for k, vv := range r.Header {
		if hopByHop[k] || dynamicHopByHop[k] {
			continue
		}
		// Strip client auth — we inject our own
		if k == "Authorization" || k == "X-Api-Key" {
			continue
		}
		for _, v := range vv {
			upReq.Header.Add(k, v)
		}
	}

	// Inject real auth
	upReq.Header.Set(vendorCfg.AuthHeader, vendorCfg.AuthFormat(realKey))
	upReq.Host = vendorCfg.UpstreamHost

	resp, err := client.Do(upReq)
	if err != nil {
		return 0, fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	// Parse response Connection header for dynamic hop-by-hop
	respDynHop := make(map[string]bool)
	if conn := resp.Header.Get("Connection"); conn != "" {
		for _, h := range strings.Split(conn, ",") {
			respDynHop[http.CanonicalHeaderKey(strings.TrimSpace(h))] = true
		}
	}

	// Copy response headers
	for k, vv := range resp.Header {
		if hopByHop[k] || respDynHop[k] {
			continue
		}
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// Stream the response — use Flusher for SSE responses
	flusher, canFlush := w.(http.Flusher)
	ct := resp.Header.Get("Content-Type")
	isStreaming := strings.HasPrefix(ct, "text/event-stream")

	if isStreaming && canFlush {
		rc := http.NewResponseController(w)
		buf := make([]byte, 4096)
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				// Extend write deadline per chunk so active streams continue
				// but stalled connections time out after 30s of no data.
				if err := rc.SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil {
					log.Printf("failed to set write deadline: %v", err)
				}
				if _, writeErr := w.Write(buf[:n]); writeErr != nil {
					log.Printf("client write error during streaming: %v", writeErr)
					break
				}
				flusher.Flush()
			}
			if readErr != nil {
				break
			}
		}
	} else {
		// Limit response body to prevent unbounded memory usage
		limited := io.LimitReader(resp.Body, maxResponseBodySize)
		if _, err := io.Copy(w, limited); err != nil {
			log.Printf("error copying response body: %v", err)
		}
	}

	return resp.StatusCode, nil
}
