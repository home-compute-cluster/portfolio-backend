// Command smoke verifies a deployed backend through its public and internal admin paths.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const maxResponseBytes = 64 << 10

type smokeConfig struct {
	publicURL      *url.URL
	adminOriginURL *url.URL
	adminEdgeURL   *url.URL
	postSlug       string
	assertion      string
}

func main() {
	var publicURL string
	var adminOriginURL string
	var adminEdgeURL string
	var postSlug string
	var assertionFile string
	flag.StringVar(&publicURL, "public-url", "", "public origin, for example https://packetcraft.dev")
	flag.StringVar(&adminOriginURL, "admin-origin-url", "", "trusted origin URL used to exercise Go admin middleware")
	flag.StringVar(&adminEdgeURL, "admin-edge-url", "", "optional Access-protected admin origin")
	flag.StringVar(&postSlug, "post-slug", "", "published post identity reserved for smoke checks")
	flag.StringVar(&assertionFile, "access-assertion-file", "", "file containing a short-lived Cloudflare Access assertion")
	flag.Parse()

	config, err := parseConfig(publicURL, adminOriginURL, adminEdgeURL, postSlug, assertionFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "smoke configuration:", err)
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 8 * time.Second}
	if err := run(ctx, config, client); err != nil {
		fmt.Fprintln(os.Stderr, "smoke failed:", err)
		os.Exit(1)
	}
	fmt.Println("deployment smoke checks passed")
}

// parseConfig validates command inputs and loads the assertion without exposing it in the process list.
func parseConfig(publicRaw, adminOriginRaw, adminEdgeRaw, postSlug, assertionFile string) (smokeConfig, error) {
	publicURL, err := parseBaseURL("public URL", publicRaw)
	if err != nil {
		return smokeConfig{}, err
	}
	adminOriginURL, err := parseBaseURL("admin origin URL", adminOriginRaw)
	if err != nil {
		return smokeConfig{}, err
	}
	var adminEdgeURL *url.URL
	if adminEdgeRaw != "" {
		adminEdgeURL, err = parseBaseURL("admin edge URL", adminEdgeRaw)
		if err != nil {
			return smokeConfig{}, err
		}
	}
	if strings.TrimSpace(postSlug) == "" {
		return smokeConfig{}, errors.New("post slug is required")
	}
	if assertionFile == "" {
		return smokeConfig{}, errors.New("access assertion file is required")
	}
	assertionBytes, err := os.ReadFile(assertionFile)
	if err != nil {
		return smokeConfig{}, fmt.Errorf("read access assertion file: %w", err)
	}
	assertion := strings.TrimSpace(string(assertionBytes))
	if assertion == "" || len(assertion) > 16<<10 {
		return smokeConfig{}, errors.New("access assertion file must contain one assertion of at most 16 KiB")
	}
	return smokeConfig{
		publicURL:      publicURL,
		adminOriginURL: adminOriginURL,
		adminEdgeURL:   adminEdgeURL,
		postSlug:       postSlug,
		assertion:      assertion,
	}, nil
}

func parseBaseURL(name, raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("%s must be an absolute HTTP(S) URL without a query or fragment", name)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("%s must use HTTP(S)", name)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, nil
}

