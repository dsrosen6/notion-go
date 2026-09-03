package notion

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCommentsList(t *testing.T) {
	var gotQuery, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("block_id")
		io.WriteString(w, `{"object":"list","results":[
			{"object":"comment","id":"c1","discussion_id":"d1",
			 "parent":{"type":"page_id","page_id":"p1"},
			 "created_by":{"object":"user","id":"u1"},
			 "rich_text":[{"type":"text","text":{"content":"Nice"},"plain_text":"Nice","annotations":{"color":"default"}}]}
		],"next_cursor":null,"has_more":false}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	list, err := c.Comments.List(context.Background(), "p1", PageParams{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// This endpoint addresses the block by query parameter, not path segment.
	if gotPath != "/v1/comments" {
		t.Errorf("path = %q, want /v1/comments", gotPath)
	}
	if gotQuery != "p1" {
		t.Errorf("block_id = %q, want p1", gotQuery)
	}
	if len(list.Results) != 1 {
		t.Fatalf("results = %+v", list.Results)
	}
	comment := list.Results[0]
	if !comment.IsFull() {
		t.Error("IsFull = false, want true")
	}
	if got := comment.Text(); got != "Nice" {
		t.Errorf("Text = %q, want Nice", got)
	}
	if comment.DiscussionID != "d1" {
		t.Errorf("DiscussionID = %q, want d1", comment.DiscussionID)
	}
}

func TestCommentsAllRequestsMaxPageSize(t *testing.T) {
	var gotSize, gotBlock string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSize = r.URL.Query().Get("page_size")
		gotBlock = r.URL.Query().Get("block_id")
		io.WriteString(w, `{"object":"list","results":[],"next_cursor":null,"has_more":false}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	if _, err := Collect(c.Comments.All(context.Background(), "p1")); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	// A full walk should make as few requests as the API allows.
	if gotSize != "100" {
		t.Errorf("page_size = %q, want 100", gotSize)
	}
	if gotBlock != "p1" {
		t.Errorf("block_id = %q, want p1", gotBlock)
	}
}

func TestCommentsCreate(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		io.WriteString(w, `{"object":"comment","id":"c1","discussion_id":"d1","created_by":{"object":"user","id":"u1"}}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	_, err := c.Comments.Create(context.Background(), CreateCommentParams{
		Parent:   &Parent{Type: ParentTypePage, PageID: "p1"},
		RichText: NewRichText("Looks good"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	parent := gotBody["parent"].(map[string]any)
	if parent["page_id"] != "p1" {
		t.Errorf("parent = %#v", parent)
	}
	// A new discussion must not carry a discussion ID.
	if _, present := gotBody["discussion_id"]; present {
		t.Errorf("body carries discussion_id: %#v", gotBody)
	}
}

func TestCommentsReplyToDiscussion(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		io.WriteString(w, `{"object":"comment","id":"c2","created_by":{"object":"user","id":"u1"}}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	_, err := c.Comments.Create(context.Background(), CreateCommentParams{
		DiscussionID: "d1",
		RichText:     NewRichText("Agreed"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if gotBody["discussion_id"] != "d1" {
		t.Errorf("discussion_id = %v, want d1", gotBody["discussion_id"])
	}
	if _, present := gotBody["parent"]; present {
		t.Errorf("a reply carries a parent: %#v", gotBody)
	}
}

func TestCommentsDelete(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	if err := c.Comments.Delete(context.Background(), "c1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/v1/comments/c1" {
		t.Errorf("got %s %s, want DELETE /v1/comments/c1", gotMethod, gotPath)
	}
}

func TestPartialComment(t *testing.T) {
	var comment Comment
	if err := json.Unmarshal([]byte(`{"object":"comment","id":"c1"}`), &comment); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if comment.IsFull() {
		t.Error("IsFull = true, want false for a partial comment")
	}
	var nilComment *Comment
	if nilComment.IsFull() || nilComment.Text() != "" {
		t.Error("nil comment accessors misbehaved")
	}
}
