package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type IssueRequest struct {
	RequestID string `json:"request_id"`
	SKU       string `json:"sku"`
	OrderID   string `json:"order_id"`
}

type IssueResponse struct {
	Status string `json:"status"`
	Code   string `json:"code,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type Supplier interface {
	Issue(ctx context.Context, req IssueRequest) (IssueResponse, error)
}

type HTTPClient struct {
	BaseURL string
	Timeout time.Duration
	Client  *http.Client
}

func NewHTTPClient(baseURL string, timeout time.Duration) *HTTPClient {
	return &HTTPClient{
		BaseURL: baseURL,
		Timeout: timeout,
		Client:  &http.Client{Timeout: timeout},
	}
}

func (c *HTTPClient) Issue(ctx context.Context, req IssueRequest) (IssueResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return IssueResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/issue", bytes.NewReader(body))
	if err != nil {
		return IssueResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return IssueResponse{}, err
	}
	defer resp.Body.Close()

	var issueResp IssueResponse
	if err := json.NewDecoder(resp.Body).Decode(&issueResp); err != nil {
		return IssueResponse{}, err
	}
	if resp.StatusCode >= 400 {
		return issueResp, fmt.Errorf("supplier error: status %d, reason: %s", resp.StatusCode, issueResp.Reason)
	}
	return issueResp, nil
}
