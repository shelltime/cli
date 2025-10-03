package model

import (
	"context"
	"fmt"
	"net/http"
)

// Add these structs to your model package
type OpenTokenPublicKeyResponse struct {
	Data OpenTokenPublicKey `json:"data"`
}

type OpenTokenPublicKey struct {
	ID        int    `json:"id"`
	PublicKey string `json:"publicKey"`
}

// Add this new function
func GetOpenTokenPublicKey(ctx context.Context, endpoint Endpoint, tokenID int) (*OpenTokenPublicKey, error) {
	// Create path with query parameter
	path := fmt.Sprintf("/api/v1/opentoken/publickey?tid=%d", tokenID)

	var response OpenTokenPublicKeyResponse
	err := SendHTTPRequestJSON(HTTPRequestOptions[interface{}, OpenTokenPublicKeyResponse]{
		Context:  ctx,
		Endpoint: endpoint,
		Method:   http.MethodGet,
		Path:     path,
		Payload:  nil, // GET request with no body
		Response: &response,
	})

	if err != nil {
		return nil, err
	}

	return &response.Data, nil
}
