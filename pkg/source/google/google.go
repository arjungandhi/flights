// Package google prices routes by fetching the public Google Flights search page and reading
// the result payload it inlines (AF_initDataCallback key ds:1). The JSON RPCs the page's own
// JS uses have been signature-gated since 2026-08; the page itself is not. No API key, no
// TLS impersonation needed as of this writing. Each page is ~2-3 MB, so callers must budget.
package google

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/arjungandhi/flights/pkg/source"
)

const (
	pageURL   = "https://www.google.com/travel/flights"
	userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	// Pre-set EU consent so we never get the consent interstitial instead of results.
	consentCookie = "CONSENT=YES+; SOCS=CAESHAgBEhJnd3NfMjAyMzA4MTAtMF9SQzIaAmVuIAEaBgiA_LyaBg"
	attempts      = 3
)

type Client struct {
	HTTP     *http.Client
	MinGap   time.Duration // pause between requests; jitter of up to MinGap/2 is added
	mu       sync.Mutex
	lastSent time.Time
}

func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 45 * time.Second}, MinGap: 2500 * time.Millisecond}
}

func (c *Client) Name() string { return "google" }

// URL is the human-facing search page for a query; the same page the scanner reads.
func URL(q source.Query) string {
	return pageURL + "?tfs=" + encodeTFS(q) + "&hl=en&gl=US&curr=USD"
}

// LinkURL rebuilds the search page from an Itinerary.Link produced by this client.
func LinkURL(link string) string {
	return pageURL + "?tfs=" + link + "&hl=en&gl=US&curr=USD"
}

func (c *Client) Search(ctx context.Context, q source.Query) (source.Result, error) {
	tfs := encodeTFS(q)
	url := pageURL + "?tfs=" + tfs + "&hl=en&gl=US&curr=USD"
	var lastErr error
	for i := range attempts {
		if err := c.pace(ctx); err != nil {
			return source.Result{}, err
		}
		body, err := c.get(ctx, url)
		if err != nil {
			return source.Result{}, err
		}
		payload, err := extractPayload(body)
		if err == nil {
			res, perr := parse(payload)
			if perr != nil {
				return source.Result{}, perr
			}
			for j := range res.Itineraries {
				res.Itineraries[j].Link = tfs
			}
			return res, nil
		}
		lastErr = err
		if i+1 < attempts {
			select {
			case <-ctx.Done():
				return source.Result{}, ctx.Err()
			case <-time.After(time.Duration(i+1) * time.Second):
			}
		}
	}
	return source.Result{}, fmt.Errorf("%w: %v", source.ErrBlocked, lastErr)
}

func (c *Client) pace(ctx context.Context) error {
	c.mu.Lock()
	wait := time.Until(c.lastSent.Add(c.MinGap + rand.N(c.MinGap/2+1)))
	c.lastSent = time.Now().Add(max(wait, 0))
	c.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
		return nil
	}
}

func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cookie", consentCookie)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google flights: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 16<<20))
}

var (
	blobRe = regexp.MustCompile(`(?s)AF_initDataCallback\((\{.*?\})\);`)
	keyRe  = regexp.MustCompile(`key:\s*'([^']+)'`)
	dataRe = regexp.MustCompile(`(?s)data:(.*?), sideChannel`)
)

// extractPayload finds the ds:1 blob. A 200 without it is a transient page variant (~1 in 60),
// a consent page, or a block; the caller retries.
func extractPayload(html []byte) (json.RawMessage, error) {
	for _, m := range blobRe.FindAllSubmatch(html, -1) {
		k := keyRe.FindSubmatch(m[1])
		if k == nil || string(k[1]) != "ds:1" {
			continue
		}
		d := dataRe.FindSubmatch(m[1])
		if d == nil {
			return nil, fmt.Errorf("ds:1 blob without data")
		}
		return json.RawMessage(d[1]), nil
	}
	return nil, fmt.Errorf("no ds:1 payload in page")
}
