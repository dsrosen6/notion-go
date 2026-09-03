package notion

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBlocksRetrieve(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/blocks/b1" {
			t.Errorf("path = %q, want /v1/blocks/b1", r.URL.Path)
		}
		w.Write(readFixture(t, "block_paragraph.json"))
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	block, err := c.Blocks.Retrieve(context.Background(), "b1")
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	paragraph, ok := block.(*ParagraphBlock)
	if !ok {
		t.Fatalf("got %T, want *ParagraphBlock", block)
	}
	if got := paragraph.Paragraph.RichText.PlainText(); got != "Some text" {
		t.Errorf("text = %q", got)
	}
}

func TestBlocksAllChildren(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("start_cursor") {
		case "":
			io.WriteString(w, `{"object":"list","results":[
				{"object":"block","id":"1","type":"heading_1","heading_1":{"rich_text":[{"type":"text","text":{"content":"Title"},"plain_text":"Title","annotations":{"color":"default"}}],"color":"default"}}
			],"next_cursor":"c2","has_more":true}`)
		case "c2":
			io.WriteString(w, `{"object":"list","results":[
				{"object":"block","id":"2","type":"divider","divider":{}}
			],"next_cursor":null,"has_more":false}`)
		}
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	blocks, err := Collect(c.Blocks.AllChildren(context.Background(), "page1"))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	if blocks[0].BlockType() != BlockTypeHeading1 || blocks[1].BlockType() != BlockTypeDivider {
		t.Errorf("types = %q, %q", blocks[0].BlockType(), blocks[1].BlockType())
	}
}

func TestBlocksAppendChildren(t *testing.T) {
	var gotBody map[string]any
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		json.NewDecoder(r.Body).Decode(&gotBody)
		io.WriteString(w, `{"object":"list","results":[
			{"object":"block","id":"new1","type":"paragraph","paragraph":{"rich_text":[{"type":"text","text":{"content":"Added"},"plain_text":"Added","annotations":{"color":"default"}}],"color":"default"}}
		],"next_cursor":null,"has_more":false}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	created, err := c.Blocks.AppendChildren(context.Background(), "page1", []Block{
		NewHeading1("Overview"),
		NewParagraph("Added"),
	})
	if err != nil {
		t.Fatalf("AppendChildren: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", gotMethod)
	}
	children, ok := gotBody["children"].([]any)
	if !ok || len(children) != 2 {
		t.Fatalf("body children = %#v, want 2 blocks", gotBody["children"])
	}
	// Locally built blocks must not carry response-only envelope fields.
	first := children[0].(map[string]any)
	if _, present := first["id"]; present {
		t.Errorf("sent block carries an id: %#v", first)
	}
	if first["type"] != "heading_1" {
		t.Errorf("first block type = %v, want heading_1", first["type"])
	}
	if len(created) != 1 || created[0].Base().ID != "new1" {
		t.Errorf("created = %+v, want the block the server returned", created)
	}
}

func TestBlocksAppendRejectsDeepNesting(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		io.WriteString(w, `{"object":"list","results":[],"next_cursor":null,"has_more":false}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	tooDeep := NewToggle("a", NewToggle("b", NewToggle("c", NewParagraph("d"))))
	_, err := c.Blocks.AppendChildren(context.Background(), "page1", []Block{tooDeep})

	if !errors.Is(err, ErrNestingTooDeep) {
		t.Fatalf("err = %v, want ErrNestingTooDeep", err)
	}
	// The request must be rejected locally rather than sent and refused.
	if called {
		t.Error("the request was sent, want it rejected before sending")
	}
}

func TestBlocksDelete(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		io.WriteString(w, `{"object":"block","id":"b1","in_trash":true,"type":"paragraph","paragraph":{"rich_text":[],"color":"default"}}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	block, err := c.Blocks.Delete(context.Background(), "b1")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}
	if !block.Base().InTrash {
		t.Error("InTrash = false, want true")
	}
}

func TestBlocksUpdate(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		io.WriteString(w, `{"object":"block","id":"b1","type":"paragraph","paragraph":{"rich_text":[{"type":"text","text":{"content":"Revised"},"plain_text":"Revised","annotations":{"color":"default"}}],"color":"default"}}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	block, err := c.Blocks.Update(context.Background(), "b1", NewParagraph("Revised"))
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if gotBody["type"] != "paragraph" {
		t.Errorf("body type = %v, want paragraph", gotBody["type"])
	}
	if got := block.(*ParagraphBlock).Paragraph.RichText.PlainText(); got != "Revised" {
		t.Errorf("text = %q, want Revised", got)
	}
}

func TestBlocksAllChildrenRequestsMaxPageSize(t *testing.T) {
	var gotSize string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSize = r.URL.Query().Get("page_size")
		io.WriteString(w, `{"object":"list","results":[],"next_cursor":null,"has_more":false}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	if _, err := Collect(c.Blocks.AllChildren(context.Background(), "page1")); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if gotSize != "100" {
		t.Errorf("page_size = %q, want 100", gotSize)
	}
}

func TestBlocksAppendRejectsMisplacedColumnList(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		io.WriteString(w, `{"object":"list","results":[],"next_cursor":null,"has_more":false}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	nested := NewToggle("a", &ColumnListBlock{ColumnList: ColumnListContent{Children: BlockList{
		&ColumnBlock{Column: ColumnContent{Children: BlockList{NewParagraph("b")}}},
	}}})
	_, err := c.Blocks.AppendChildren(context.Background(), "page1", []Block{nested})

	if !errors.Is(err, ErrInvalidNesting) {
		t.Fatalf("err = %v, want ErrInvalidNesting", err)
	}
	if called {
		t.Error("the request was sent, want it rejected before sending")
	}
}
