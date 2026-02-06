package model

import (
	"context"
	"time"
)

// FetchCurrentUserProfileQuery is the GraphQL query for fetching the current user's profile
const FetchCurrentUserProfileQuery = `query fetchCurrentUserProfile {
	fetchUser {
		id
		login
	}
}`

// UserProfileResponse is the GraphQL response structure for user profile
type UserProfileResponse struct {
	FetchUser struct {
		ID    int    `json:"id"`
		Login string `json:"login"`
	} `json:"fetchUser"`
}

// FetchCurrentUserProfile fetches the current user's profile from the GraphQL API
func FetchCurrentUserProfile(ctx context.Context, config ShellTimeConfig) (UserProfileResponse, error) {
	ctx, span := modelTracer.Start(ctx, "userProfile.fetch")
	defer span.End()

	var result GraphQLResponse[UserProfileResponse]

	err := SendGraphQLRequest(GraphQLRequestOptions[GraphQLResponse[UserProfileResponse]]{
		Context: ctx,
		Endpoint: Endpoint{
			Token:       config.Token,
			APIEndpoint: config.APIEndpoint,
		},
		Query:    FetchCurrentUserProfileQuery,
		Response: &result,
		Timeout:  5 * time.Second,
	})

	if err != nil {
		return UserProfileResponse{}, err
	}

	return result.Data, nil
}
