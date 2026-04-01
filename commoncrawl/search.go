package commoncrawl

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const userAgent = "go: kmsec.uk (contact: kmsec-uk@pm.me)"

var (
	defaultRateLimit      time.Duration = time.Duration(300 * time.Millisecond)
	maxRetries            int           = 5
	ErrRateLimitExhausted error         = errors.New("client backed off to the maximum rate limit (>30s) but is still getting unexpected status code")
)

// HTTPError is a custom error that holds the HTTP status code
type HTTPError struct {
	StatusCode int
	Err        error
}

func (e *HTTPError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("HTTP %d: %v", e.StatusCode, e.Err)
	}
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

func (e *HTTPError) Unwrap() error {
	return e.Err
}

// SearchMetadata represents the response from index
// when we request showNumPages=true.
// only care about number of pages tbh.
type SearchMetadata struct {
	Pages int `json:"pages"`
}

type SearchResult struct {
	Record CDXRecord
	Error  error
}

// The Client interacts with
// CommonCrawl
type Client struct {
	client    *http.Client
	limiter   *rate.Limiter
	delay     time.Duration
	baseDelay time.Duration

	mu sync.Mutex
}

// NewClient creates a new client to interact with commoncrawl
func NewClient() *Client {
	// in order to have relaxed body read timeouts but strict header
	// receipt timeout, we need to clone the default transport
	// the Client Timeouet is then set to 0
	// For rate limiting, the default rate-limit is 300ms
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 5 * time.Second
	return &Client{
		client:    &http.Client{Transport: t, Timeout: 0},
		delay:     defaultRateLimit,
		baseDelay: defaultRateLimit,
		limiter:   rate.NewLimiter(rate.Every(defaultRateLimit), 1),
	}
}

// WithRateLimit globally sets the baseline
// time to wait between requests
func (c *Client) WithRateLimit(d time.Duration) *Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.delay = d
	c.baseDelay = d
	c.limiter = rate.NewLimiter(rate.Every(d), 1)
	return c
}

// recoverRate ramps down the
// request interval by cutting the rate limit
// in half until we're back to baseline
func (c *Client) recoverRate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.delay == c.baseDelay {
		return
	}
	c.delay /= 2
	if c.delay < c.baseDelay {
		c.delay = c.baseDelay
	}
	c.limiter.SetLimit(rate.Every(c.delay))
}

// backoff ramps up request interval to ease
// off pressure on CommonCrawl
func (c *Client) backoff() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.delay > 30*time.Second {
		return ErrRateLimitExhausted
	}
	c.delay *= 2
	c.limiter.SetLimit(rate.Every(c.delay))
	return nil
}

func (c *Client) DoReq(req *http.Request) (*http.Response, error) {
	var lastErr error
	for range maxRetries {
		if err := c.limiter.Wait(req.Context()); err != nil {
			return nil, fmt.Errorf("rate limiter aborted: %w", err)
		}
		res, err := c.client.Do(req)
		if err != nil {
			err := c.backoff()
			if err != nil {
				return nil, fmt.Errorf("error backing off: %w: status code %d", err, res.StatusCode)
			}
			lastErr = err
			continue
		}
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			c.recoverRate()
			return res, nil
		}

		// https://commoncrawl.org/faq#:~:text=If%20you%20receive%20HTTP%20503%20responses%2C%20please%20slow%20down%20your%20request%20rate.%20These%20are%20a%20sign%20you%27ve%20exceeded%20the%20acceptable%20request%20rate.%20If%20your%20IP%20is%20temporarily%20blocked%2C%20please%20wait%2024%20hours%20before%20trying%20again.
		if res.StatusCode == http.StatusServiceUnavailable || res.StatusCode == http.StatusTooManyRequests {
			res.Body.Close()

			err := c.backoff()
			if err != nil {
				return nil, fmt.Errorf("error backing off: %w: status code %d", err, res.StatusCode)
			}

			lastErr = &HTTPError{
				StatusCode: res.StatusCode,
				Err:        fmt.Errorf("unexpected status: %s", res.Status),
			}
			continue
		}
		// unexpected status received, therefore we just exit
		res.Body.Close()
		return nil, &HTTPError{
			StatusCode: res.StatusCode,
			Err:        fmt.Errorf("unexpected status: %s", res.Status),
		}

	}
	return nil, fmt.Errorf("max retries exhausted. Last error: %w", lastErr)

}

// Search searches a commoncrawl collection via the index, with a given raw query string.
// For example `c.Search("CC-MAIN-2026-08", "url=https://example.com/*&filter==status:200")`
// will search the February 2026 dataset for results for example.com/*, filtering where the
// status is 200
func (c *Client) Search(collection, rawquery string) (<-chan SearchResult, error) {

	// get number of pages
	req, err := http.NewRequest("GET", "https://index.commoncrawl.org/"+collection+"-index?output=json&"+rawquery+"&showNumPages=true", nil)
	if err != nil {
		return nil, fmt.Errorf("retrieving number of pages: creationg request: %w", err)
	}
	req.Header.Add("user-agent", userAgent)
	res, err := c.DoReq(req)
	if err != nil {
		return nil, fmt.Errorf("retrieving number of pages: doing request: %w", err)
	}
	defer res.Body.Close()

	var sm SearchMetadata
	err = json.NewDecoder(res.Body).Decode(&sm)
	if err != nil {
		return nil, err
	}
	if sm.Pages == 0 {
		return nil, fmt.Errorf("the index says there are 0 pages of results")
	}
	// actually start getting results.
	// i've opted for a single goroutine for this as I don't want to hammer the index.
	ch := make(chan SearchResult, 15000)
	go func() {
		defer close(ch)
		for p := 0; p < sm.Pages; p++ {
			url := "https://index.commoncrawl.org/" + collection + "-index?output=json&page=" + strconv.Itoa(p) + "&" + rawquery
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				ch <- SearchResult{Error: fmt.Errorf("page %d: creating request: %w", p, err)}
				continue
			}
			req.Header.Add("user-agent", userAgent)

			res, err := c.DoReq(req)
			if err != nil {
				ch <- SearchResult{Error: fmt.Errorf("page %d: doing request: %w", p, err)}
				var httpErr *HTTPError
				if errors.As(err, &httpErr) || errors.Is(err, ErrRateLimitExhausted) {
					return
				}
				continue
			}

			scanner := bufio.NewScanner(res.Body)
			for scanner.Scan() {
				var record CDXRecord
				if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
					ch <- SearchResult{Error: err}
					continue
				}
				ch <- SearchResult{Record: record}
			}
			if err := scanner.Err(); err != nil {
				ch <- SearchResult{Error: err}
			}
			res.Body.Close()
		}
	}()

	return ch, nil
}
