package model

import (
	"context"
	"time"
)

// SearchCommandEdge represents a command from server search
type SearchCommandEdge struct {
	ID              int     `json:"id"`
	Shell           string  `json:"shell"`
	Command         string  `json:"command"`
	MainCommand     string  `json:"mainCommand"`
	Hostname        string  `json:"hostname"`
	Username        string  `json:"username"`
	Time            float64 `json:"time"`
	EndTime         float64 `json:"endTime"`
	Result          int     `json:"result"`
	IsEncrypted     bool    `json:"isEncrypted"`
	OriginalCommand string  `json:"originalCommand"`
}

// SearchCommandsFilter for filtering search results
type SearchCommandsFilter struct {
	Shell       []string  `json:"shell"`
	MainCommand []string  `json:"mainCommand"`
	Hostname    []string  `json:"hostname"`
	Username    []string  `json:"username"`
	IP          []string  `json:"ip"`
	Result      []int     `json:"result"`
	Time        []float64 `json:"time"`
	SessionID   []float64 `json:"sessionId"`
	Command     string    `json:"command,omitempty"`
}

// SearchCommandsPagination for pagination (cursor-based)
type SearchCommandsPagination struct {
	LastID int `json:"lastId"`
	Limit  int `json:"limit"`
}

// SearchCommandsResult wraps the response
type SearchCommandsResult struct {
	Count int                 `json:"count"`
	Edges []SearchCommandEdge `json:"edges"`
}

// fetchCommandsData wraps the GraphQL data response
type fetchCommandsData struct {
	FetchCommands SearchCommandsResult `json:"fetchCommands"`
}

// fetchCommandsResponse is the complete GraphQL response
type fetchCommandsResponse = GraphQLResponse[fetchCommandsData]

// FetchCommandsFromServer searches commands via GraphQL
func FetchCommandsFromServer(ctx context.Context, endpoint Endpoint, filter *SearchCommandsFilter, pagination *SearchCommandsPagination) (*SearchCommandsResult, error) {
	query := `query fetchCommands($pagination: InputPagination!, $filter: CommandFilter!) {
		fetchCommands(pagination: $pagination, filter: $filter) {
			count
			edges {
				id
				shell
				command
				mainCommand
				hostname
				username
				time
				endTime
				result
				isEncrypted
				originalCommand
			}
		}
	}`

	variables := map[string]interface{}{
		"pagination": pagination,
		"filter":     filter,
	}

	var result fetchCommandsResponse
	err := SendGraphQLRequest(GraphQLRequestOptions[fetchCommandsResponse]{
		Context:   ctx,
		Endpoint:  endpoint,
		Query:     query,
		Variables: variables,
		Response:  &result,
		Timeout:   time.Second * 30,
	})

	if err != nil {
		return nil, err
	}

	return &result.Data.FetchCommands, nil
}