// run exercises public reads/writes, Access rejection, and authenticated moderation in one bounded workflow.
func run(ctx context.Context, config smokeConfig, client *http.Client) error {
	if client == nil {
		return errors.New("HTTP client is required")
	}
	public := newRequester(client, config.publicURL, "")
	admin := newRequester(client, config.adminOriginURL, config.assertion)
	unauthenticatedAdmin := newRequester(client, config.adminOriginURL, "")
	forgedAdmin := newRequester(client, config.adminOriginURL, "forged.smoke.assertion")

	if err := public.expect(ctx, http.MethodGet, "/api/healthz", nil, http.StatusOK); err != nil {
		return err
	}
	if err := public.expect(ctx, http.MethodGet, "/api/readyz", nil, http.StatusOK); err != nil {
		return err
	}
	postPath := "/api/posts/" + url.PathEscape(config.postSlug)
	if err := public.expect(ctx, http.MethodGet, postPath+"/stats", nil, http.StatusOK); err != nil {
		return err
	}
	if err := public.expect(ctx, http.MethodPost, postPath+"/view", nil, http.StatusNoContent); err != nil {
		return err
	}
	if err := public.expect(ctx, http.MethodPut, postPath+"/like", nil, http.StatusNoContent); err != nil {
		return err
	}
	if err := public.expect(ctx, http.MethodDelete, postPath+"/like", nil, http.StatusNoContent); err != nil {
		return err
	}

	marker := fmt.Sprintf("deployment smoke %d", time.Now().UTC().UnixNano())
	createdBody, err := public.expectJSON(ctx, http.MethodPost, postPath+"/comments", map[string]string{
		"author_name": "deployment-smoke",
		"body":        marker,
		"website":     "",
	}, http.StatusCreated)
	if err != nil {
		return err
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(createdBody, &created); err != nil || created.ID <= 0 {
		return errors.New("comment create returned an invalid identifier")
	}
	publicCommentsBody, err := public.expectJSON(ctx, http.MethodGet, postPath+"/comments?limit=100", nil, http.StatusOK)
	if err != nil {
		return err
	}
	if !commentPageContains(publicCommentsBody, created.ID) {
		return errors.New("created comment was not present in the public comment page")
	}

	if err := unauthenticatedAdmin.expect(ctx, http.MethodGet, "/api/admin/comments", nil, http.StatusUnauthorized); err != nil {
		return fmt.Errorf("go missing-assertion check: %w", err)
	}
	if err := forgedAdmin.expect(ctx, http.MethodGet, "/api/admin/comments", nil, http.StatusUnauthorized); err != nil {
		return fmt.Errorf("go forged-assertion check: %w", err)
	}
	adminCommentsBody, err := admin.expectJSON(ctx, http.MethodGet, "/api/admin/comments?limit=100", nil, http.StatusOK)
	if err != nil {
		return err
	}
	if !commentPageContains(adminCommentsBody, created.ID) {
		return errors.New("created comment was not present in the admin comment page")
	}
	commentPath := fmt.Sprintf("/api/admin/comments/%d", created.ID)
	if err := admin.expect(ctx, http.MethodPost, commentPath+"/hide", nil, http.StatusOK); err != nil {
		return err
	}
	if err := admin.expect(ctx, http.MethodPost, commentPath+"/unhide", nil, http.StatusOK); err != nil {
		return err
	}
	// Leave deployment-generated content hidden after proving both transitions.
	if err := admin.expect(ctx, http.MethodPost, commentPath+"/hide", nil, http.StatusOK); err != nil {
		return err
	}

	if config.adminEdgeURL != nil {
		edge := newRequester(&http.Client{
			Timeout: client.Timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}, config.adminEdgeURL, "")
		status, err := edge.status(ctx, http.MethodGet, "/api/admin/comments", nil)
		if err != nil {
			return fmt.Errorf("access edge check: %w", err)
		}
		if status >= 200 && status < 300 {
			return errors.New("access edge allowed an unauthenticated admin request")
		}
	}
	return nil
}

func commentPageContains(body []byte, id int64) bool {
	var page struct {
		Comments []struct {
			ID int64 `json:"id"`
		} `json:"comments"`
	}
	if json.Unmarshal(body, &page) != nil {
		return false
	}
	for _, comment := range page.Comments {
		if comment.ID == id {
			return true
		}
	}
	return false
}

type requester struct {
	client    *http.Client
	base      *url.URL
	assertion string
}

func newRequester(client *http.Client, base *url.URL, assertion string) requester {
	return requester{client: client, base: base, assertion: assertion}
}

func (requester requester) expect(
	ctx context.Context,
	method string,
	path string,
	body any,
	wantStatus int,
) error {
	status, err := requester.status(ctx, method, path, body)
	if err != nil {
		return err
	}
	if status != wantStatus {
		return fmt.Errorf("%s %s returned %d, want %d", method, path, status, wantStatus)
	}
	return nil
}

func (requester requester) status(ctx context.Context, method, path string, body any) (int, error) {
	response, _, err := requester.do(ctx, method, path, body)
	if err != nil {
		return 0, err
	}
	return response.StatusCode, nil
}

func (requester requester) expectJSON(
	ctx context.Context,
	method string,
	path string,
	body any,
	wantStatus int,
) ([]byte, error) {
	response, responseBody, err := requester.do(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != wantStatus {
		return nil, fmt.Errorf("%s %s returned %d, want %d", method, path, response.StatusCode, wantStatus)
	}
	return responseBody, nil
}

func (requester requester) do(ctx context.Context, method, path string, body any) (*http.Response, []byte, error) {
	var bodyReader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, nil, fmt.Errorf("encode %s %s request: %w", method, path, err)
		}
		bodyReader = bytes.NewReader(encoded)
	}
	endpoint := *requester.base
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + strings.SplitN(path, "?", 2)[0]
	if split := strings.SplitN(path, "?", 2); len(split) == 2 {
		endpoint.RawQuery = split[1]
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bodyReader)
	if err != nil {
		return nil, nil, fmt.Errorf("build %s %s request: %w", method, path, err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("User-Agent", "portfolio-backend-deployment-smoke/1")
	if requester.assertion != "" {
		request.Header.Set("Cf-Access-Jwt-Assertion", requester.assertion)
	}
	response, err := requester.client.Do(request)
	if err != nil {
		return nil, nil, fmt.Errorf("perform %s %s: %w", method, path, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, nil, fmt.Errorf("read %s %s response: %w", method, path, err)
	}
	if len(responseBody) > maxResponseBytes {
		return nil, nil, fmt.Errorf("%s %s response exceeds %d bytes", method, path, maxResponseBytes)
	}
	return response, responseBody, nil
}
