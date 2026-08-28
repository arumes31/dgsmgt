package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
)

type remoteClient struct {
	httpClient *http.Client
}

type createRequest struct {
	Name    string   `json:"name"`
	Image   string   `json:"image"`
	Ports   []string `json:"ports"`
	Env     []string `json:"env"`
	Volumes []string `json:"volumes"`
}

type createResponse struct {
	ID string `json:"id"`
}

func NewRemoteService(socketPath string) (*Service, error) {
	cleaned := path.Clean(strings.TrimSpace(socketPath))
	if !path.IsAbs(cleaned) || cleaned == "/" {
		return nil, fmt.Errorf("docker proxy socket must be an absolute non-root path")
	}

	dialer := &net.Dialer{}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", cleaned)
		},
		DisableCompression: true,
	}

	return &Service{
		remote: &remoteClient{
			httpClient: &http.Client{Transport: transport},
		},
	}, nil
}

func (c *remoteClient) getStatus(ctx context.Context, containerID string) (*ContainerInfo, error) {
	var info ContainerInfo
	if err := c.doJSON(ctx, http.MethodGet, resourcePath("status", containerID), nil, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (c *remoteClient) action(ctx context.Context, action, containerID string) error {
	return c.doJSON(ctx, http.MethodPost, resourcePath(action, containerID), nil, nil)
}

func (c *remoteClient) create(ctx context.Context, input createRequest) (string, error) {
	var response createResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/containers", input, &response); err != nil {
		return "", err
	}
	return response.ID, nil
}

func (c *remoteClient) delete(ctx context.Context, containerID string, force bool) error {
	endpoint := resourcePath("containers", containerID) + "?force=" + strconv.FormatBool(force)
	return c.doJSON(ctx, http.MethodDelete, endpoint, nil, nil)
}

func (c *remoteClient) list(ctx context.Context) ([]ContainerInfo, error) {
	containers := []ContainerInfo{}
	if err := c.doJSON(ctx, http.MethodGet, "/v1/containers", nil, &containers); err != nil {
		return nil, err
	}
	return containers, nil
}

func (c *remoteClient) stream(ctx context.Context, operation, containerID, tail string) (io.ReadCloser, error) {
	endpoint := resourcePath(operation, containerID)
	if tail != "" {
		endpoint += "?tail=" + url.QueryEscape(tail)
	}
	// #nosec G704 -- the transport always dials the validated local Unix socket; the URL host is never resolved.
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix"+endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating Docker proxy request: %w", err)
	}
	// #nosec G704 -- httpClient uses the hard-wired Unix-socket DialContext created by NewRemoteService.
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("calling Docker proxy: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		defer func() { _ = response.Body.Close() }()
		return nil, decodeProxyError(response)
	}
	return response.Body, nil
}

func (c *remoteClient) doJSON(
	ctx context.Context,
	method string,
	endpoint string,
	input any,
	output any,
) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encoding Docker proxy request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}

	// #nosec G704 -- the transport always dials the validated local Unix socket; the URL host is never resolved.
	request, err := http.NewRequestWithContext(ctx, method, "http://unix"+endpoint, body)
	if err != nil {
		return fmt.Errorf("creating Docker proxy request: %w", err)
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	// #nosec G704 -- httpClient uses the hard-wired Unix-socket DialContext created by NewRemoteService.
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("calling Docker proxy: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return decodeProxyError(response)
	}
	if output == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(output); err != nil {
		return fmt.Errorf("decoding Docker proxy response: %w", err)
	}
	return nil
}

func decodeProxyError(response *http.Response) error {
	message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	text := strings.TrimSpace(string(message))
	if text == "" {
		text = response.Status
	}
	switch response.StatusCode {
	case http.StatusBadRequest:
		return fmt.Errorf("%w: %s", ErrCreationDenied, text)
	case http.StatusForbidden:
		return fmt.Errorf("%w: %s", ErrUnmanagedContainer, text)
	default:
		return fmt.Errorf("docker proxy returned %s: %s", response.Status, text)
	}
}

func resourcePath(operation, containerID string) string {
	return "/v1/" + operation + "/" + url.PathEscape(containerID)
}
