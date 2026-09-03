package notion

import (
	"context"
	"iter"
	"net/http"
	"time"
)

// CommentDisplayName is the name shown against a comment.
type CommentDisplayName struct {
	// Type is "user", "integration", or "custom".
	Type string `json:"type"`
	// ResolvedName is the name Notion displays.
	ResolvedName string `json:"resolved_name,omitempty"`
}

// CommentAttachment is a file attached to a comment.
type CommentAttachment struct {
	// Category is "audio", "image", "pdf", "productivity", or "video".
	Category string `json:"category"`
	// File is a Notion-hosted file whose URL is signed and expires.
	File FileRef `json:"file"`
}

// Comment is a comment on a page or a block.
//
// Comments belong to a discussion: the first comment starts one, and replies
// carry the same DiscussionID.
type Comment struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	// Parent is the page or block the comment is attached to.
	Parent Parent `json:"parent,omitzero"`
	// DiscussionID groups a comment with its replies.
	DiscussionID   string       `json:"discussion_id,omitempty"`
	RichText       RichTextList `json:"rich_text,omitempty"`
	CreatedTime    time.Time    `json:"created_time,omitzero"`
	LastEditedTime time.Time    `json:"last_edited_time,omitzero"`
	// CreatedBy is a partial user carrying only an ID.
	CreatedBy   User                `json:"created_by,omitzero"`
	DisplayName *CommentDisplayName `json:"display_name,omitempty"`
	Attachments []CommentAttachment `json:"attachments,omitempty"`
}

// IsFull reports whether the comment carries more than an ID. Notion signals a
// full comment by including its author, per isFullComment in
// helpers.ts:422-426.
func (c *Comment) IsFull() bool { return c != nil && c.CreatedBy.ID != "" }

// Text returns the comment's text with styling removed.
func (c *Comment) Text() string {
	if c == nil {
		return ""
	}
	return c.RichText.PlainText()
}

// CommentsService reads and writes comments. Reach it through
// [Client.Comments].
//
// The integration needs comment capabilities, which are granted separately from
// read and write access in the integration's settings.
type CommentsService struct {
	c *Client
}

// CreateCommentParams describes a comment to post. Set exactly one of Parent
// and DiscussionID.
type CreateCommentParams struct {
	// Parent starts a new discussion on a page or block.
	Parent *Parent `json:"parent,omitempty"`
	// DiscussionID replies to an existing discussion.
	DiscussionID string       `json:"discussion_id,omitempty"`
	RichText     RichTextList `json:"rich_text"`
}

// Create posts a comment, either starting a discussion on a page or replying
// to an existing one.
//
//	client.Comments.Create(ctx, notion.CreateCommentParams{
//		Parent:   &notion.Parent{Type: notion.ParentTypePage, PageID: pageID},
//		RichText: notion.NewRichText("Looks good to me."),
//	})
func (s *CommentsService) Create(ctx context.Context, params CreateCommentParams, opts ...RequestOption) (*Comment, error) {
	var out Comment
	err := s.c.do(ctx, request{
		method: http.MethodPost,
		path:   "comments",
		body:   params,
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Retrieve returns a comment by ID.
func (s *CommentsService) Retrieve(ctx context.Context, commentID string, opts ...RequestOption) (*Comment, error) {
	var out Comment
	err := s.c.do(ctx, request{
		method: http.MethodGet,
		path:   "comments/" + escapeID(commentID),
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns one page of the unresolved comments on a page or block.
//
// Resolved comments are not returned. Most callers want
// [CommentsService.All].
func (s *CommentsService) List(ctx context.Context, blockID string, params PageParams, opts ...RequestOption) (*List[Comment], error) {
	query := params.query()
	// This endpoint takes the block ID as a query parameter rather than in the
	// path, unlike every other endpoint that addresses a block.
	query["block_id"] = []string{blockID}

	var out List[Comment]
	err := s.c.do(ctx, request{
		method: http.MethodGet,
		path:   "comments",
		query:  query,
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// All iterates every unresolved comment on a page or block, fetching pages as
// needed. Each page requests the maximum size, so a full walk makes the fewest
// requests.
func (s *CommentsService) All(ctx context.Context, blockID string, opts ...RequestOption) iter.Seq2[Comment, error] {
	return paginate(ctx, "", func(ctx context.Context, cursor string) ([]Comment, string, error) {
		page, err := s.List(ctx, blockID, PageParams{StartCursor: cursor, PageSize: pageSizeMax}, opts...)
		if err != nil {
			return nil, "", err
		}
		return page.Results, page.NextCursor, nil
	})
}

// Update replaces a comment's text.
func (s *CommentsService) Update(ctx context.Context, commentID string, richText RichTextList, opts ...RequestOption) (*Comment, error) {
	body := struct {
		RichText RichTextList `json:"rich_text"`
	}{RichText: richText}

	var out Comment
	err := s.c.do(ctx, request{
		method: http.MethodPatch,
		path:   "comments/" + escapeID(commentID),
		body:   body,
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes a comment.
func (s *CommentsService) Delete(ctx context.Context, commentID string, opts ...RequestOption) error {
	return s.c.do(ctx, request{
		method: http.MethodDelete,
		path:   "comments/" + escapeID(commentID),
	}, nil, opts...)
}
