package grok

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	fhttp "github.com/bogdanfinn/fhttp"
)

const maxJSONResponseSize = 1 << 20

type uploadFileRequest struct {
	FileName     string `json:"fileName"`
	FileMimeType string `json:"fileMimeType"`
	Content      string `json:"content"`
}

type uploadFileResponse struct {
	FileMetadataID string `json:"fileMetadataId"`
	FileURI        string `json:"fileUri"`
}

func (g *GrokUpstream) uploadFile(ctx context.Context, token, fileName, fileMimeType, contentBase64 string) (string, string, error) {
	if strings.TrimSpace(fileName) == "" {
		return "", "", fmt.Errorf("upload file: fileName is required")
	}
	if strings.TrimSpace(fileMimeType) == "" {
		return "", "", fmt.Errorf("upload file: fileMimeType is required")
	}
	if strings.TrimSpace(contentBase64) == "" {
		return "", "", fmt.Errorf("upload file: content is required")
	}

	body, err := json.Marshal(uploadFileRequest{
		FileName:     fileName,
		FileMimeType: fileMimeType,
		Content:      contentBase64,
	})
	if err != nil {
		return "", "", fmt.Errorf("upload file: marshal request: %w", err)
	}

	req, err := fhttp.NewRequestWithContext(ctx, fhttp.MethodPost, g.uploadURL(), bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("upload file: create request: %w", err)
	}
	req.Header = g.buildHeaders(token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.doer.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("upload file: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fhttp.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxJSONResponseSize))
		return "", "", fmt.Errorf("upload file: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxJSONResponseSize))
	if err != nil {
		return "", "", fmt.Errorf("upload file: read response: %w", err)
	}

	var result uploadFileResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", "", fmt.Errorf("upload file: decode response: %w", err)
	}
	if strings.TrimSpace(result.FileMetadataID) == "" {
		return "", "", fmt.Errorf("upload file: missing fileMetadataId")
	}

	return result.FileMetadataID, result.FileURI, nil
}
