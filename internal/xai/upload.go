package xai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	http "github.com/bogdanfinn/fhttp"
)

const uploadFileAPI = "https://grok.com/rest/app-chat/upload-file"

type uploadFileRequest struct {
	FileName     string `json:"fileName"`
	FileMimeType string `json:"fileMimeType"`
	Content      string `json:"content"`
}

type uploadFileResponse struct {
	FileMetadataID string `json:"fileMetadataId"`
	FileURI        string `json:"fileUri"`
}

// UploadFile uploads an attachment and returns (fileMetadataId, fileUri).
func (c *client) UploadFile(ctx context.Context, fileName, fileMimeType, contentBase64 string) (string, string, error) {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadFileAPI, bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("upload file: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.doRequest(req)
	if err != nil {
		return "", "", fmt.Errorf("upload file: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
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
