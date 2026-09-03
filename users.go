package notion

import (
	"context"
	"iter"
	"net/http"
)

// UsersService accesses the workspace's users. Reach it through
// [Client.Users].
type UsersService struct {
	c *Client
}

// Me returns the bot user the client's token belongs to.
//
// Its Bot.Owner identifies who installed the integration, which is the simplest
// way to confirm a token works.
func (s *UsersService) Me(ctx context.Context, opts ...RequestOption) (*User, error) {
	var out User
	err := s.c.do(ctx, request{
		method: http.MethodGet,
		path:   "users/me",
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Retrieve returns a single user by ID.
func (s *UsersService) Retrieve(ctx context.Context, userID string, opts ...RequestOption) (*User, error) {
	var out User
	err := s.c.do(ctx, request{
		method: http.MethodGet,
		path:   "users/" + escapeID(userID),
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns one page of the workspace's users. Guest users are not included.
//
// Most callers want [UsersService.All] instead.
func (s *UsersService) List(ctx context.Context, params PageParams, opts ...RequestOption) (*List[User], error) {
	var out List[User]
	err := s.c.do(ctx, request{
		method: http.MethodGet,
		path:   "users",
		query:  params.query(),
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// All iterates every user in the workspace, fetching pages as needed. Each
// page requests the maximum size, so a full walk makes the fewest requests.
//
//	for user, err := range client.Users.All(ctx) {
//		if err != nil {
//			return err
//		}
//		fmt.Println(user.Name)
//	}
func (s *UsersService) All(ctx context.Context, opts ...RequestOption) iter.Seq2[User, error] {
	return paginate(ctx, "", func(ctx context.Context, cursor string) ([]User, string, error) {
		page, err := s.List(ctx, PageParams{StartCursor: cursor, PageSize: pageSizeMax}, opts...)
		if err != nil {
			return nil, "", err
		}
		return page.Results, page.NextCursor, nil
	})
}
