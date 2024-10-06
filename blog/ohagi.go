package blog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"

	"github.com/paralleltree/markov-bot-go/lib"
)

const (
	OhagiPostVisibilityPublic        = "public"
	OhagiPostVisibilitySemiPublic    = "semi-public"
	OhagiPostVisibilityFollowersOnly = "followers-only"
	OhagiPostVisibilityDirect        = "direct"
)

type OhagiClient struct {
	Origin         string
	AccessToken    string
	PostVisibility string
	client         *http.Client
}

var _ BlogClient = &OhagiClient{}

func NewOhagiClient(origin, accessToken, postVisibility string) *OhagiClient {
	return &OhagiClient{
		Origin:         origin,
		AccessToken:    accessToken,
		PostVisibility: postVisibility,
		client:         &http.Client{},
	}
}

func (c *OhagiClient) GetPostsFetcher(ctx context.Context) lib.ChunkIteratorFunc[string] {
	accountID := ""
	paginationID := ""

	return func() ([]string, bool, error) {
		if accountID == "" {
			id, err := c.fetchCredentialAccountID(ctx)
			if err != nil {
				return nil, false, fmt.Errorf("fetch account id: %w", err)
			}
			accountID = id
		}

		posts, hasNext, nextPaginationID, err := c.fetchPosts(ctx, accountID, paginationID)
		if err != nil {
			return nil, false, err
		}

		paginationID = nextPaginationID
		return posts, hasNext, nil
	}
}

func (c *OhagiClient) CreatePost(ctx context.Context, payload string) error {
	post := &struct {
		Text       string `json:"text"`
		Visibility string `json:"visibility"`
	}{
		Text:       fmt.Sprintf("%s #bot", payload),
		Visibility: c.PostVisibility,
	}

	u, err := c.buildURL("/api/v1/posts")
	if err != nil {
		return err
	}

	body, err := json.Marshal(post)
	if err != nil {
		return err
	}

	req, err := c.createRequest(ctx, "POST", u.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	res, err := c.client.Do(req)
	if err != nil {
		return err
	}

	if res.StatusCode != 200 {
		return fmt.Errorf("post failed: (status_code: %d, status: %s)", res.StatusCode, res.Status)
	}

	return nil
}

func (c *OhagiClient) fetchPosts(ctx context.Context, accountID string, paginationID string) ([]string, bool, string, error) {
	u, err := c.buildURL(fmt.Sprintf("/api/v1/accounts/%s/posts", accountID))
	if err != nil {
		return nil, false, "", err
	}

	queries := u.Query()
	if paginationID != "" {
		queries.Set("lt_id", paginationID)
	}
	queries.Set("order", "desc")
	queries.Set("exclude_replies", "true")
	queries.Set("exclude_reblogs", "true")
	u.RawQuery = queries.Encode()

	req, err := c.createRequest(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, false, "", err
	}

	res, err := c.client.Do(req)
	if err != nil {
		return nil, false, "", err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, false, "", err
	}

	posts := []struct {
		ID           string `json:"id"`
		PaginationID string `json:"pagination_id"`
		Content      string `json:"content"`
		Visibility   string `json:"visibility"`
	}{}

	if err := json.Unmarshal(body, &posts); err != nil {
		return nil, false, "", fmt.Errorf("unmarshal response: %w(%s)", err, body)
	}

	if len(posts) == 0 {
		return nil, false, "", nil
	}

	tagPattern := regexp.MustCompile(`<[^>]*?>`)
	result := make([]string, 0, len(posts))
	for _, v := range posts {
		if v.Visibility == OhagiPostVisibilityFollowersOnly || v.Visibility == OhagiPostVisibilityDirect {
			continue
		}

		// remove tags
		result = append(result, html.UnescapeString(tagPattern.ReplaceAllLiteralString(v.Content, "")))
	}

	return result, true, posts[len(posts)-1].PaginationID, nil
}

func (c *OhagiClient) fetchCredentialAccountID(ctx context.Context) (string, error) {
	u, err := c.buildURL("/api/v1/session/credential_account")
	if err != nil {
		return "", err
	}

	req, err := c.createRequest(ctx, "GET", u.String(), nil)
	if err != nil {
		return "", err
	}

	res, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", nil
	}

	account := &struct {
		ID string `json:"id"`
	}{}

	if err := json.Unmarshal(body, account); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	return account.ID, nil
}

func (c *OhagiClient) createRequest(ctx context.Context, method string, apiEndpointURL string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, apiEndpointURL, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.AccessToken))

	return req, nil
}

func (c *OhagiClient) buildURL(apiPath string) (*url.URL, error) {
	return url.Parse(fmt.Sprintf("%s%s", c.Origin, apiPath))
}
