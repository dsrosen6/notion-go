package notion

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUsersMe(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		io.WriteString(w, `{
			"object": "user",
			"id": "bot-1",
			"name": "My Integration",
			"type": "bot",
			"bot": {"owner": {"type": "workspace", "workspace": true}, "workspace_name": "Acme"}
		}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	user, err := c.Users.Me(context.Background())
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if gotPath != "/v1/users/me" {
		t.Errorf("path = %q, want /v1/users/me", gotPath)
	}
	if user.Type != UserTypeBot {
		t.Errorf("Type = %q, want bot", user.Type)
	}
	if user.Bot == nil || user.Bot.WorkspaceName != "Acme" {
		t.Fatalf("Bot = %+v, want workspace Acme", user.Bot)
	}
	if user.Bot.Owner == nil || !user.Bot.Owner.Workspace {
		t.Errorf("Owner = %+v, want a workspace owner", user.Bot.Owner)
	}
}

func TestUsersRetrieve(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		io.WriteString(w, `{
			"object": "user", "id": "u1", "name": "Ada", "type": "person",
			"person": {"email": "ada@example.com"}
		}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	user, err := c.Users.Retrieve(context.Background(), "u1")
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if gotPath != "/v1/users/u1" {
		t.Errorf("path = %q, want /v1/users/u1", gotPath)
	}
	if !user.IsFull() || user.Name != "Ada" {
		t.Errorf("user = %+v, want a full user named Ada", user)
	}
}

func TestUsersAllPaginates(t *testing.T) {
	var cursors []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursor := r.URL.Query().Get("start_cursor")
		cursors = append(cursors, cursor)
		switch cursor {
		case "":
			io.WriteString(w, `{"object":"list","results":[
				{"object":"user","id":"u1","name":"Ada","type":"person"},
				{"object":"user","id":"u2","name":"Grace","type":"person"}
			],"next_cursor":"cur2","has_more":true}`)
		case "cur2":
			io.WriteString(w, `{"object":"list","results":[
				{"object":"user","id":"u3","name":"Alan","type":"person"}
			],"next_cursor":null,"has_more":false}`)
		default:
			t.Errorf("unexpected cursor %q", cursor)
		}
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	users, err := Collect(c.Users.All(context.Background()))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if len(users) != 3 {
		t.Fatalf("got %d users, want 3", len(users))
	}
	var names []string
	for _, u := range users {
		names = append(names, u.Name)
	}
	if fmt.Sprint(names) != "[Ada Grace Alan]" {
		t.Errorf("names = %v, want [Ada Grace Alan]", names)
	}
	// A null next_cursor must end iteration.
	if len(cursors) != 2 {
		t.Errorf("made %d requests, want 2", len(cursors))
	}
}

func TestUsersAllRequestsMaxPageSize(t *testing.T) {
	var gotSize string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSize = r.URL.Query().Get("page_size")
		io.WriteString(w, `{"object":"list","results":[],"next_cursor":null,"has_more":false}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	if _, err := Collect(c.Users.All(context.Background())); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	// A full walk should make as few requests as the API allows.
	if gotSize != "100" {
		t.Errorf("page_size = %q, want 100", gotSize)
	}
}

func TestUsersListPageSize(t *testing.T) {
	var gotSize string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSize = r.URL.Query().Get("page_size")
		io.WriteString(w, `{"object":"list","results":[],"next_cursor":null,"has_more":false}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	if _, err := c.Users.List(context.Background(), PageParams{PageSize: 50}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if gotSize != "50" {
		t.Errorf("page_size = %q, want 50", gotSize)
	}
}

func TestUsersErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"code":"object_not_found","message":"No user."}`)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	user, err := c.Users.Retrieve(context.Background(), "missing")
	if !IsNotFound(err) {
		t.Fatalf("err = %v, want object_not_found", err)
	}
	if user != nil {
		t.Errorf("user = %+v, want nil on error", user)
	}

	// An error mid-iteration must surface and stop the loop.
	var count int
	for _, err := range c.Users.All(context.Background()) {
		count++
		if err == nil {
			t.Error("expected an error from the iterator")
		}
	}
	if count != 1 {
		t.Errorf("iterated %d times, want 1", count)
	}
}
